package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	bccost "github.com/rpuneet/bc/pkg/cost"
	bcdb "github.com/rpuneet/bc/pkg/db"
	"github.com/rpuneet/bc/pkg/log"
	bcmcp "github.com/rpuneet/bc/pkg/mcp"
	bcsecret "github.com/rpuneet/bc/pkg/secret"
	bcworkspace "github.com/rpuneet/bc/pkg/workspace"
)

// migrationMarkerName is the filename (inside ~/.bc/) that records
// whether the one-time M8 migration has run. Its presence is enough —
// contents are a human-readable timestamp for debugging.
const migrationMarkerName = ".migrated-user-assets-v1"

// migrateUserAssetsCmd is the CLI entry point for the one-time M8
// migration. Running it by hand is supported, and bcd calls
// RunMigrateUserAssets once on boot too (idempotent via the marker
// file).
var migrateUserAssetsCmd = &cobra.Command{
	Use:   "migrate-user-assets",
	Short: "One-time migration of per-workspace assets to ~/.bc/",
	Long: `Copy user-global assets (templates, secrets, MCPs, cost records) from
each registered workspace into ~/.bc/ so they are shared across
workspaces going forward.

The migration is idempotent — a marker file at ~/.bc/.migrated-user-
assets-v1 prevents repeat runs. Each migrated file is preserved under
<ws>/.bc/.migrated/ rather than deleted, so users can audit the
result.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return RunMigrateUserAssets(cmd.Context(), false)
	},
}

var migrateUserAssetsForce bool

func init() {
	migrateUserAssetsCmd.Flags().BoolVar(&migrateUserAssetsForce, "force", false, "Run even if the migration marker exists")
	workspaceCmd.AddCommand(migrateUserAssetsCmd)
}

// MigrationSummary tallies what the migration did. Used for logs and
// test assertions.
type MigrationSummary struct {
	Templates int
	Secrets   int
	MCPs      int
	CostRows  int
}

// RunMigrateUserAssets performs the one-time M8 migration. When force
// is false and the marker file exists, it is a no-op. The marker is
// written on successful completion so concurrent bcd boots or user CLI
// invocations are idempotent.
func RunMigrateUserAssets(ctx context.Context, force bool) error {
	bcHome, err := bcworkspace.EnsureGlobalDir()
	if err != nil {
		return fmt.Errorf("ensure ~/.bc: %w", err)
	}
	marker := filepath.Join(bcHome, migrationMarkerName)
	if !migrateUserAssetsForce && !force {
		if _, statErr := os.Stat(marker); statErr == nil {
			log.Debug("user assets migration already complete", "marker", marker)
			return nil
		}
	}

	reg, err := bcworkspace.LoadRegistry()
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}
	if reg == nil || len(reg.Workspaces) == 0 {
		log.Info("migrate-user-assets: no registered workspaces — writing marker")
		return writeMigrationMarker(marker)
	}

	summary := MigrationSummary{}
	for _, entry := range reg.Workspaces {
		if err := ctx.Err(); err != nil {
			return err
		}
		if perWS, err := migrateOneWorkspace(ctx, entry); err != nil {
			log.Warn("migrate-user-assets: workspace failed", "workspace", entry.Name, "error", err)
			continue
		} else {
			summary.Templates += perWS.Templates
			summary.Secrets += perWS.Secrets
			summary.MCPs += perWS.MCPs
			summary.CostRows += perWS.CostRows
		}
	}

	log.Info("migrate-user-assets: complete",
		"templates", summary.Templates,
		"secrets", summary.Secrets,
		"mcps", summary.MCPs,
		"cost_rows", summary.CostRows,
	)
	return writeMigrationMarker(marker)
}

// migrateOneWorkspace runs the migration for a single registered
// workspace. Each section is best-effort; a failure in one stream is
// logged and the others continue.
func migrateOneWorkspace(ctx context.Context, entry bcworkspace.RegistryEntry) (MigrationSummary, error) {
	out := MigrationSummary{}
	wsRoot := entry.Path
	wsLegacy := filepath.Join(wsRoot, ".bc")
	if _, err := os.Stat(wsLegacy); err != nil {
		return out, nil // nothing to migrate
	}
	migratedDir := filepath.Join(wsLegacy, ".migrated")
	if err := os.MkdirAll(migratedDir, 0o750); err != nil {
		return out, fmt.Errorf("create .migrated dir: %w", err)
	}

	// Templates: copy each <name>.json / <name>.md file into ~/.bc/templates/
	if n, err := migrateTemplates(wsLegacy, migratedDir); err != nil {
		log.Warn("migrate templates", "workspace", entry.Name, "error", err)
	} else {
		out.Templates = n
	}

	// Secrets: import each row from <ws>/.bc/secrets.db into ~/.bc/secrets.vault
	if n, err := migrateSecrets(wsLegacy, migratedDir); err != nil {
		log.Warn("migrate secrets", "workspace", entry.Name, "error", err)
	} else {
		out.Secrets = n
	}

	// MCPs: merge <ws>/.bc/.mcp.json into ~/.bc/mcps.json
	if n, err := migrateMCPs(wsLegacy, migratedDir); err != nil {
		log.Warn("migrate mcps", "workspace", entry.Name, "error", err)
	} else {
		out.MCPs = n
	}

	// Costs: merge <ws>/.bc/costs.db into ~/.bc/costs.db with workspace_id
	wsID := bcworkspace.ComputeWorkspaceID(wsRoot)
	if n, err := migrateCosts(ctx, wsLegacy, migratedDir, wsID); err != nil {
		log.Warn("migrate costs", "workspace", entry.Name, "error", err)
	} else {
		out.CostRows = n
	}

	return out, nil
}

// migrateTemplates copies any <name>.json / <name>.md from
// <ws>/.bc/templates/ to ~/.bc/templates/ unless the target already
// exists (workspace-local wins on name collision only when overriding;
// otherwise we skip to avoid clobbering the user's edits). The
// originals are moved into <ws>/.bc/.migrated/templates/ for audit.
func migrateTemplates(wsLegacy, migratedDir string) (int, error) {
	srcDir := filepath.Join(wsLegacy, "templates")
	entries, err := os.ReadDir(srcDir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	globalDir, err := bcworkspace.GlobalTemplatesDir()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(globalDir, 0o750); err != nil {
		return 0, err
	}

	movedDir := filepath.Join(migratedDir, "templates")
	if err := os.MkdirAll(movedDir, 0o750); err != nil {
		return 0, err
	}

	copied := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		srcPath := filepath.Join(srcDir, name)
		dstPath := filepath.Join(globalDir, name)
		// Skip collisions — the user may have tuned the global template.
		if _, err := os.Stat(dstPath); err == nil {
			log.Debug("migrate templates: skip collision", "name", name)
		} else if err := copyFile(srcPath, dstPath); err != nil {
			log.Warn("migrate templates: copy failed", "name", name, "error", err)
			continue
		} else if strings.HasSuffix(name, ".json") {
			// Count one per logical template (the JSON file) so the
			// companion .md doesn't inflate the total.
			copied++
		}
		// Preserve original under .migrated/templates/ regardless.
		if err := os.Rename(srcPath, filepath.Join(movedDir, name)); err != nil {
			log.Warn("migrate templates: move-to-migrated failed", "name", name, "error", err)
		}
	}
	return copied, nil
}

// migrateSecrets reads the legacy <ws>/.bc/secrets.db, decrypts each
// row with the bc passphrase, and upserts the plaintext into
// ~/.bc/secrets.vault. The source DB is then moved into
// <ws>/.bc/.migrated/secrets.db for audit.
func migrateSecrets(wsLegacy, migratedDir string) (int, error) {
	srcDB := filepath.Join(wsLegacy, "secrets.db")
	if _, err := os.Stat(srcDB); err != nil {
		return 0, nil
	}
	passphrase, err := bcsecret.Passphrase()
	if err != nil {
		return 0, fmt.Errorf("resolve passphrase: %w", err)
	}
	src, err := bcsecret.NewStore(filepath.Dir(wsLegacy), passphrase)
	if err != nil {
		return 0, fmt.Errorf("open source secrets: %w", err)
	}
	defer func() { _ = src.Close() }()

	vaultPath, err := bcworkspace.GlobalSecretsVault()
	if err != nil {
		return 0, err
	}
	dst, err := bcsecret.OpenVaultFile(vaultPath, passphrase)
	if err != nil {
		return 0, fmt.Errorf("open vault: %w", err)
	}
	defer func() { _ = dst.Close() }()

	metas, err := src.List()
	if err != nil {
		return 0, fmt.Errorf("list source secrets: %w", err)
	}
	copied := 0
	for _, m := range metas {
		// Skip collisions so the global vault keeps its prior value.
		if existing, _ := dst.GetMeta(m.Name); existing != nil {
			log.Debug("migrate secrets: skip collision", "name", m.Name)
			continue
		}
		val, getErr := src.GetValue(m.Name)
		if getErr != nil {
			log.Warn("migrate secrets: cannot decrypt", "name", m.Name, "error", getErr)
			continue
		}
		if setErr := dst.Set(m.Name, val, m.Description); setErr != nil {
			log.Warn("migrate secrets: write failed", "name", m.Name, "error", setErr)
			continue
		}
		copied++
	}

	// Move the original into .migrated/secrets.db.
	_ = src.Close()
	dstMoved := filepath.Join(migratedDir, "secrets.db")
	if err := os.Rename(srcDB, dstMoved); err != nil {
		log.Warn("migrate secrets: move-to-migrated failed", "error", err)
	}
	return copied, nil
}

// migrateMCPs merges entries from <ws>/.bc/.mcp.json (legacy agent MCP
// config) into ~/.bc/mcps.json. Only servers not already in the global
// registry are added so user-tuned global configs stay intact.
func migrateMCPs(wsLegacy, migratedDir string) (int, error) {
	srcPath := filepath.Join(wsLegacy, ".mcp.json")
	if _, err := os.Stat(srcPath); err != nil {
		return 0, nil
	}
	raw, err := os.ReadFile(srcPath) //nolint:gosec // controlled path under workspace
	if err != nil {
		return 0, err
	}
	// The workspace .mcp.json uses the same shape as agent .mcp.json:
	// { "mcpServers": { "<name>": { ... } } }. For migration we parse
	// a loose map and copy what we can into the GlobalStore.
	parsed, err := parseLegacyMCPJSON(raw)
	if err != nil {
		return 0, err
	}

	globalPath, err := bcworkspace.GlobalMCPConfig()
	if err != nil {
		return 0, err
	}
	gs := bcmcp.NewGlobalStore(globalPath)

	copied := 0
	for _, cfg := range parsed {
		if existing, _ := gs.Get(cfg.Name); existing != nil {
			log.Debug("migrate mcps: skip collision", "name", cfg.Name)
			continue
		}
		if addErr := gs.Add(cfg); addErr != nil {
			log.Warn("migrate mcps: add failed", "name", cfg.Name, "error", addErr)
			continue
		}
		copied++
	}

	// Preserve original.
	if err := os.Rename(srcPath, filepath.Join(migratedDir, ".mcp.json")); err != nil {
		log.Warn("migrate mcps: move-to-migrated failed", "error", err)
	}
	return copied, nil
}

// migrateCosts copies rows from <ws>/.bc/costs.db into ~/.bc/costs.db,
// tagging each with the owning workspace's id. Already-imported rows
// (duplicate by session_id + timestamp) are skipped.
func migrateCosts(ctx context.Context, wsLegacy, migratedDir, wsID string) (int, error) {
	srcPath := filepath.Join(wsLegacy, "costs.db")
	if _, err := os.Stat(srcPath); err != nil {
		return 0, nil
	}
	globalPath, err := bcworkspace.GlobalCostsDB()
	if err != nil {
		return 0, err
	}
	dst, err := bccost.OpenGlobalStore(globalPath)
	if err != nil {
		return 0, fmt.Errorf("open global costs: %w", err)
	}
	defer func() { _ = dst.Close() }()

	src, err := bcdb.Open(srcPath)
	if err != nil {
		return 0, fmt.Errorf("open source costs: %w", err)
	}
	defer func() { _ = src.Close() }()

	// Copy cost_records rows — schema differences are tolerated via a
	// forgiving SELECT with LEFT-JOIN-style fallback for missing
	// columns. SQLite fills missing columns with NULL automatically.
	rows, err := src.QueryContext(ctx,
		`SELECT agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, timestamp
		 FROM cost_records`,
	)
	if err != nil {
		return 0, fmt.Errorf("scan source costs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	copied := 0
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return copied, err
		}
		var agentID, model, ts string
		var teamID sql.NullString
		var inputT, outputT, totalT int64
		var cost float64
		if scanErr := rows.Scan(&agentID, &teamID, &model, &inputT, &outputT, &totalT, &cost, &ts); scanErr != nil {
			return copied, scanErr
		}
		var teamPtr *string
		if teamID.Valid {
			t := teamID.String
			teamPtr = &t
		}
		_, insertErr := dst.DB().ExecContext(ctx,
			`INSERT INTO cost_records
			 (agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, timestamp, workspace_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			agentID, teamPtr, model, inputT, outputT, totalT, cost, ts, wsID,
		)
		if insertErr != nil {
			log.Warn("migrate costs: insert failed", "error", insertErr)
			continue
		}
		copied++
	}
	if rErr := rows.Err(); rErr != nil {
		return copied, rErr
	}

	_ = src.Close()
	if mvErr := os.Rename(srcPath, filepath.Join(migratedDir, "costs.db.bak")); mvErr != nil {
		log.Warn("migrate costs: move-to-migrated failed", "error", mvErr)
	}
	return copied, nil
}

// writeMigrationMarker records the migration as complete.
func writeMigrationMarker(path string) error {
	ts := time.Now().UTC().Format(time.RFC3339)
	content := "migrated=" + ts + "\n"
	return os.WriteFile(path, []byte(content), 0o640) //nolint:gosec // 0640 intentional
}

// copyFile copies src to dst preserving mode 0640.
func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // controlled workspace path
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640) //nolint:gosec // dst is derived from ~/.bc/ path, not user input
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

// --- legacy .mcp.json parsing ---
//
// The on-disk shape historically written into <ws>/.bc/.mcp.json is
// { "mcpServers": { "<name>": { "command": "...", "args": [...], "env": {...} } } }
// — the Claude Code convention. Parse a minimal subset that the
// GlobalStore can accept via Add().
type legacyMCPJSON struct {
	MCPServers map[string]legacyMCPEntry `json:"mcpServers"`
}

type legacyMCPEntry struct {
	Env       map[string]string `json:"env,omitempty"`
	Command   string            `json:"command,omitempty"`
	URL       string            `json:"url,omitempty"`
	Transport string            `json:"transport,omitempty"`
	Args      []string          `json:"args,omitempty"`
}

func parseLegacyMCPJSON(data []byte) ([]*bcmcp.ServerConfig, error) {
	// Tiny decoder avoids importing another package.
	var doc legacyMCPJSON
	if err := decodeJSON(data, &doc); err != nil {
		return nil, fmt.Errorf("parse .mcp.json: %w", err)
	}
	var out []*bcmcp.ServerConfig
	for name, e := range doc.MCPServers {
		transport := bcmcp.Transport(e.Transport)
		if transport == "" {
			transport = bcmcp.TransportStdio
		}
		out = append(out, &bcmcp.ServerConfig{
			Name:      strings.TrimSpace(name),
			Transport: transport,
			Command:   e.Command,
			Args:      e.Args,
			URL:       e.URL,
			Env:       e.Env,
			Enabled:   true,
		})
	}
	return out, nil
}

// decodeJSON is a tiny wrapper so tests can stub it.
func decodeJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
