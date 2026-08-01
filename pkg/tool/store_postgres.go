package tool

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
)

// PostgresStore provides Postgres-backed tool management.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a PostgresStore from an existing *sql.DB connection.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// InitSchema creates the tools table in Postgres if it doesn't exist.
func (p *PostgresStore) InitSchema() error {
	ctx := context.Background()

	stmt := `CREATE TABLE IF NOT EXISTS tools (
		name        TEXT PRIMARY KEY,
		type        TEXT NOT NULL DEFAULT 'cli',
		command     TEXT NOT NULL,
		install_cmd TEXT,
		upgrade_cmd TEXT,
		version_cmd TEXT,
		transport   TEXT,
		url         TEXT,
		slash_cmds  TEXT,
		mcp_servers TEXT,
		config      TEXT,
		builtin     BOOLEAN DEFAULT FALSE,
		enabled     BOOLEAN DEFAULT TRUE,
		created_at  TIMESTAMPTZ DEFAULT NOW()
	)`

	if _, err := p.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("postgres tools schema: %w", err)
	}

	// Migrate: add columns that may be missing on older schemas.
	migrations := []string{
		`ALTER TABLE tools ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'cli'`,
		`ALTER TABLE tools ADD COLUMN IF NOT EXISTS version_cmd TEXT`,
		`ALTER TABLE tools ADD COLUMN IF NOT EXISTS transport TEXT`,
		`ALTER TABLE tools ADD COLUMN IF NOT EXISTS url TEXT`,
		`ALTER TABLE tools ADD COLUMN IF NOT EXISTS health_status TEXT DEFAULT 'unknown'`,
		`ALTER TABLE tools ADD COLUMN IF NOT EXISTS last_checked TEXT`,
	}
	for _, m := range migrations {
		if _, err := p.db.ExecContext(ctx, m); err != nil {
			log.Debug("tool schema migration (may already exist): %v", err)
		}
	}

	return nil
}

// Close is a no-op — the shared DB is owned by the caller.
func (p *PostgresStore) Close() error {
	return nil
}

// SeedBuiltins seeds built-in tools and MCP servers if they don't exist.
func (p *PostgresStore) SeedBuiltins(ctx context.Context) error {
	for _, t := range allBuiltins() {
		t := t
		existing, err := p.Get(ctx, t.Name)
		if err != nil {
			return fmt.Errorf("failed to check %s: %w", t.Name, err)
		}
		if existing != nil {
			continue
		}
		if err := p.add(ctx, &t); err != nil {
			return fmt.Errorf("failed to seed %s: %w", t.Name, err)
		}
	}
	return nil
}

func (p *PostgresStore) add(ctx context.Context, t *Tool) error {
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

	builtin, enabled := 0, 0
	if t.Builtin {
		builtin = 1
	}
	if t.Enabled {
		enabled = 1
	}
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO tools (name, type, command, install_cmd, upgrade_cmd, version_cmd, transport, url, slash_cmds, mcp_servers, config, builtin, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		t.Name, t.Type, t.Command, t.InstallCmd, t.UpgradeCmd, t.VersionCmd,
		t.Transport, t.URL,
		slashCmds, mcpServers, config, builtin, enabled,
	)
	return err
}

// Add inserts a new tool.
func (p *PostgresStore) Add(ctx context.Context, t *Tool) error {
	if t.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if t.Command == "" {
		return fmt.Errorf("tool command is required")
	}
	existing, err := p.Get(ctx, t.Name)
	if err == nil && existing != nil {
		return fmt.Errorf("tool %q already exists", t.Name)
	}
	return p.add(ctx, t)
}

// pgToolColumns is the shared column list for Get/List so both stay in sync
// with pgScanToolFrom's Scan order.
const pgToolColumns = `name, type, command, install_cmd, upgrade_cmd, version_cmd, transport, url, slash_cmds, mcp_servers, config, health_status, last_checked, builtin, enabled, created_at`

// Get returns a tool by name. Returns nil, nil if not found.
func (p *PostgresStore) Get(ctx context.Context, name string) (*Tool, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT `+pgToolColumns+` FROM tools WHERE name = $1`, name)
	return pgScanToolFrom(row)
}

// pgToolScanner is implemented by both *sql.Row and *sql.Rows.
type pgToolScanner interface {
	Scan(dest ...any) error
}

// pgScanToolFrom scans a Postgres row into a Tool. Returns (nil, nil) for sql.ErrNoRows.
func pgScanToolFrom(sc pgToolScanner) (*Tool, error) {
	var t Tool
	var toolType, installCmd, upgradeCmd, versionCmd, transport, url, slashCmds, mcpServers, config sql.NullString
	var healthStatus, lastChecked sql.NullString
	var createdAt time.Time
	if err := sc.Scan(
		&t.Name, &toolType, &t.Command,
		&installCmd, &upgradeCmd, &versionCmd,
		&transport, &url,
		&slashCmds, &mcpServers, &config,
		&healthStatus, &lastChecked,
		&t.Builtin, &t.Enabled, &createdAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	t.CreatedAt = createdAt
	t.Type = toolType.String
	t.InstallCmd = installCmd.String
	t.UpgradeCmd = upgradeCmd.String
	t.VersionCmd = versionCmd.String
	t.Transport = transport.String
	t.URL = url.String
	t.SlashCmds = unmarshalStrings(slashCmds.String)
	t.MCPServers = unmarshalStrings(mcpServers.String)
	t.Config = unmarshalMap(config.String)
	t.HealthStatus = healthStatus.String
	t.LastChecked = lastChecked.String
	return &t, nil
}

// List returns all tools.
func (p *PostgresStore) List(ctx context.Context) ([]*Tool, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT `+pgToolColumns+` FROM tools ORDER BY builtin DESC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var tools []*Tool
	for rows.Next() {
		t, scanErr := pgScanToolFrom(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		tools = append(tools, t)
	}
	return tools, rows.Err()
}

// Update replaces a tool's mutable fields.
func (p *PostgresStore) Update(ctx context.Context, t *Tool) error {
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

	enabledInt := 0
	if t.Enabled {
		enabledInt = 1
	}
	res, err := p.db.ExecContext(ctx,
		`UPDATE tools SET type=$1, command=$2, install_cmd=$3, upgrade_cmd=$4, version_cmd=$5, transport=$6, url=$7, slash_cmds=$8, mcp_servers=$9, config=$10, enabled=$11
		 WHERE name=$12`,
		t.Type, t.Command, t.InstallCmd, t.UpgradeCmd, t.VersionCmd,
		t.Transport, t.URL,
		slashCmds, mcpServers, config, enabledInt,
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
// a tool without touching its other mutable fields.
func (p *PostgresStore) UpdateHealth(ctx context.Context, name, status, lastChecked string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE tools SET health_status=$1, last_checked=$2 WHERE name=$3`,
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
func (p *PostgresStore) Delete(ctx context.Context, name string) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM tools WHERE name = $1`, name)
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
func (p *PostgresStore) SetEnabled(ctx context.Context, name string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	res, err := p.db.ExecContext(ctx, `UPDATE tools SET enabled=$1 WHERE name=$2`, v, name)
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
