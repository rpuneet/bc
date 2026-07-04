package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/rpuneet/mycel/pkg/log"
)

// DefaultPassword returns the database password from BC_DB_PASSWORD env var,
// falling back to "bc" for local development with a warning log.
// Production deployments should always set BC_DB_PASSWORD.
func DefaultPassword() string {
	if pw := os.Getenv("BC_DB_PASSWORD"); pw != "" {
		return pw
	}
	log.Warn("BC_DB_PASSWORD not set — using default password (not suitable for production)")
	return "bc"
}

// BCDBPath returns the path to the unified bc database for a workspace.
func BCDBPath(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".bc", "bc.db")
}

// defaultRegistry is the process-wide per-workspace connection registry.
// Unlike the old single "shared" connection (which pinned every store to
// the LAUNCH workspace's bc.db), the registry keys connections by
// workspace root, so workspace B's stores can never write into
// workspace A's database.
var defaultRegistry = NewRegistry()

// ForWorkspace returns the (lazily opened, cached) database connection
// and driver ("sqlite" or "timescale") for the workspace rooted at root.
// cfg is only consulted on the first open for that root; pass nil to use
// DATABASE_URL / SQLite defaults.
func ForWorkspace(root string, cfg *StorageSettings) (*DB, string, error) {
	return defaultRegistry.Get(root, cfg)
}

// CloseWorkspaceDB closes and evicts the cached connection for root from
// the default registry. A later ForWorkspace reopens it.
func CloseWorkspaceDB(root string) error {
	return defaultRegistry.CloseWorkspace(root)
}

// CloseAllWorkspaceDBs closes every connection in the default registry.
// Call at process shutdown.
func CloseAllWorkspaceDBs() error {
	return defaultRegistry.Close()
}

// StorageSettings holds the storage configuration from settings.json.
// Used by OpenWorkspaceDB to determine the database backend.
type StorageSettings struct {
	Default   string // "sqlite" or "timescale"
	SQLite    SQLiteSettings
	Timescale TimescaleSettings
}

// SQLiteSettings configures the SQLite database path.
type SQLiteSettings struct {
	Path string // base directory for bc.db (default: workspace .bc/ dir)
}

// TimescaleSettings configures the TimescaleDB (Postgres) connection.
type TimescaleSettings struct {
	Host     string
	User     string
	Password string
	Database string
	Port     int
}

// DSN builds a Postgres connection string from config fields.
func (p TimescaleSettings) DSN() string {
	host := p.Host
	if host == "" {
		host = "localhost"
	}
	port := p.Port
	if port == 0 {
		port = 5432
	}
	user := p.User
	if user == "" {
		user = "bc"
	}
	pw := p.Password
	if pw == "" {
		pw = DefaultPassword()
	}
	db := p.Database
	if db == "" {
		db = "bc"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s", user, url.PathEscape(pw), host, port, db)
}

// OpenWorkspaceDB opens the workspace database based on configuration.
// Priority: DATABASE_URL env var > settings.json storage config > SQLite default.
func OpenWorkspaceDB(workspaceRoot string) (*sql.DB, string, error) {
	return OpenWorkspaceDBWithConfig(workspaceRoot, nil)
}

// OpenWorkspaceDBWithConfig opens the workspace database using settings.json config.
// If DATABASE_URL env var is set, it takes priority (for Docker/CI).
// Otherwise, settings.json storage.default determines the backend.
func OpenWorkspaceDBWithConfig(workspaceRoot string, cfg *StorageSettings) (*sql.DB, string, error) {
	// Priority 1: DATABASE_URL env var (Docker/CI override)
	if IsPostgresEnabled() {
		db, err := OpenPostgres(PostgresDSN())
		if err != nil {
			return nil, "", fmt.Errorf("open timescale: %w", err)
		}
		return db, "timescale", nil
	}

	// Priority 2: settings.json storage config
	if cfg != nil && cfg.Default == "timescale" {
		dsn := cfg.Timescale.DSN()
		db, err := OpenPostgres(dsn)
		if err == nil {
			return db, "timescale", nil
		}
		// A dead TimescaleDB must not take every store down with it —
		// nil stores mean notifications, cron, MCP, tools, and events all
		// silently vanish. Fall back to SQLite and keep the daemon usable;
		// data written during the fallback stays in SQLite and does not
		// sync back once TimescaleDB returns.
		log.Warn("configured timescale database unreachable — falling back to sqlite",
			"error", err)
	}

	// Priority 3: SQLite (default)
	//
	// The default settings.json writes sqlite.path = ".bc", which used to
	// resolve CWD-relative to ".bc/.bc/bc.db" (issue #3237). The default
	// always means the canonical <workspaceRoot>/.bc/bc.db; custom paths
	// resolve against the workspace root — never the process CWD.
	path := BCDBPath(workspaceRoot)
	if cfg != nil && cfg.SQLite.Path != "" && cfg.SQLite.Path != ".bc" {
		base := cfg.SQLite.Path
		if !filepath.IsAbs(base) {
			base = filepath.Join(workspaceRoot, base)
		}
		path = filepath.Join(base, "bc.db")
	}
	d, err := Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open sqlite %s: %w", path, err)
	}
	return d.DB, "sqlite", nil
}
