// Package mcp provides SQLite-backed storage for MCP server configurations.
package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// Transport represents an MCP server transport type.
type Transport string

const (
	TransportStdio Transport = "stdio"
	TransportSSE   Transport = "sse"
)

// ServerConfig represents an MCP server configuration.
// Env values should use ${secret:NAME} references (resolved at runtime via
// pkg/secret) rather than storing sensitive values directly.
type ServerConfig struct {
	CreatedAt time.Time         `json:"created_at"`
	Env       map[string]string `json:"env,omitempty"`
	Name      string            `json:"name"`
	Transport Transport         `json:"transport"`
	Command   string            `json:"command,omitempty"`
	URL       string            `json:"url,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Enabled   bool              `json:"enabled"`
}

// ConfigLookupFunc resolves a server config by name from an external source
// (e.g., the unified tool store). Returns nil if not found.
type ConfigLookupFunc func(name string) *ServerConfig

// Store provides MCP server config storage backed by SQLite or Postgres.
type Store struct {
	configLookup ConfigLookupFunc // optional fallback for config-only servers
	db           *db.DB
	pg           *PostgresStore // non-nil when using Postgres via OpenStore
	shared       bool           // true when using shared bc.db (don't close on Close())
}

// SetConfigLookup registers a fallback function that resolves server configs
// not yet present in the database (e.g., servers defined only in settings or
// the unified tool store). Used by SetEnabled to auto-insert config-only
// servers on first toggle.
func (s *Store) SetConfigLookup(fn ConfigLookupFunc) {
	s.configLookup = fn
	if s.pg != nil {
		s.pg.configLookup = fn
	}
}

// NewStore creates a new MCP store on the given workspace database. The
// handle is borrowed: callers (typically the per-workspace db registry)
// own its lifecycle.
func NewStore(d *db.DB, driver string) (*Store, error) {
	if d == nil {
		return nil, fmt.Errorf("mcp store requires a workspace database (nil handle)")
	}

	s := &Store{db: d, shared: true}
	if driver == "timescale" {
		// Use PostgresStore for proper $1 placeholder queries.
		pg := NewPostgresStore(d.DB)
		if schemaErr := pg.InitSchema(); schemaErr != nil {
			return nil, fmt.Errorf("init mcp schema on timescale: %w", schemaErr)
		}
		s.pg = pg
	} else {
		if err := s.initSchema(); err != nil {
			return nil, fmt.Errorf("init mcp schema: %w", err)
		}
	}
	return s, nil
}

// initSchema creates the MCP server configs table.
func (s *Store) initSchema() error {
	ctx := context.Background()
	// Use CURRENT_TIMESTAMP — works in both SQLite and Postgres
	// (strftime is SQLite-only and breaks on Postgres)
	schema := `
		CREATE TABLE IF NOT EXISTS mcp_servers (
			name        TEXT PRIMARY KEY,
			transport   TEXT NOT NULL DEFAULT 'stdio' CHECK (transport IN ('stdio', 'sse')),
			command     TEXT,
			args        TEXT,
			url         TEXT,
			env         TEXT,
			enabled     INTEGER NOT NULL DEFAULT 1,
			created_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_mcp_servers_enabled ON mcp_servers(enabled);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// Close closes the database connection.
// No-op when using the borrowed workspace DB — its owner (the
// per-workspace registry) handles closing.
func (s *Store) Close() error {
	if s.pg != nil {
		return s.pg.Close()
	}
	if s.shared {
		return nil // shared DB, don't close
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Add inserts a new MCP server configuration.
func (s *Store) Add(cfg *ServerConfig) error {
	if s.pg != nil {
		return s.pg.Add(cfg)
	}
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

	ctx := context.Background()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO mcp_servers (name, transport, command, args, url, env, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cfg.Name, cfg.Transport, cfg.Command, string(argsJSON), cfg.URL, string(envJSON), cfg.Enabled,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return fmt.Errorf("mcp server %q already exists (use 'mycel mcp remove %s' first)", cfg.Name, cfg.Name)
		}
		return fmt.Errorf("add mcp server %q: %w", cfg.Name, err)
	}
	return nil
}

// Get returns an MCP server config by name.
func (s *Store) Get(name string) (*ServerConfig, error) {
	if s.pg != nil {
		return s.pg.Get(name)
	}
	ctx := context.Background()
	row := s.db.QueryRowContext(ctx,
		`SELECT name, transport, command, args, url, env, enabled, created_at
		 FROM mcp_servers WHERE name = ?`, name,
	)
	return scanInto(row)
}

// List returns all MCP server configurations.
func (s *Store) List() ([]*ServerConfig, error) {
	if s.pg != nil {
		return s.pg.List()
	}
	ctx := context.Background()
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, transport, command, args, url, env, enabled, created_at
		 FROM mcp_servers ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list mcp servers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var configs []*ServerConfig
	for rows.Next() {
		cfg, err := scanInto(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

// Remove deletes an MCP server config by name.
func (s *Store) Remove(name string) error {
	if s.pg != nil {
		return s.pg.Remove(name)
	}
	ctx := context.Background()
	result, err := s.db.ExecContext(ctx, "DELETE FROM mcp_servers WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("remove mcp server %q: %w", name, err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("mcp server %q not found", name)
	}
	return nil
}

// UpdateEnv replaces the env map for a named MCP server. Keys with empty
// values are dropped so the UI can "remove" a variable by clearing it.
// Returns an error if the server doesn't exist (writes never auto-create
// the row — use Add for that).
//
// The mutation runs inside a transaction so a partial failure rolls back
// to the original env (important for the Postgres path, which internally
// replaces the row).
func (s *Store) UpdateEnv(name string, env map[string]string) error {
	if s.pg != nil {
		return s.pg.UpdateEnv(name, env)
	}
	cleaned := cleanEnv(env)
	envJSON, err := json.Marshal(cleaned)
	if err != nil {
		return fmt.Errorf("marshal env: %w", err)
	}
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update env tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op if Commit already ran

	result, err := tx.ExecContext(ctx,
		`UPDATE mcp_servers SET env = ? WHERE name = ?`,
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

// cleanEnv drops entries with empty values — the UI uses "" to mark a
// row for deletion rather than sending an explicit delete.
func cleanEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

// SetEnabled enables or disables an MCP server config.
// If the server is not yet in the database but a ConfigLookupFunc is set,
// the full config is resolved and inserted before applying the toggle.
func (s *Store) SetEnabled(name string, enabled bool) error {
	if s.pg != nil {
		return s.pg.SetEnabled(name, enabled)
	}
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	ctx := context.Background()
	result, err := s.db.ExecContext(ctx,
		`UPDATE mcp_servers SET enabled = ? WHERE name = ?`,
		enabledInt, name,
	)
	if err != nil {
		return fmt.Errorf("update mcp server %q: %w", name, err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		// Server not in DB — try config lookup (e.g., unified tool store).
		if s.configLookup != nil {
			if cfg := s.configLookup(name); cfg != nil {
				cfg.Enabled = enabled
				if addErr := s.Add(cfg); addErr != nil {
					return fmt.Errorf("insert config-only mcp server %q: %w", name, addErr)
				}
				return nil
			}
		}
		return fmt.Errorf("mcp server %q not found", name)
	}
	return nil
}

// scanner is implemented by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanInto scans a row into a ServerConfig. Returns (nil, nil) for sql.ErrNoRows.
func scanInto(sc scanner) (*ServerConfig, error) {
	var (
		cfg               ServerConfig
		command, url      sql.NullString
		argsJSON, envJSON sql.NullString
		enabled           int
		createdAt         string
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
	cfg.Enabled = enabled == 1

	if argsJSON.Valid && argsJSON.String != "" {
		if err := json.Unmarshal([]byte(argsJSON.String), &cfg.Args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
	}

	if envJSON.Valid && envJSON.String != "" {
		if err := json.Unmarshal([]byte(envJSON.String), &cfg.Env); err != nil {
			return nil, fmt.Errorf("unmarshal env: %w", err)
		}
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		cfg.CreatedAt = t
	}

	return &cfg, nil
}

// validateConfig checks that a ServerConfig has valid fields.
func validateConfig(cfg *ServerConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("mcp server name is required")
	}

	switch cfg.Transport {
	case TransportStdio:
		if cfg.Command == "" {
			return fmt.Errorf("command is required for stdio transport")
		}
	case TransportSSE:
		if cfg.URL == "" {
			return fmt.Errorf("url is required for sse transport")
		}
	default:
		return fmt.Errorf("invalid transport %q (must be 'stdio' or 'sse')", cfg.Transport)
	}

	return nil
}
