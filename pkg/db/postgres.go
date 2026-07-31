package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // Postgres driver via pgx
)

// DefaultPostgresDSN is the connection string for the mycel-db (TimescaleDB) container.
const DefaultPostgresDSN = "postgres://mycel:mycel@localhost:5432/mycel" //nolint:gosec // not a credential, it's a default local DSN

// PostgresDSN returns the Postgres connection string from DATABASE_URL env var,
// or the default mycel-db DSN if not set.
func PostgresDSN() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	return DefaultPostgresDSN
}

// IsPostgresEnabled returns true if DATABASE_URL is set, indicating Postgres should be used.
func IsPostgresEnabled() bool {
	return os.Getenv("DATABASE_URL") != ""
}

// OpenPostgres opens a connection pool to Postgres using the given DSN.
// The DSN should be a postgres:// URL (e.g. postgres://mycel:mycel@localhost:5432/mycel).
func OpenPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	// Connection pool tuned for multi-agent concurrent access
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres at %s: %w", dsn, err)
	}

	return db, nil
}

// TryOpenPostgres attempts to open a Postgres connection.
// Returns (nil, nil) if DATABASE_URL is not set (Postgres not configured).
// Returns (nil, err) only when DATABASE_URL is set but connection fails.
func TryOpenPostgres() (*sql.DB, error) {
	if !IsPostgresEnabled() {
		return nil, nil
	}
	return OpenPostgres(PostgresDSN())
}
