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
	ToolTypeMCP      = "mcp"      // MCP server (bc, playwright, github)
	ToolTypeProvider = "provider" // AI provider (claude, gemini, cursor)
)

// Tool represents a configured tool in the workspace (CLI, MCP server, or AI provider).
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
		Name:       "gemini",
		Command:    "gemini",
		InstallCmd: "npm install -g @google/gemini-cli",
		SlashCmds:  []string{"/help", "/quit"},
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
		Name:      "bc",
		Type:      ToolTypeMCP,
		Transport: "sse",
		URL:       "http://host.docker.internal:9374/_mcp/sse",
		Enabled:   true,
		Builtin:   true,
	},
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
	db *db.DB
	pg *PostgresStore // non-nil when using Postgres via OpenStore
}

// NewStore creates a new tool store for the given workspace state directory.
func NewStore(stateDir string) *Store {
	return &Store{}
}

// Open initializes the database and seeds built-in tools.
// Uses the shared workspace database; returns an error if unavailable.
func (s *Store) Open() error {
	shared := db.SharedWrapped()
	if shared == nil {
		return fmt.Errorf("tool store requires shared database (none available)")
	}

	s.db = shared

	if db.SharedDriver() == "timescale" {
		// Use PostgresStore for proper $1 placeholder queries.
		s.pg = NewPostgresStore(db.Shared())
	} else {
		if err := initSchema(shared.DB); err != nil {
			return fmt.Errorf("failed to initialize schema: %w", err)
		}
	}

	if err := s.seedBuiltins(context.Background()); err != nil {
		return fmt.Errorf("failed to seed built-in tools: %w", err)
	}

	return nil
}

// Close is a no-op — the shared DB is owned by the caller.
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
			type          TEXT NOT NULL DEFAULT 'provider',
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
		"ALTER TABLE tools ADD COLUMN type TEXT NOT NULL DEFAULT 'provider'",
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
		existing, err := s.Get(ctx, t.Name)
		if err != nil {
			return fmt.Errorf("failed to check %s: %w", t.Name, err)
		}
		if existing != nil {
			continue
		}
		if err := s.add(ctx, &t); err != nil {
			return fmt.Errorf("failed to seed %s: %w", t.Name, err)
		}
	}
	return nil
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

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO tools (name, command, install_cmd, upgrade_cmd, slash_cmds, mcp_servers, config, builtin, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.Command, t.InstallCmd, t.UpgradeCmd,
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

	res, err := s.db.ExecContext(ctx,
		`UPDATE tools SET command=?, install_cmd=?, upgrade_cmd=?, slash_cmds=?, mcp_servers=?, config=?, enabled=?
		 WHERE name=?`,
		t.Command, t.InstallCmd, t.UpgradeCmd,
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
