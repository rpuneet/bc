package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PostgresStore provides Postgres-backed MCP server config storage.
type PostgresStore struct {
	configLookup ConfigLookupFunc // optional fallback for config-only servers
	db           *sql.DB
}

// NewPostgresStore creates a PostgresStore from an existing *sql.DB connection.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// InitSchema creates the MCP tables in Postgres if they don't exist.
func (p *PostgresStore) InitSchema() error {
	ctx := context.Background()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS mcp_servers (
			name        TEXT PRIMARY KEY,
			transport   TEXT NOT NULL DEFAULT 'stdio' CHECK (transport IN ('stdio', 'sse')),
			command     TEXT,
			args        TEXT,
			url         TEXT,
			env         TEXT,
			enabled     BOOLEAN NOT NULL DEFAULT TRUE,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_servers_enabled ON mcp_servers(enabled)`,
	}

	for _, stmt := range stmts {
		if _, err := p.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("postgres mcp schema: %w", err)
		}
	}
	return nil
}

// Close closes the database connection.
// Close is a no-op — the shared DB is owned by the caller.
func (p *PostgresStore) Close() error {
	return nil
}

// Add inserts a new MCP server configuration.
func (p *PostgresStore) Add(cfg *ServerConfig) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}

	argsJSON, err := json.Marshal(cfg.Args)
	if err != nil {
		return fmt.Errorf("marshal args: %w", err)
	}

	envJSON, err := json.Marshal(cfg.Env)
	if err != nil {
		return fmt.Errorf("marshal env: %w", err)
	}

	enabledInt := 0
	if cfg.Enabled {
		enabledInt = 1
	}

	ctx := context.Background()
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO mcp_servers (name, transport, command, args, url, env, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		cfg.Name, cfg.Transport, cfg.Command, string(argsJSON), cfg.URL, string(envJSON), enabledInt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return fmt.Errorf("mcp server %q already exists (use 'mycel mcp remove %s' first)", cfg.Name, cfg.Name)
		}
		return fmt.Errorf("add mcp server %q: %w", cfg.Name, err)
	}
	return nil
}

// Get returns an MCP server config by name.
func (p *PostgresStore) Get(name string) (*ServerConfig, error) {
	ctx := context.Background()
	row := p.db.QueryRowContext(ctx,
		`SELECT name, transport, command, args, url, env, enabled, created_at
		 FROM mcp_servers WHERE name = $1`, name,
	)
	return pgScanMCPInto(row)
}

// List returns all MCP server configurations.
func (p *PostgresStore) List() ([]*ServerConfig, error) {
	ctx := context.Background()
	rows, err := p.db.QueryContext(ctx,
		`SELECT name, transport, command, args, url, env, enabled, created_at
		 FROM mcp_servers ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list mcp servers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var configs []*ServerConfig
	for rows.Next() {
		cfg, scanErr := pgScanMCPInto(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

// Remove deletes an MCP server config by name.
func (p *PostgresStore) Remove(name string) error {
	ctx := context.Background()
	result, err := p.db.ExecContext(ctx, "DELETE FROM mcp_servers WHERE name = $1", name)
	if err != nil {
		return fmt.Errorf("remove mcp server %q: %w", name, err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("mcp server %q not found", name)
	}
	return nil
}

// UpdateEnv replaces the env map for a named MCP server in a single
// transaction. Returns an error if the server doesn't exist.
func (p *PostgresStore) UpdateEnv(name string, env map[string]string) error {
	cleaned := cleanEnv(env)
	envJSON, err := json.Marshal(cleaned)
	if err != nil {
		return fmt.Errorf("marshal env: %w", err)
	}
	ctx := context.Background()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update env tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op if Commit already ran

	result, err := tx.ExecContext(ctx,
		`UPDATE mcp_servers SET env = $1 WHERE name = $2`,
		string(envJSON), name,
	)
	if err != nil {
		return fmt.Errorf("update env on mcp server %q: %w", name, err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("mcp server %q not found", name)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update env: %w", err)
	}
	return nil
}

// SetEnabled enables or disables an MCP server config.
// If the server is not yet in the database but a ConfigLookupFunc is set,
// the full config is resolved and inserted before applying the toggle.
func (p *PostgresStore) SetEnabled(name string, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	ctx := context.Background()
	result, err := p.db.ExecContext(ctx,
		`UPDATE mcp_servers SET enabled = $1 WHERE name = $2`,
		enabledInt, name,
	)
	if err != nil {
		return fmt.Errorf("update mcp server %q: %w", name, err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		// Server not in DB — try config lookup (e.g., unified tool store).
		if p.configLookup != nil {
			if cfg := p.configLookup(name); cfg != nil {
				cfg.Enabled = enabled
				if addErr := p.Add(cfg); addErr != nil {
					return fmt.Errorf("insert config-only mcp server %q: %w", name, addErr)
				}
				return nil
			}
		}
		return fmt.Errorf("mcp server %q not found", name)
	}
	return nil
}

// pgScanMCPInto scans a Postgres row into a ServerConfig.
// Uses the scanner interface defined in store.go.
func pgScanMCPInto(sc scanner) (*ServerConfig, error) {
	var (
		cfg               ServerConfig
		command, url      sql.NullString
		argsJSON, envJSON sql.NullString
		enabled           bool
		createdAt         time.Time
	)

	err := sc.Scan(&cfg.Name, &cfg.Transport, &command, &argsJSON, &url, &envJSON, &enabled, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan mcp server: %w", err)
	}

	cfg.Command = command.String
	cfg.URL = url.String
	cfg.Enabled = enabled
	cfg.CreatedAt = createdAt

	if argsJSON.Valid && argsJSON.String != "" {
		if unmarshalErr := json.Unmarshal([]byte(argsJSON.String), &cfg.Args); unmarshalErr != nil {
			return nil, fmt.Errorf("unmarshal args: %w", unmarshalErr)
		}
	}

	if envJSON.Valid && envJSON.String != "" {
		if unmarshalErr := json.Unmarshal([]byte(envJSON.String), &cfg.Env); unmarshalErr != nil {
			return nil, fmt.Errorf("unmarshal env: %w", unmarshalErr)
		}
	}

	return &cfg, nil
}
