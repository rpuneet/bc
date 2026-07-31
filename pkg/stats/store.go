package stats

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // Postgres driver via pgx

	"github.com/rpuneet/mycel/pkg/log"
)

// DefaultStatsDSN is the connection string for the unified mycel-db TimescaleDB container.
const DefaultStatsDSN = "postgres://mycel:mycel@localhost:5432/mycel" //nolint:gosec // not a credential, it's a default DSN

// StatsDSN returns the TimescaleDB connection string.
func StatsDSN() string {
	if dsn := os.Getenv("STATS_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return DefaultStatsDSN
}

// Store provides time-series metrics storage backed by TimescaleDB.
type Store struct {
	db *sql.DB
}

// NewStore connects to TimescaleDB and ensures schema exists.
func NewStore(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open timescaledb: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping timescaledb at %s: %w", dsn, err)
	}

	s := &Store{db: db}
	if err := s.ensureSchema(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}

	return s, nil
}

// Close closes the database connection pool.
func (s *Store) Close() error { return s.db.Close() }

// DB returns the underlying database connection.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) ensureSchema(ctx context.Context) error {
	// Detect and migrate stale schemas from older init.sql versions.
	// If a required column is missing, drop and recreate the table.
	// Metrics are ephemeral (7-day retention) so data loss is acceptable.
	if err := s.migrateStaleSchemas(ctx); err != nil {
		return fmt.Errorf("migrate stale schemas: %w", err)
	}

	stmts := []string{
		// System metrics — mycel-daemon, mycel-db containers
		`CREATE TABLE IF NOT EXISTS system_metrics (
			time            TIMESTAMPTZ NOT NULL,
			system_name     TEXT NOT NULL,
			cpu_percent     DOUBLE PRECISION NOT NULL DEFAULT 0,
			mem_used_bytes  BIGINT NOT NULL DEFAULT 0,
			mem_limit_bytes BIGINT NOT NULL DEFAULT 0,
			mem_percent     DOUBLE PRECISION NOT NULL DEFAULT 0,
			net_rx_bytes    BIGINT NOT NULL DEFAULT 0,
			net_tx_bytes    BIGINT NOT NULL DEFAULT 0,
			disk_read_bytes BIGINT NOT NULL DEFAULT 0,
			disk_write_bytes BIGINT NOT NULL DEFAULT 0
		)`,
		// Agent metrics — per-agent container stats
		`CREATE TABLE IF NOT EXISTS agent_metrics (
			time            TIMESTAMPTZ NOT NULL,
			agent_name      TEXT NOT NULL,
			role            TEXT NOT NULL DEFAULT '',
			tool            TEXT NOT NULL DEFAULT '',
			runtime         TEXT NOT NULL DEFAULT 'docker',
			state           TEXT NOT NULL DEFAULT '',
			cpu_percent     DOUBLE PRECISION NOT NULL DEFAULT 0,
			mem_used_bytes  BIGINT NOT NULL DEFAULT 0,
			mem_limit_bytes BIGINT NOT NULL DEFAULT 0,
			mem_percent     DOUBLE PRECISION NOT NULL DEFAULT 0,
			net_rx_bytes    BIGINT NOT NULL DEFAULT 0,
			net_tx_bytes    BIGINT NOT NULL DEFAULT 0,
			disk_read_bytes BIGINT NOT NULL DEFAULT 0,
			disk_write_bytes BIGINT NOT NULL DEFAULT 0
		)`,
		// Channel metrics — message/member/reaction counts
		`CREATE TABLE IF NOT EXISTS channel_metrics (
			time           TIMESTAMPTZ NOT NULL,
			channel_name   TEXT NOT NULL,
			message_count  BIGINT NOT NULL DEFAULT 0,
			member_count   INT NOT NULL DEFAULT 0,
			reaction_count BIGINT NOT NULL DEFAULT 0
		)`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec schema: %w", err)
		}
	}

	hypertables := []string{
		`SELECT create_hypertable('system_metrics', 'time', if_not_exists => TRUE)`,
		`SELECT create_hypertable('agent_metrics', 'time', if_not_exists => TRUE)`,
		`SELECT create_hypertable('channel_metrics', 'time', if_not_exists => TRUE)`,
	}

	for _, stmt := range hypertables {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create hypertable: %w", err)
		}
	}

	return nil
}

// TimeRange specifies a query window and aggregation interval.
type TimeRange struct {
	From     time.Time
	To       time.Time
	Interval string // e.g. "5m", "1h" — converted to Postgres interval via PGInterval()
}

// PGInterval converts short notation to Postgres interval format.
// Uses an allowlist to prevent SQL injection via the interval query parameter.
func (tr TimeRange) PGInterval() string {
	switch tr.Interval {
	case "1s", "5s", "10s", "30s":
		return tr.Interval[:len(tr.Interval)-1] + " seconds"
	case "1m", "5m", "10m", "15m", "30m":
		return tr.Interval[:len(tr.Interval)-1] + " minutes"
	case "1h", "6h", "12h":
		return tr.Interval[:len(tr.Interval)-1] + " hours"
	case "1d", "7d", "30d":
		return tr.Interval[:len(tr.Interval)-1] + " days"
	default:
		return "5 minutes"
	}
}

// ── Types ──────────────────────────────────────────────────────────────────────

// SystemMetric represents a system container resource sample.
type SystemMetric struct {
	Time           time.Time `json:"time"`
	SystemName     string    `json:"system_name"`
	CPUPercent     float64   `json:"cpu_percent"`
	MemUsedBytes   int64     `json:"mem_used_bytes"`
	MemLimitBytes  int64     `json:"mem_limit_bytes"`
	MemPercent     float64   `json:"mem_percent"`
	NetRxBytes     int64     `json:"net_rx_bytes"`
	NetTxBytes     int64     `json:"net_tx_bytes"`
	DiskReadBytes  int64     `json:"disk_read_bytes"`
	DiskWriteBytes int64     `json:"disk_write_bytes"`
}

// AgentMetric represents an agent container resource sample.
type AgentMetric struct {
	Time           time.Time `json:"time"`
	AgentName      string    `json:"agent_name"`
	Role           string    `json:"role"`
	Tool           string    `json:"tool"`
	Runtime        string    `json:"runtime"`
	State          string    `json:"state"`
	CPUPercent     float64   `json:"cpu_percent"`
	MemUsedBytes   int64     `json:"mem_used_bytes"`
	MemLimitBytes  int64     `json:"mem_limit_bytes"`
	MemPercent     float64   `json:"mem_percent"`
	NetRxBytes     int64     `json:"net_rx_bytes"`
	NetTxBytes     int64     `json:"net_tx_bytes"`
	DiskReadBytes  int64     `json:"disk_read_bytes"`
	DiskWriteBytes int64     `json:"disk_write_bytes"`
}

// ChannelMetric represents channel activity at a point in time.
type ChannelMetric struct {
	Time          time.Time `json:"time"`
	ChannelName   string    `json:"channel_name"`
	MessageCount  int64     `json:"message_count"`
	MemberCount   int       `json:"member_count"`
	ReactionCount int64     `json:"reaction_count"`
}

// migrateStaleSchemas detects tables created by an older init.sql and
// drops them so ensureSchema can recreate with the correct columns.
// Each entry checks for a required column that only exists in the new schema.
func (s *Store) migrateStaleSchemas(ctx context.Context) error {
	// Map: table → column that must exist in the new schema.
	checks := []struct {
		table  string
		column string
	}{
		{"system_metrics", "system_name"},
		{"agent_metrics", "tool"},
		{"channel_metrics", "message_count"},
	}

	for _, c := range checks {
		var exists bool
		err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = $1 AND column_name = $2
			)`, c.table, c.column).Scan(&exists)
		if err != nil {
			// Table may not exist yet — that's fine, ensureSchema will create it.
			continue
		}
		if !exists {
			// Old schema detected — drop and let ensureSchema recreate.
			log.Warn("stats: migrating stale schema", "table", c.table, "missing_column", c.column)
			if _, dropErr := s.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", c.table)); dropErr != nil {
				return fmt.Errorf("drop stale table %s: %w", c.table, dropErr)
			}
		}
	}
	return nil
}
