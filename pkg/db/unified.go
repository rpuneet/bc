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

// GlobalDBFileName is the file name of the single global database.
const GlobalDBFileName = "mycel.db"

// DefaultPassword returns the database password from MYCEL_DB_PASSWORD env var,
// falling back to "bc" for local development with a warning log.
// Production deployments should always set MYCEL_DB_PASSWORD.
func DefaultPassword() string {
	if pw := os.Getenv("MYCEL_DB_PASSWORD"); pw != "" {
		return pw
	}
	log.Warn("MYCEL_DB_PASSWORD not set — using default password (not suitable for production)")
	return "bc"
}

// mycelHome resolves the global mycel home directory: the MYCEL_HOME
// env var when set (tests, containers), otherwise ~/.mycel. Kept local
// to pkg/db to avoid an import cycle with pkg/workspace, which imports
// this package for StorageSettings.
func mycelHome() (string, error) {
	if env := os.Getenv("MYCEL_HOME"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".mycel"), nil
}

// GlobalDBPath returns the path of the single global database file:
// <MycelHome>/mycel.db. Every store — agents, events, notify,
// mcp, tools, roles — lives in this one database; isolation between
// repos comes from data keys (agent name, repo path), not from
// separate files.
func GlobalDBPath() (string, error) {
	home, err := mycelHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, GlobalDBFileName), nil
}

// globalConns caches the process-wide database handle(s), keyed by the
// resolved mycel.db path. In production this holds exactly one entry —
// THE global mycel.db — but keying by path keeps tests that point
// MYCEL_HOME at different temp dirs isolated from each other.
//
// Lifecycle: the handle stays cached for the life of the process even
// if a workspace's services are evicted for idleness — a cached idle
// SQLite handle is cheap (max one conn), and keeping it avoids reopen
// churn plus use-after-close races with other holders. Stores treat
// the handle as borrowed and never close it; only CloseGlobal tears it
// down (process shutdown, or tests).
var globalConns = struct {
	entries map[string]*globalEntry
	mu      sync.Mutex
}{entries: make(map[string]*globalEntry)}

type globalEntry struct {
	db     *DB
	driver string // "sqlite" or "timescale"
}

// Global returns the process-wide database handle and driver ("sqlite"
// or "timescale"), opening <MycelHome>/mycel.db lazily on first use.
//
// cfg selects the backend (DATABASE_URL env > cfg timescale > SQLite
// at GlobalDBPath) and is only consulted on the first open; later
// calls return the cached handle regardless of cfg. Pass nil to use
// DATABASE_URL / SQLite defaults.
//
// The lock is held across the open so concurrent callers can never
// race two connections into existence.
func Global(cfg *StorageSettings) (*DB, string, error) {
	path, err := GlobalDBPath()
	if err != nil {
		return nil, "", err
	}

	globalConns.mu.Lock()
	defer globalConns.mu.Unlock()

	if e, ok := globalConns.entries[path]; ok {
		return e.db, e.driver, nil
	}

	sqlDB, driver, err := OpenGlobalDBWithConfig(path, cfg)
	if err != nil {
		return nil, "", fmt.Errorf("open global db %s: %w", path, err)
	}

	e := &globalEntry{db: &DB{DB: sqlDB, path: path}, driver: driver}
	globalConns.entries[path] = e
	return e.db, e.driver, nil
}

// CloseGlobal closes every cached global connection and empties the
// cache. Call at process shutdown; a subsequent Global reopens.
func CloseGlobal() error {
	globalConns.mu.Lock()
	defer globalConns.mu.Unlock()

	var firstErr error
	for key, e := range globalConns.entries {
		if err := e.db.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close global db %q: %w", key, err)
		}
	}
	globalConns.entries = make(map[string]*globalEntry)
	return firstErr
}

// StorageSettings holds the storage configuration from settings.json.
// Used by Global / OpenGlobalDBWithConfig to determine the database
// backend.
type StorageSettings struct {
	Default   string // "sqlite" or "timescale"
	SQLite    SQLiteSettings
	Timescale TimescaleSettings
}

// SQLiteSettings configures the SQLite database path.
//
// NOTE: with the single global mycel.db the per-workspace SQLite path
// override is ignored — the database always lives at
// <MycelHome>/mycel.db. The field is kept so existing settings.json
// files still parse and map through DBStorageSettings.
type SQLiteSettings struct {
	Path string
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

// OpenGlobalDBWithConfig opens the global database at sqlitePath using
// the given storage settings to choose the backend.
// Priority: DATABASE_URL env var (Docker/CI) > cfg timescale > SQLite
// at sqlitePath.
//
// Most callers should use Global instead; this is exported so the
// backend-choice logic is testable without touching the process-wide
// singleton.
func OpenGlobalDBWithConfig(sqlitePath string, cfg *StorageSettings) (*sql.DB, string, error) {
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
		// nil stores mean notifications, MCP, tools, and events all
		// silently vanish. Fall back to SQLite and keep the daemon usable;
		// data written during the fallback stays in SQLite and does not
		// sync back once TimescaleDB returns.
		log.Warn("configured timescale database unreachable — falling back to sqlite",
			"error", err)
	}

	// Priority 3: SQLite (default). The single global database always
	// lives at sqlitePath (<MycelHome>/mycel.db for Global callers);
	// the legacy per-workspace sqlite.path override is intentionally
	// ignored — one process, one file.
	d, err := Open(sqlitePath)
	if err != nil {
		return nil, "", fmt.Errorf("open sqlite %s: %w", sqlitePath, err)
	}
	return d.DB, "sqlite", nil
}
