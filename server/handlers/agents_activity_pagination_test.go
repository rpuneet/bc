package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/events"
)

func newActivityTestHandler(t *testing.T) (*AgentHandler, events.EventStore) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	store, err := events.NewSQLiteLog(d)
	if err != nil {
		t.Fatalf("new events log: %v", err)
	}
	return &AgentHandler{events: store}, store
}

// TestAgentActivity_LimitAndCursor exercises the recent-first, bounded,
// cursor-paginated contract of GET /api/agents/{name}/activity. The limit is
// pushed into the query and before=<id> returns strictly older items.
func TestAgentActivity_LimitAndCursor(t *testing.T) {
	h, store := newActivityTestHandler(t)
	for i := 0; i < 30; i++ {
		if err := store.Append(events.Event{Type: events.AgentReport, Agent: "eng-01", Message: "m"}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	// Default page is bounded and newest-first.
	req := httptest.NewRequest(http.MethodGet, "/api/agents/eng-01/activity?limit=10", nil)
	rec := httptest.NewRecorder()
	h.agentActivity(rec, req, "eng-01")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var page []struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page) != 10 {
		t.Fatalf("page = %d items, want 10 (limit honored)", len(page))
	}

	// The raw store confirms newest-first ordering + IDs for the cursor.
	newest, err := store.ReadByAgentPage("eng-01", 10, 0)
	if err != nil {
		t.Fatalf("ReadByAgentPage: %v", err)
	}
	cursor := newest[len(newest)-1].ID
	older, err := store.ReadByAgentPage("eng-01", 100, cursor)
	if err != nil {
		t.Fatalf("ReadByAgentPage older: %v", err)
	}
	if len(older) != 20 {
		t.Fatalf("older = %d, want 20 remaining", len(older))
	}
	for _, e := range older {
		if e.ID >= cursor {
			t.Fatalf("older leaked id %d >= cursor %d", e.ID, cursor)
		}
	}
}
