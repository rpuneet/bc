// Package db provides unified SQLite database management for the mycel CLI.
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
	"strings"
	"time"
	// The "sqlite3" driver is registered in a build-tagged file:
	//   - sqlite_cgo.go   (cgo builds)   → github.com/mattn/go-sqlite3 (C driver)
	//   - sqlite_nocgo.go (nocgo builds) → modernc.org/sqlite (pure Go)
	// Both register under the name "sqlite3", so sql.Open("sqlite3", …) is
	// identical on every platform. All PRAGMAs are applied post-open via
	// db.Exec (see applyPragmas), which is driver-agnostic — this avoids the
	// divergent DSN pragma syntax between the two drivers.
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

// DB wraps a sql.DB with mycel-specific functionality.
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
	// The path feeds MkdirAll and the SQLite driver — clean it and
	// reject traversal sequences before touching the filesystem.
	path = filepath.Clean(path)
	if strings.Contains(path, "..") {
		return nil, fmt.Errorf("unsafe database path: %s", path)
	}
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

// buildConnectionString constructs the SQLite connection string.
//
// PRAGMAs are intentionally NOT encoded in the DSN: mattn/go-sqlite3 and
// modernc.org/sqlite use incompatible DSN pragma syntax, so we apply them
// post-open via applyPragmas instead (driver-agnostic). The only DSN option
// we still need is read-only mode, which both drivers accept as `mode=ro`.
func buildConnectionString(path string, cfg Config) string {
	if cfg.ReadOnly {
		// The file: URI form is required for the mode query parameter to be
		// honored by both drivers.
		return "file:" + path + "?mode=ro"
	}
	return path
}

// applyPragmas applies connection and performance pragmas to the database.
//
// These run as plain PRAGMA statements (not DSN parameters) so the behavior is
// identical on both the CGO (mattn) and pure-Go (modernc) drivers. With a
// single-connection pool (see OpenWithConfig) these settings persist for the
// lifetime of the handle.
func applyPragmas(db *sql.DB, cfg Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connection-scoped pragmas that are safe on read-only handles too.
	stmts := []string{
		"PRAGMA foreign_keys = ON",
		fmt.Sprintf("PRAGMA busy_timeout = %d", cfg.BusyTimeout),
		"PRAGMA synchronous = NORMAL",
		fmt.Sprintf("PRAGMA cache_size = -%d", cfg.CacheSize),
		"PRAGMA temp_store = MEMORY",
		"PRAGMA mmap_size = 268435456",
	}
	// WAL mutates the database file header, so only set it on writable handles.
	if !cfg.ReadOnly {
		stmts = append(stmts, "PRAGMA journal_mode = WAL")
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}
