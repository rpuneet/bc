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
