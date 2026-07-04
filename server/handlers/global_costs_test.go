package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/cost"
)

func newTestGlobalStore(t *testing.T) *cost.Store {
	t.Helper()
	store, err := cost.OpenGlobalStore(filepath.Join(t.TempDir(), "costs.db"))
	if err != nil {
		t.Fatalf("OpenGlobalStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func seedCost(t *testing.T, store *cost.Store, wsID, agentID string, usd float64) {
	t.Helper()
	scoped := store.ScopedTo(wsID)
	if _, err := scoped.Record(context.Background(), agentID, "", "claude", 1, 1, usd); err != nil {
		t.Fatalf("Record: %v", err)
	}
}

func TestGlobalCosts_NilStoreReturns503(t *testing.T) {
	h := NewGlobalCostsHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/global/costs", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503", rec.Code)
	}
}

func TestGlobalCosts_MethodNotAllowed(t *testing.T) {
	h := NewGlobalCostsHandler(newTestGlobalStore(t))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/global/costs", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("code = %d, want 405", rec.Code)
	}
}

func TestGlobalCosts_InvalidGroupBy(t *testing.T) {
	h := NewGlobalCostsHandler(newTestGlobalStore(t))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/global/costs?groupBy=garbage", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

func TestGlobalCosts_InvalidStart(t *testing.T) {
	h := NewGlobalCostsHandler(newTestGlobalStore(t))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/global/costs?start=not-a-date", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

func TestGlobalCosts_GroupByWorkspace(t *testing.T) {
	store := newTestGlobalStore(t)
	seedCost(t, store, "/repos/alpha", "a1", 1.25)
	seedCost(t, store, "/repos/alpha", "a2", 0.50)
	seedCost(t, store, "/repos/beta", "a3", 2.00)
	seedCost(t, store, "", "orphan", 0.10) // unattributed

	h := NewGlobalCostsHandler(store)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/global/costs", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var rep CostReport
	if err := json.NewDecoder(rec.Body).Decode(&rep); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rep.GroupBy != "workspace" {
		t.Errorf("groupBy = %q, want workspace", rep.GroupBy)
	}
	if len(rep.Rows) != 3 {
		t.Fatalf("rows = %d, want 3; got %+v", len(rep.Rows), rep.Rows)
	}
	// Highest total first — /repos/beta at 2.00, labeled by basename.
	if rep.Rows[0].Key != "/repos/beta" || rep.Rows[0].Label != "beta" {
		t.Errorf("first row = %+v, want /repos/beta labeled beta", rep.Rows[0])
	}
	if rep.Rows[0].Total != 2.00 {
		t.Errorf("first row total = %v, want 2.00", rep.Rows[0].Total)
	}
	// /repos/alpha second: 1.25 + 0.50 = 1.75
	if rep.Rows[1].Key != "/repos/alpha" || rep.Rows[1].Total != 1.75 {
		t.Errorf("second row = %+v, want /repos/alpha at 1.75", rep.Rows[1])
	}
	// Unattributed last
	if rep.Rows[2].Key != "unattributed" || rep.Rows[2].Label != "Unattributed" {
		t.Errorf("third row = %+v, want unattributed", rep.Rows[2])
	}
}

func TestGlobalCosts_GroupByProject(t *testing.T) {
	store := newTestGlobalStore(t)
	seedCost(t, store, "/repos/alpha", "a1", 1.00)
	seedCost(t, store, "/repos/beta", "a2", 3.00)

	h := NewGlobalCostsHandler(store)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/global/costs?groupBy=project", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var rep CostReport
	if err := json.NewDecoder(rec.Body).Decode(&rep); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rep.GroupBy != "project" {
		t.Errorf("groupBy = %q, want project", rep.GroupBy)
	}
	if len(rep.Rows) != 2 {
		t.Fatalf("rows = %d, want 2; got %+v", len(rep.Rows), rep.Rows)
	}
	// Rows sorted by total desc: beta first (basename label)
	if rep.Rows[0].Label != "beta" || rep.Rows[0].Total != 3.00 {
		t.Errorf("first row = %+v, want beta at 3.00", rep.Rows[0])
	}
}

func TestGlobalCosts_StartBoundsRespected(t *testing.T) {
	store := newTestGlobalStore(t)
	seedCost(t, store, "ws", "recent", 1.00)

	// Start far in the future — no records should match. Build a proper
	// url.Values so the '+' offset doesn't get decoded as a space.
	future := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	h := NewGlobalCostsHandler(store)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/global/costs", nil)
	q := req.URL.Query()
	q.Set("start", future)
	req.URL.RawQuery = q.Encode()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var rep CostReport
	_ = json.NewDecoder(rec.Body).Decode(&rep)
	if len(rep.Rows) != 0 {
		t.Errorf("rows = %d, want 0 after future start", len(rep.Rows))
	}
}
