// Package tool provides persistent storage and management for AI tool providers.
package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// ToolType classifies a tool.
const (
	ToolTypeCLI      = "cli"      // CLI binary (gh, aws, wrangler)
	ToolTypeMCP      = "mcp"      // MCP server (playwright, github, etc.)
	ToolTypeProvider = "provider" // AI provider (claude, agy, cursor)
)

// Tool represents a configured tool (CLI, MCP server, or AI provider).
type Tool struct {
	CreatedAt    time.Time         `json:"created_at"`
	Config       map[string]any    `json:"config,omitempty"`
	Env          map[string]string `json:"env,omitempty"` // env vars, supports ${secret:NAME}
	Name         string            `json:"name"`
	Type         string            `json:"type"` // "cli", "mcp", "provider"
	Command      string            `json:"command"`
	InstallCmd   string            `json:"install_cmd,omitempty"`
	UpgradeCmd   string            `json:"upgrade_cmd,omitempty"`
	VersionCmd   string            `json:"version_cmd,omitempty"`   // e.g., "gh --version"
	Transport    string            `json:"transport,omitempty"`     // "stdio", "sse" (MCP only)
	URL          string            `json:"url,omitempty"`           // SSE endpoint (MCP only)
	HealthStatus string            `json:"health_status,omitempty"` // connected/installed/not_installed/error
	LastChecked  string            `json:"last_checked,omitempty"`  // ISO timestamp
	SlashCmds    []string          `json:"slash_cmds,omitempty"`
	Args         []string          `json:"args,omitempty"`        // stdio args (MCP only)
	MCPServers   []string          `json:"mcp_servers,omitempty"` // associated MCP server names
	Builtin      bool              `json:"builtin,omitempty"`
	Enabled      bool              `json:"enabled"`
}

// builtinTools contains default configurations for popular AI tools.
var builtinTools = []Tool{
	{
		Name:       "claude",
		Command:    "claude --dangerously-skip-permissions",
		InstallCmd: "npm install -g @anthropic-ai/claude-code",
		UpgradeCmd: "npm update -g @anthropic-ai/claude-code",
		SlashCmds:  []string{"/clear", "/compact", "/help", "/mcp", "/cost", "/quit"},
		Enabled:    true,
		Builtin:    true,
		Type:       ToolTypeProvider,
	},
	{
		Name:       "cursor",
		Command:    "cursor-agent",
		InstallCmd: "npm install -g cursor-agent",
		SlashCmds:  []string{"/exit", "/help"},
		Enabled:    true,
		Builtin:    true,
		Type:       ToolTypeProvider,
	},
	{
		Name:       "agy",
		Command:    "agy --dangerously-skip-permissions",
		InstallCmd: "curl -fsSL https://antigravity.google/install.sh | sh",
		SlashCmds:  []string{"/help", "/model", "/resume", "/exit"},
		Enabled:    true,
		Builtin:    true,
		Type:       ToolTypeProvider,
	},
	{
		Name:       "codex",
		Command:    "codex --full-auto",
		InstallCmd: "npm install -g @openai/codex",
		SlashCmds:  []string{"/help", "/quit"},
		Enabled:    true,
		Builtin:    true,
		Type:       ToolTypeProvider,
	},
}

// builtinMCPServers contains default MCP server definitions.
var builtinMCPServers = []Tool{
	{
		Name:       "playwright",
		Type:       ToolTypeMCP,
		Transport:  "sse",
		URL:        "http://host.docker.internal:3000/sse",
		InstallCmd: "npx -y @playwright/mcp@latest",
		Enabled:    true,
		Builtin:    true,
	},
	{
		Name:       "github",
		Type:       ToolTypeMCP,
		Transport:  "stdio",
		Command:    "github-mcp-server",
		InstallCmd: "go install github.com/github/github-mcp-server@latest",
		Env:        map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "${secret:GITHUB_PERSONAL_ACCESS_TOKEN}"},
		Enabled:    true,
		Builtin:    true,
	},
}

// builtinCLITools contains common system CLI tools that should be auto-detected.
var builtinCLITools = []Tool{
	{Name: "gh", Command: "gh", Type: ToolTypeCLI, Builtin: true, Enabled: true, InstallCmd: "brew install gh", VersionCmd: "gh --version"},
	{Name: "git", Command: "git", Type: ToolTypeCLI, Builtin: true, Enabled: true, VersionCmd: "git --version"},
	{Name: "go", Command: "go", Type: ToolTypeCLI, Builtin: true, Enabled: true, VersionCmd: "go version"},
	{Name: "make", Command: "make", Type: ToolTypeCLI, Builtin: true, Enabled: true, VersionCmd: "make --version"},
	{Name: "docker", Command: "docker", Type: ToolTypeCLI, Builtin: true, Enabled: true, VersionCmd: "docker --version"},
	{Name: "bun", Command: "bun", Type: ToolTypeCLI, Builtin: true, Enabled: true, VersionCmd: "bun --version"},
	{Name: "node", Command: "node", Type: ToolTypeCLI, Builtin: true, Enabled: true, VersionCmd: "node --version"},
	{Name: "python3", Command: "python3", Type: ToolTypeCLI, Builtin: true, Enabled: true, VersionCmd: "python3 --version"},
	{Name: "curl", Command: "curl", Type: ToolTypeCLI, Builtin: true, Enabled: true, VersionCmd: "curl --version"},
	{Name: "jq", Command: "jq", Type: ToolTypeCLI, Builtin: true, Enabled: true, VersionCmd: "jq --version"},
	{Name: "aws", Command: "aws", Type: ToolTypeCLI, Builtin: true, Enabled: true, InstallCmd: "brew install awscli", VersionCmd: "aws --version"},
	{Name: "tmux", Command: "tmux", Type: ToolTypeCLI, Builtin: true, Enabled: true, VersionCmd: "tmux -V"},
}

// Store provides tool management backed by SQLite or TimescaleDB (Postgres).
type Store struct {
	db     *db.DB
	pg     *PostgresStore // non-nil when using the timescale driver
	driver string         // "sqlite" or "timescale"
}

// NewStore creates a new tool store on the given database. The
// handle is borrowed: callers (typically the global db registry)
// own its lifecycle. Call Open to initialize the schema and seed
// built-in tools.
func NewStore(d *db.DB, driver string) *Store {
	return &Store{db: d, driver: driver}
}

// Open initializes the database schema and seeds built-in tools.
// Returns an error if the store was constructed without a database.
func (s *Store) Open() error {
	if s.db == nil {
		return fmt.Errorf("tool store requires a database (nil handle)")
	}

	if s.driver == "timescale" {
		// Use PostgresStore for proper $1 placeholder queries.
		pg := NewPostgresStore(s.db.DB)
		if err := pg.InitSchema(); err != nil {
			return fmt.Errorf("failed to initialize timescale schema: %w", err)
		}
		s.pg = pg
	} else {
		if err := initSchema(s.db.DB); err != nil {
			return fmt.Errorf("failed to initialize schema: %w", err)
		}
	}

	if err := s.seedBuiltins(context.Background()); err != nil {
		return fmt.Errorf("failed to seed built-in tools: %w", err)
	}

	return nil
}

// Close is a no-op — the global DB is owned by the caller.
func (s *Store) Close() error {
	if s.pg != nil {
		return s.pg.Close()
	}
	return nil
}

func initSchema(db *sql.DB) error {
	// context.TODO() retained: initSchema runs synchronously during Store.Open
	// at startup/test setup before any request context exists; threading ctx
	// through would force a public API change on NewStore/Open and dozens of
	// call sites across tests/services for no operational benefit (DDL completes
	// in microseconds and cannot be canceled meaningfully).
	_, err := db.ExecContext(context.TODO(), `
		CREATE TABLE IF NOT EXISTS tools (
			name          TEXT PRIMARY KEY,
			type          TEXT NOT NULL DEFAULT 'cli',
			command       TEXT NOT NULL DEFAULT '',
			install_cmd   TEXT,
			upgrade_cmd   TEXT,
			version_cmd   TEXT,
			transport     TEXT DEFAULT '',
			url           TEXT,
			args          TEXT DEFAULT '[]',
			env           TEXT DEFAULT '{}',
			slash_cmds    TEXT,
			mcp_servers   TEXT,
			config        TEXT,
			health_status TEXT DEFAULT 'unknown',
			last_checked  TEXT,
			builtin       BOOLEAN DEFAULT FALSE,
			enabled       BOOLEAN DEFAULT TRUE,
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// Migration: add new columns to existing tables
	for _, col := range []string{
		"ALTER TABLE tools ADD COLUMN type TEXT NOT NULL DEFAULT 'cli'",
		"ALTER TABLE tools ADD COLUMN transport TEXT DEFAULT ''",
		"ALTER TABLE tools ADD COLUMN url TEXT",
		"ALTER TABLE tools ADD COLUMN args TEXT DEFAULT '[]'",
		"ALTER TABLE tools ADD COLUMN env TEXT DEFAULT '{}'",
		"ALTER TABLE tools ADD COLUMN version_cmd TEXT",
		"ALTER TABLE tools ADD COLUMN health_status TEXT DEFAULT 'unknown'",
		"ALTER TABLE tools ADD COLUMN last_checked TEXT",
	} {
		// context.TODO() retained: see initSchema header comment — startup-only DDL.
		_, _ = db.ExecContext(context.TODO(), col) //nolint:errcheck // ignore if columns exist
	}

	return nil
}

func (s *Store) seedBuiltins(ctx context.Context) error {
	// Delegate to PostgresStore when using timescale (uses $1 placeholders).
	if s.pg != nil {
		return s.pg.SeedBuiltins(ctx)
	}

	for _, t := range allBuiltins() {
		t := t
		existing, err := s.Get(ctx, t.Name)
		if err != nil {
			return fmt.Errorf("failed to check %s: %w", t.Name, err)
		}
		if existing != nil {
			// Correct rows seeded by older code that dropped the type and
			// version_cmd columns (all builtins landed as type='provider'
			// with an empty version_cmd). Idempotent: only writes when the
			// builtin row's type/version_cmd drifts from the definition.
			if existing.Builtin && (existing.Type != t.Type || existing.VersionCmd != t.VersionCmd) {
				if err := s.correctBuiltin(ctx, t.Name, t.Type, t.VersionCmd); err != nil {
					return fmt.Errorf("failed to correct %s: %w", t.Name, err)
				}
			}
			continue
		}
		if err := s.add(ctx, &t); err != nil {
			return fmt.Errorf("failed to seed %s: %w", t.Name, err)
		}
	}
	return nil
}

// correctBuiltin fixes the type/version_cmd of an existing built-in row that
// predates the columns being persisted. Idempotent by construction.
func (s *Store) correctBuiltin(ctx context.Context, name, toolType, versionCmd string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tools SET type=?, version_cmd=? WHERE name=? AND builtin=1`,
		toolType, versionCmd, name,
	)
	return err
}

// allBuiltins returns all built-in tool definitions (providers, MCP servers, CLI tools).
func allBuiltins() []Tool {
	all := make([]Tool, 0, len(builtinTools)+len(builtinMCPServers)+len(builtinCLITools))
	all = append(all, builtinTools...)
	all = append(all, builtinMCPServers...)
	all = append(all, builtinCLITools...)
	return all
}

func marshalJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalStrings(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

func unmarshalMap(s string) map[string]any {
	if s == "" {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

func unmarshalStringMap(s string) map[string]string {
	if s == "" || s == "{}" {
		return nil
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

func (s *Store) add(ctx context.Context, t *Tool) error {
	slashCmds, err := marshalJSON(t.SlashCmds)
	if err != nil {
		return err
	}
	mcpServers, err := marshalJSON(t.MCPServers)
	if err != nil {
		return err
	}
	config, err := marshalJSON(t.Config)
	if err != nil {
		return err
	}
	args, err := marshalJSON(t.Args)
	if err != nil {
		return err
	}
	env, err := marshalJSON(t.Env)
	if err != nil {
		return err
	}

	toolType := t.Type
	if toolType == "" {
		toolType = ToolTypeCLI
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO tools (name, type, command, install_cmd, upgrade_cmd, version_cmd, transport, url, args, env, slash_cmds, mcp_servers, config, builtin, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Name, toolType, t.Command, t.InstallCmd, t.UpgradeCmd, t.VersionCmd,
		t.Transport, t.URL, args, env,
		slashCmds, mcpServers, config, t.Builtin, t.Enabled,
	)
	return err
}

// Add inserts a new tool. Returns an error if a tool with that name already exists.
func (s *Store) Add(ctx context.Context, t *Tool) error {
	if s.pg != nil {
		return s.pg.Add(ctx, t)
	}
	if t.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if t.Command == "" {
		return fmt.Errorf("tool command is required")
	}
	existing, err := s.Get(ctx, t.Name)
	if err == nil && existing != nil {
		return fmt.Errorf("tool %q already exists", t.Name)
	}
	return s.add(ctx, t)
}

// allColumns is the SELECT column list for the unified tools table.
const allColumns = `name, type, command, install_cmd, upgrade_cmd, version_cmd,
	transport, url, args, env, slash_cmds, mcp_servers, config,
	health_status, last_checked, builtin, enabled, created_at`

// Get returns a tool by name. Returns nil, nil if not found.
func (s *Store) Get(ctx context.Context, name string) (*Tool, error) {
	if s.pg != nil {
		return s.pg.Get(ctx, name)
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+allColumns+` FROM tools WHERE name = ?`, name)
	return scanToolFrom(row)
}

// toolScanner is implemented by both *sql.Row and *sql.Rows.
type toolScanner interface {
	Scan(dest ...any) error
}

// scanToolFrom scans a row into a Tool. Returns (nil, nil) for sql.ErrNoRows.
func scanToolFrom(sc toolScanner) (*Tool, error) {
	var t Tool
	var toolType, installCmd, upgradeCmd, versionCmd sql.NullString
	var transport, url, argsJSON, envJSON sql.NullString
	var slashCmds, mcpServers, config sql.NullString
	var healthStatus, lastChecked sql.NullString
	if err := sc.Scan(
		&t.Name, &toolType, &t.Command,
		&installCmd, &upgradeCmd, &versionCmd,
		&transport, &url, &argsJSON, &envJSON,
		&slashCmds, &mcpServers, &config,
		&healthStatus, &lastChecked,
		&t.Builtin, &t.Enabled, &t.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	t.Type = toolType.String
	if t.Type == "" {
		t.Type = ToolTypeProvider
	}
	t.InstallCmd = installCmd.String
	t.UpgradeCmd = upgradeCmd.String
	t.VersionCmd = versionCmd.String
	t.Transport = transport.String
	t.URL = url.String
	t.Args = unmarshalStrings(argsJSON.String)
	t.Env = unmarshalStringMap(envJSON.String)
	t.SlashCmds = unmarshalStrings(slashCmds.String)
	t.MCPServers = unmarshalStrings(mcpServers.String)
	t.Config = unmarshalMap(config.String)
	t.HealthStatus = healthStatus.String
	t.LastChecked = lastChecked.String
	return &t, nil
}

// ListOptions controls tool listing behavior.
type ListOptions struct {
	Types []string // filter by type (e.g., ["cli", "mcp"])
}

// List returns all tools, optionally filtered by type.
func (s *Store) List(ctx context.Context) ([]*Tool, error) {
	return s.ListWithOptions(ctx, ListOptions{})
}

// ListWithOptions returns tools filtered by the given options.
func (s *Store) ListWithOptions(ctx context.Context, opts ListOptions) ([]*Tool, error) {
	if s.pg != nil {
		return s.pg.List(ctx)
	}

	query := `SELECT ` + allColumns + ` FROM tools`
	var args []any
	if len(opts.Types) > 0 {
		placeholders := make([]string, len(opts.Types))
		for i, t := range opts.Types {
			placeholders[i] = "?"
			args = append(args, t)
		}
		query += ` WHERE type IN (` + strings.Join(placeholders, ",") + `)`
	}
	query += ` ORDER BY builtin DESC, type ASC, name ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var tools []*Tool
	for rows.Next() {
		t, scanErr := scanToolFrom(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		tools = append(tools, t)
	}
	return tools, rows.Err()
}

// Update replaces a tool's mutable fields (command, install_cmd, upgrade_cmd, slash_cmds, mcp_servers, config, enabled).
func (s *Store) Update(ctx context.Context, t *Tool) error {
	if s.pg != nil {
		return s.pg.Update(ctx, t)
	}
	slashCmds, err := marshalJSON(t.SlashCmds)
	if err != nil {
		return err
	}
	mcpServers, err := marshalJSON(t.MCPServers)
	if err != nil {
		return err
	}
	config, err := marshalJSON(t.Config)
	if err != nil {
		return err
	}
	args, err := marshalJSON(t.Args)
	if err != nil {
		return err
	}
	env, err := marshalJSON(t.Env)
	if err != nil {
		return err
	}

	toolType := t.Type
	if toolType == "" {
		toolType = ToolTypeCLI
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE tools SET type=?, command=?, install_cmd=?, upgrade_cmd=?, version_cmd=?, transport=?, url=?, args=?, env=?, slash_cmds=?, mcp_servers=?, config=?, enabled=?
		 WHERE name=?`,
		toolType, t.Command, t.InstallCmd, t.UpgradeCmd, t.VersionCmd,
		t.Transport, t.URL, args, env,
		slashCmds, mcpServers, config, t.Enabled,
		t.Name,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("tool %q not found", t.Name)
	}
	return nil
}

// UpdateHealth persists a fresh health_status + last_checked timestamp for
// a tool without touching its other mutable fields. Used by the manual
// /api/tools/check force-refresh and the background auto-check loop, so
// GET /api/tools always serves recently-verified status instead of the
// seed-time default.
func (s *Store) UpdateHealth(ctx context.Context, name, status, lastChecked string) error {
	if s.pg != nil {
		return s.pg.UpdateHealth(ctx, name, status, lastChecked)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE tools SET health_status=?, last_checked=? WHERE name=?`,
		status, lastChecked, name,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("tool %q not found", name)
	}
	return nil
}

// Delete removes a tool by name.
func (s *Store) Delete(ctx context.Context, name string) error {
	if s.pg != nil {
		return s.pg.Delete(ctx, name)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM tools WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("tool %q not found", name)
	}
	return nil
}

// SetEnabled enables or disables a tool.
func (s *Store) SetEnabled(ctx context.Context, name string, enabled bool) error {
	if s.pg != nil {
		return s.pg.SetEnabled(ctx, name, enabled)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE tools SET enabled=? WHERE name=?`, enabled, name)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("tool %q not found", name)
	}
	return nil
}
