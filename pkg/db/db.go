// Package db provides unified SQLite database management for bc CLI.
//
// This package consolidates SQLite connection management, ensuring consistent
// configuration across all database operations. It provides:
//
//   - Connection pooling optimized for SQLite's single-writer model
//   - Consistent pragma settings for WAL mode and performance
//   - Automatic directory creation for database files
//   - Graceful shutdown handling
//
// # Usage
//
//	db, err := db.Open("/path/to/database.db")
//	if err != nil {
//	    return err
//	}
//	defer db.Close()
//
// # Configuration
//
// All connections use these settings:
//   - WAL journal mode for better concurrency
//   - Foreign keys enabled
//   - 30 second busy timeout
//   - Single connection pool (SQLite limitation)
//   - Optimized cache and synchronous settings
package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// DefaultBusyTimeout is the default timeout for SQLite busy handling.
// Set to 30s to handle concurrent agent access; SQLite returns as soon as
// the lock is available — this is just the worst-case upper bound.
const DefaultBusyTimeout = 30000 // milliseconds

// DefaultCacheSize is the default SQLite page cache size in KB.
const DefaultCacheSize = 2000

// Config holds database configuration options.
type Config struct {
	// BusyTimeout is the SQLite busy timeout in milliseconds.
	// Default: 30000 (30 seconds)
	BusyTimeout int

	// CacheSize is the SQLite page cache size in KB.
	// Default: 2000 (2MB)
	CacheSize int

	// ReadOnly opens the database in read-only mode.
	ReadOnly bool
}

// DefaultConfig returns the default database configuration.
func DefaultConfig() Config {
	return Config{
		BusyTimeout: DefaultBusyTimeout,
		CacheSize:   DefaultCacheSize,
		ReadOnly:    false,
	}
}

// DB wraps a sql.DB with bc-specific functionality.
type DB struct {
	*sql.DB
	path   string
	config Config
}

// Open opens a SQLite database at the given path with default configuration.
// The directory containing the database file is created if it doesn't exist.
func Open(path string) (*DB, error) {
	return OpenWithConfig(path, DefaultConfig())
}

// OpenWithConfig opens a SQLite database with custom configuration.
func OpenWithConfig(path string, cfg Config) (*DB, error) {
	// Create directory if needed
	if !cfg.ReadOnly {
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	// Build connection string with pragmas
	connStr := buildConnectionString(path, cfg)

	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configure connection pool for SQLite's single-writer model
	// SQLite only allows one writer at a time, so limit connections
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(10 * time.Minute)

	// Apply performance pragmas
	if err := applyPragmas(db, cfg); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply pragmas: %w", err)
	}

	return &DB{
		DB:     db,
		path:   path,
		config: cfg,
	}, nil
}

// Path returns the database file path.
func (d *DB) Path() string {
	return d.path
}

// buildConnectionString constructs the SQLite connection string with pragmas.
func buildConnectionString(path string, cfg Config) string {
	params := fmt.Sprintf("?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=%d",
		cfg.BusyTimeout)

	if cfg.ReadOnly {
		params += "&mode=ro"
	}

	return path + params
}

// applyPragmas applies performance pragmas to the database.
func applyPragmas(db *sql.DB, cfg Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pragmas := fmt.Sprintf(`
		PRAGMA synchronous = NORMAL;
		PRAGMA cache_size = -%d;
		PRAGMA temp_store = MEMORY;
		PRAGMA mmap_size = 268435456;
	`, cfg.CacheSize)

	_, err := db.ExecContext(ctx, pragmas)
	return err
}

// Registry caches one workspace database connection per workspace root.
//
// It is the replacement for the old process-global "shared" connection:
// instead of every store writing into whichever workspace's bc.db the
// daemon happened to launch with, each workspace root gets its own
// connection, opened lazily on first use via OpenWorkspaceDBWithConfig
// (SQLite at BCDBPath(root) by default, or that workspace's own
// TimescaleDB with SQLite fallback).
//
// Lifecycle: connections stay cached for the life of the process even if
// the workspace's services are evicted for idleness — a cached idle
// SQLite handle is cheap (max one conn), and keeping it avoids reopen
// churn plus use-after-close races with other holders of the handle.
// Stores treat the handle as borrowed and never close it; only
// CloseWorkspace / Close tear connections down (process shutdown, or
// tests).
type Registry struct {
	entries map[string]*registryEntry
	mu      sync.Mutex
}

type registryEntry struct {
	db     *DB
	driver string // "sqlite" or "timescale"
}

// NewRegistry creates a new empty database registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]*registryEntry)}
}

// registryKey normalizes a workspace root into a stable map key.
func registryKey(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root %q: %w", root, err)
	}
	return filepath.Clean(abs), nil
}

// Get returns the database connection for the workspace rooted at root,
// opening (and caching) it on first use. cfg is only consulted on the
// first open for a given root; later calls return the cached handle and
// its driver regardless of cfg.
//
// The lock is held across the open so concurrent callers for the same
// root can never race two connections into existence.
func (r *Registry) Get(root string, cfg *StorageSettings) (*DB, string, error) {
	key, err := registryKey(root)
	if err != nil {
		return nil, "", err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if e, ok := r.entries[key]; ok {
		return e.db, e.driver, nil
	}

	sqlDB, driver, err := OpenWorkspaceDBWithConfig(root, cfg)
	if err != nil {
		return nil, "", fmt.Errorf("open workspace db %s: %w", root, err)
	}

	e := &registryEntry{db: &DB{DB: sqlDB}, driver: driver}
	r.entries[key] = e
	return e.db, e.driver, nil
}

// CloseWorkspace closes and evicts the cached connection for root, if
// any. A subsequent Get reopens it.
func (r *Registry) CloseWorkspace(root string) error {
	key, err := registryKey(root)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[key]
	if !ok {
		return nil // not open
	}
	delete(r.entries, key)
	return e.db.Close()
}

// Close closes all cached connections and empties the registry.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var firstErr error
	for key, e := range r.entries {
		if err := e.db.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close workspace db %q: %w", key, err)
		}
	}
	r.entries = make(map[string]*registryEntry)
	return firstErr
}
