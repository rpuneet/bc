package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"

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

// shared holds the workspace-wide database connection.
// Set via SetShared() at app startup, used by all stores.
var (
	sharedDB     *sql.DB
	sharedDriver string // "sqlite" or "timescale"
	sharedMu     sync.RWMutex
)

// SetShared sets the shared workspace database connection.
// Call once at startup (in cmd/bcd or CLI init).
func SetShared(db *sql.DB, driver string) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	sharedDB = db
	sharedDriver = driver
}

// Shared returns the shared workspace database connection.
// Returns nil if not set (stores should fall back to opening their own).
func Shared() *sql.DB {
	sharedMu.RLock()
	defer sharedMu.RUnlock()
	return sharedDB
}

// SharedWrapped returns the shared database as a *DB wrapper.
// Returns nil if no shared connection is set.
func SharedWrapped() *DB {
	sharedMu.RLock()
	defer sharedMu.RUnlock()
	if sharedDB == nil {
		return nil
	}
	return &DB{DB: sharedDB}
}

// SharedDriver returns "sqlite" or "timescale".
func SharedDriver() string {
	sharedMu.RLock()
	defer sharedMu.RUnlock()
	return sharedDriver
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
	// Accept both "timescale" and legacy "sql" for backward compatibility
	if cfg != nil && (cfg.Default == "timescale" || cfg.Default == "sql") {
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
	basePath := workspaceRoot
	if cfg != nil && cfg.SQLite.Path != "" {
		basePath = cfg.SQLite.Path
	}
	path := filepath.Join(basePath, ".bc", "bc.db")
	if cfg != nil && cfg.SQLite.Path != "" && cfg.SQLite.Path != ".bc" {
		// If a custom path is set, use it directly
		path = filepath.Join(basePath, "bc.db")
	}
	d, err := Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open sqlite %s: %w", path, err)
	}
	return d.DB, "sqlite", nil
}

// CloseShared closes the shared connection.
func CloseShared() error {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if sharedDB != nil {
		err := sharedDB.Close()
		sharedDB = nil
		return err
	}
	return nil
}
