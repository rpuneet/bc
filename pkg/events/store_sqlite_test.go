package events

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// setupSharedDB creates a temporary SQLite shared database for tests.
func setupSharedDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bc.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetShared(d.DB, "sqlite")
	t.Cleanup(func() {
		db.SetShared(nil, "")
		_ = d.Close()
	})
}

func TestSQLiteLog_AppendAndRead(t *testing.T) {
	setupSharedDB(t)
	log, err := NewSQLiteLog("unused")
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
	setupSharedDB(t)
	log, err := NewSQLiteLog("unused")
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
	setupSharedDB(t)
	log, err := NewSQLiteLog("unused")
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
	setupSharedDB(t)
	log, err := NewSQLiteLog("unused")
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
	setupSharedDB(t)
	log, err := NewSQLiteLog("unused")
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
	setupSharedDB(t)
	log, err := NewSQLiteLog("unused")
	if err != nil {
		t.Fatalf("NewSQLiteLog: %v", err)
	}
	defer func() { _ = log.Close() }()

	// Verify it implements EventStore
	var _ EventStore = log
}
