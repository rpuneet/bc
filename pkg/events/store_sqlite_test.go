package events

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// setupSharedDB creates a temporary SQLite shared database for tests.
func setupSharedDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mycel.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestSQLiteLog_AppendAndRead(t *testing.T) {
	log, err := NewSQLiteLog(setupSharedDB(t))
	if err != nil {
		t.Fatalf("NewSQLiteLog: %v", err)
	}
	defer func() { _ = log.Close() }()

	// Append events
	for i, evType := range []EventType{AgentSpawned, WorkStarted, AgentReport} {
		appendErr := log.Append(Event{
			Type:      evType,
			Agent:     "eng-01",
			Message:   "test message",
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		})
		if appendErr != nil {
			t.Fatalf("Append %d: %v", i, appendErr)
		}
	}

	// Read all
	events, err := log.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("Read returned %d events, want 3", len(events))
	}
	if events[0].Type != AgentSpawned {
		t.Errorf("first event type = %q, want %q", events[0].Type, AgentSpawned)
	}
}

func TestSQLiteLog_ReadLast(t *testing.T) {
	log, err := NewSQLiteLog(setupSharedDB(t))
	if err != nil {
		t.Fatalf("NewSQLiteLog: %v", err)
	}
	defer func() { _ = log.Close() }()

	for i := 0; i < 10; i++ {
		_ = log.Append(Event{
			Type:      AgentReport,
			Agent:     "eng-01",
			Message:   "msg",
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	last, err := log.ReadLast(3)
	if err != nil {
		t.Fatalf("ReadLast: %v", err)
	}
	if len(last) != 3 {
		t.Fatalf("ReadLast returned %d, want 3", len(last))
	}
	// Should be in chronological order (oldest first)
	if !last[0].Timestamp.Before(last[2].Timestamp) {
		t.Error("ReadLast should return events in chronological order")
	}
}

func TestSQLiteLog_ReadByAgent(t *testing.T) {
	log, err := NewSQLiteLog(setupSharedDB(t))
	if err != nil {
		t.Fatalf("NewSQLiteLog: %v", err)
	}
	defer func() { _ = log.Close() }()

	_ = log.Append(Event{Type: AgentSpawned, Agent: "eng-01"})
	_ = log.Append(Event{Type: AgentSpawned, Agent: "eng-02"})
	_ = log.Append(Event{Type: AgentReport, Agent: "eng-01"})

	events, err := log.ReadByAgent("eng-01")
	if err != nil {
		t.Fatalf("ReadByAgent: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ReadByAgent returned %d, want 2", len(events))
	}
}

// ReadByAgent must return the NEWEST window when an agent has more events
// than DefaultReadLimit — the old ORDER BY id ASC froze derived stats like
// "last active" at the 1000th oldest event (#3259).
func TestSQLiteLog_ReadByAgent_NewestWindowOldestFirst(t *testing.T) {
	log, err := NewSQLiteLog(setupSharedDB(t))
	if err != nil {
		t.Fatalf("NewSQLiteLog: %v", err)
	}
	defer func() { _ = log.Close() }()

	base := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	total := DefaultReadLimit + 5
	for i := 0; i < total; i++ {
		_ = log.Append(Event{
			Type:      AgentReport,
			Agent:     "eng-01",
			Message:   "msg",
			Timestamp: base.Add(time.Duration(i) * time.Second),
		})
	}

	events, err := log.ReadByAgent("eng-01")
	if err != nil {
		t.Fatalf("ReadByAgent: %v", err)
	}
	if len(events) != DefaultReadLimit {
		t.Fatalf("ReadByAgent returned %d, want %d", len(events), DefaultReadLimit)
	}
	// Oldest-first ordering preserved for callers.
	if !events[0].Timestamp.Before(events[len(events)-1].Timestamp) {
		t.Error("ReadByAgent should return events oldest first")
	}
	// The window must contain the newest event, not the oldest.
	newest := base.Add(time.Duration(total-1) * time.Second)
	if got := events[len(events)-1].Timestamp; !got.Equal(newest) {
		t.Errorf("last event = %v, want newest %v (window must keep recent events)", got, newest)
	}
}

func TestSQLiteLog_EventData(t *testing.T) {
	log, err := NewSQLiteLog(setupSharedDB(t))
	if err != nil {
		t.Fatalf("NewSQLiteLog: %v", err)
	}
	defer func() { _ = log.Close() }()

	_ = log.Append(Event{
		Type:    AgentSpawned,
		Agent:   "eng-01",
		Message: "spawned",
		Data:    map[string]any{"role": "engineer", "count": float64(42)},
	})

	events, _ := log.Read()
	if len(events) != 1 {
		t.Fatal("expected 1 event")
	}
	if events[0].Data["role"] != "engineer" {
		t.Errorf("data.role = %v, want engineer", events[0].Data["role"])
	}
}

func TestSQLiteLog_ImplementsEventStore(t *testing.T) {
	log, err := NewSQLiteLog(setupSharedDB(t))
	if err != nil {
		t.Fatalf("NewSQLiteLog: %v", err)
	}
	defer func() { _ = log.Close() }()

	// Verify it implements EventStore
	var _ EventStore = log
}

// TestEventRepoAttribution covers the repo column added for the single
// global database: explicit Event.Repo round-trips, and when the
// writer doesn't know the repo the store best-effort resolves it from
// the agents table living in the same database.
func TestEventRepoAttribution(t *testing.T) {
	d := setupSharedDB(t)
	log, err := NewSQLiteLog(d)
	if err != nil {
		t.Fatalf("NewSQLiteLog: %v", err)
	}
	defer func() { _ = log.Close() }()

	// Minimal agents table alongside events — in production both live
	// in the one global mycel.db.
	ctx := context.Background()
	if _, execErr := d.ExecContext(ctx,
		`CREATE TABLE agents (name TEXT PRIMARY KEY, repo TEXT NOT NULL DEFAULT '')`); execErr != nil {
		t.Fatalf("create agents table: %v", execErr)
	}
	if _, execErr := d.ExecContext(ctx,
		`INSERT INTO agents (name, repo) VALUES ('alice', '/repos/alpha')`); execErr != nil {
		t.Fatalf("seed agent: %v", execErr)
	}

	// Explicit repo wins.
	if appendErr := log.Append(Event{Type: AgentSpawned, Agent: "alice", Repo: "/repos/explicit"}); appendErr != nil {
		t.Fatalf("append explicit: %v", appendErr)
	}
	// No repo supplied: resolved from the agents table.
	if appendErr := log.Append(Event{Type: AgentReport, Agent: "alice"}); appendErr != nil {
		t.Fatalf("append resolved: %v", appendErr)
	}
	// Unknown agent: repo stays empty.
	if appendErr := log.Append(Event{Type: AgentReport, Agent: "ghost"}); appendErr != nil {
		t.Fatalf("append unknown: %v", appendErr)
	}

	evts, err := log.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(evts) != 3 {
		t.Fatalf("events = %d, want 3", len(evts))
	}
	if evts[0].Repo != "/repos/explicit" {
		t.Errorf("explicit repo = %q, want /repos/explicit", evts[0].Repo)
	}
	if evts[1].Repo != "/repos/alpha" {
		t.Errorf("resolved repo = %q, want /repos/alpha (from agents table)", evts[1].Repo)
	}
	if evts[2].Repo != "" {
		t.Errorf("unknown agent repo = %q, want empty", evts[2].Repo)
	}
}
