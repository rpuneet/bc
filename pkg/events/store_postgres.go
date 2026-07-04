package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	bcdb "github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/log"
)

// PostgresLog stores events in a Postgres database.
// It implements the EventStore interface.
type PostgresLog struct {
	db *sql.DB
}

// NewPostgresLog creates a PostgresLog from an existing *sql.DB connection.
func NewPostgresLog(db *sql.DB) *PostgresLog {
	return &PostgresLog{db: db}
}

// InitSchema creates the events table in Postgres if it doesn't exist.
func (p *PostgresLog) InitSchema() error {
	ctx := context.Background()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS events (
			id        BIGSERIAL PRIMARY KEY,
			type      TEXT NOT NULL,
			agent     TEXT,
			message   TEXT,
			data      TEXT,
			repo      TEXT DEFAULT '',
			timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE events ADD COLUMN IF NOT EXISTS repo TEXT DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_events_agent     ON events(agent)`,
		`CREATE INDEX IF NOT EXISTS idx_events_repo      ON events(repo)`,
		`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp DESC)`,
	}

	for _, stmt := range stmts {
		if _, err := p.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("postgres events schema: %w", err)
		}
	}
	return nil
}

// Append writes a single event to the database.
func (p *PostgresLog) Append(event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	var dataJSON *string
	if event.Data != nil {
		b, err := json.Marshal(event.Data)
		if err != nil {
			return fmt.Errorf("marshal event data: %w", err)
		}
		s := string(b)
		dataJSON = &s
	}

	_, err := p.db.ExecContext(context.Background(),
		"INSERT INTO events (type, agent, message, data, repo, timestamp) VALUES ($1, $2, $3, $4, $5, $6)",
		string(event.Type),
		pgNilStr(event.Agent),
		pgNilStr(event.Message),
		dataJSON,
		event.Repo,
		event.Timestamp,
	)
	return err
}

// Read returns all events ordered by timestamp.
func (p *PostgresLog) Read() ([]Event, error) {
	rows, err := p.db.QueryContext(context.Background(),
		"SELECT type, agent, message, data, repo, timestamp FROM events ORDER BY id ASC LIMIT 1000",
	)
	if err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return pgScanEventRows(rows)
}

// ReadLast returns the last n events.
func (p *PostgresLog) ReadLast(n int) ([]Event, error) {
	rows, err := p.db.QueryContext(context.Background(),
		"SELECT type, agent, message, data, repo, timestamp FROM events ORDER BY id DESC LIMIT $1", n,
	)
	if err != nil {
		return nil, fmt.Errorf("read last events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	events, err := pgScanEventRows(rows)
	if err != nil {
		return nil, err
	}

	// Reverse so oldest first
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

// ReadByAgent returns events for a specific agent, oldest first.
// The window is the NEWEST DefaultReadLimit events — long-lived agents
// exceed the limit, and returning the oldest window froze derived stats
// like "last active" at whatever the 1000th oldest event was.
func (p *PostgresLog) ReadByAgent(name string) ([]Event, error) {
	rows, err := p.db.QueryContext(context.Background(),
		"SELECT type, agent, message, data, repo, timestamp FROM events WHERE agent = $1 ORDER BY id DESC LIMIT 1000", name,
	)
	if err != nil {
		return nil, fmt.Errorf("read events by agent: %w", err)
	}
	defer func() { _ = rows.Close() }()

	events, err := pgScanEventRows(rows)
	if err != nil {
		return nil, err
	}

	// Reverse so oldest first — callers iterate from the end for "newest".
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

// Close is a no-op — the shared DB is owned by the caller.
func (p *PostgresLog) Close() error {
	return nil
}

// --- helpers ---

func pgScanEventRows(rows *sql.Rows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		var ev Event
		var evType string
		var agent, message, dataJSON, repo *string
		var ts time.Time

		if err := rows.Scan(&evType, &agent, &message, &dataJSON, &repo, &ts); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		ev.Type = EventType(evType)
		if agent != nil {
			ev.Agent = *agent
		}
		if repo != nil {
			ev.Repo = *repo
		}
		if message != nil {
			ev.Message = *message
		}
		if dataJSON != nil && *dataJSON != "" {
			_ = json.Unmarshal([]byte(*dataJSON), &ev.Data) //nolint:errcheck // best-effort
		}
		ev.Timestamp = ts
		events = append(events, ev)
	}
	return events, rows.Err()
}

func pgNilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// OpenLog opens the event log on the given database handle (the single
// global mycel.db, or its TimescaleDB equivalent). The handle is
// borrowed: callers own its lifecycle.
func OpenLog(d *bcdb.DB, driver string) (EventStore, error) {
	if d == nil {
		return nil, fmt.Errorf("events store requires a database handle (nil handle)")
	}
	if driver == "timescale" {
		pg := NewPostgresLog(d.DB)
		if schemaErr := pg.InitSchema(); schemaErr != nil {
			return nil, fmt.Errorf("events store: init timescale schema: %w", schemaErr)
		}
		log.Debug("events store: using TimescaleDB backend")
		return pg, nil
	}

	// SQLite
	return NewSQLiteLog(d)
}
