package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// SQLiteLog stores events in a SQLite database.
// It implements the EventStore interface.
type SQLiteLog struct {
	db *db.DB
}

// NewSQLiteLog opens the events table on the given workspace database.
// The handle is borrowed: callers (typically the per-workspace db
// registry) own its lifecycle.
func NewSQLiteLog(d *db.DB) (*SQLiteLog, error) {
	if d == nil {
		return nil, fmt.Errorf("events store requires a workspace database (nil handle)")
	}

	schema := `
		CREATE TABLE IF NOT EXISTS events (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			type      TEXT NOT NULL,
			agent     TEXT,
			message   TEXT,
			data      TEXT,
			repo      TEXT DEFAULT '',
			timestamp TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_events_agent ON events(agent);
		CREATE INDEX IF NOT EXISTS idx_events_repo ON events(repo);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp DESC);
	`
	if _, err := d.ExecContext(context.Background(), schema); err != nil {
		return nil, fmt.Errorf("create events table: %w", err)
	}

	return &SQLiteLog{db: d}, nil
}

// Append writes a single event to the database.
func (l *SQLiteLog) Append(event Event) error {
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

	repo := event.Repo
	if repo == "" && event.Agent != "" {
		// Best-effort attribution: events and agents share the single
		// global database, so resolve the writer's repo from the agents
		// table when the caller didn't supply one. Any failure (e.g. no
		// agents table on a bare test handle) just leaves repo empty.
		_ = l.db.QueryRowContext(context.Background(),
			"SELECT repo FROM agents WHERE name = ?", event.Agent).Scan(&repo) //nolint:errcheck // best-effort
	}

	_, err := l.db.ExecContext(context.Background(),
		"INSERT INTO events (type, agent, message, data, repo, timestamp) VALUES (?, ?, ?, ?, ?, ?)",
		string(event.Type),
		nilStr(event.Agent),
		nilStr(event.Message),
		dataJSON,
		repo,
		event.Timestamp.Format(time.RFC3339),
	)
	return err
}

// Read returns all events ordered by timestamp.
func (l *SQLiteLog) Read() ([]Event, error) {
	rows, err := l.db.QueryContext(context.Background(),
		"SELECT type, agent, message, data, repo, timestamp FROM events ORDER BY id ASC LIMIT 1000",
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanEventRows(rows)
}

// ReadLast returns the last n events.
func (l *SQLiteLog) ReadLast(n int) ([]Event, error) {
	rows, err := l.db.QueryContext(context.Background(),
		"SELECT type, agent, message, data, repo, timestamp FROM events ORDER BY id DESC LIMIT ?", n,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	events, err := scanEventRows(rows)
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
func (l *SQLiteLog) ReadByAgent(name string) ([]Event, error) {
	rows, err := l.db.QueryContext(context.Background(),
		"SELECT type, agent, message, data, repo, timestamp FROM events WHERE agent = ? ORDER BY id DESC LIMIT 1000", name,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	events, err := scanEventRows(rows)
	if err != nil {
		return nil, err
	}

	// Reverse so oldest first — callers iterate from the end for "newest".
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

// Prune deletes events older than maxAge and trims per-agent events to maxPerAgent.
func (l *SQLiteLog) Prune(maxAge time.Duration, maxPerAgent int) (int64, error) {
	cutoff := time.Now().Add(-maxAge).Format(time.RFC3339)

	// Phase 1: delete events older than maxAge
	res, err := l.db.ExecContext(context.Background(),
		"DELETE FROM events WHERE timestamp < ?", cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("prune old events: %w", err)
	}
	deleted, _ := res.RowsAffected()

	// Phase 2: trim per-agent events to maxPerAgent (keep newest)
	rows, err := l.db.QueryContext(context.Background(),
		"SELECT agent, COUNT(*) as cnt FROM events WHERE agent IS NOT NULL GROUP BY agent HAVING cnt > ?", maxPerAgent,
	)
	if err != nil {
		return deleted, nil // best-effort
	}
	defer func() { _ = rows.Close() }()

	var agents []string
	for rows.Next() {
		var agent string
		var cnt int
		if scanErr := rows.Scan(&agent, &cnt); scanErr == nil {
			agents = append(agents, agent)
		}
	}
	_ = rows.Err()

	for _, agent := range agents {
		res2, delErr := l.db.ExecContext(context.Background(),
			`DELETE FROM events WHERE id IN (
				SELECT id FROM events WHERE agent = ? ORDER BY id DESC LIMIT -1 OFFSET ?
			)`, agent, maxPerAgent,
		)
		if delErr == nil {
			n, _ := res2.RowsAffected()
			deleted += n
		}
	}

	return deleted, nil
}

// Close is a no-op — the shared DB is owned by the caller.
func (l *SQLiteLog) Close() error {
	return nil
}

// --- helpers ---

// sqlRows is the subset of *sql.Rows used by scanEventRows.
type sqlRows interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func scanEventRows(rows sqlRows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		var ev Event
		var evType string
		var agent, message, dataJSON, repo *string
		var ts string

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
		ev.Timestamp, _ = time.Parse(time.RFC3339, ts)
		events = append(events, ev)
	}
	return events, rows.Err()
}

func nilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
