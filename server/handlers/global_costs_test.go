package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rpuneet/bc/pkg/cost"
	"github.com/rpuneet/bc/pkg/workspace"
	"github.com/rpuneet/bc/server/handlers"
)

// fakeGlobalStore is an in-memory implementation of handlers.GlobalCostStore.
// Records are plain structs; aggregation is done eagerly per call.
type fakeGlobalStore struct {
	records []fakeRecord
}

type fakeRecord struct {
	ts        time.Time
	workspace string
	project   string
	agent     string
	cost      float64
}

func (f *fakeGlobalStore) add(ws, project, agent string, costUSD float64, ts time.Time) {
	f.records = append(f.records, fakeRecord{
		ts: ts, workspace: ws, project: project, agent: agent, cost: costUSD,
	})
}

func (f *fakeGlobalStore) sum(key func(fakeRecord) string, start, end time.Time) []*cost.GlobalSummary {
	totals := map[string]*cost.GlobalSummary{}
	agents := map[string]map[string]struct{}{}
	for _, r := range f.records {
		if !start.IsZero() && r.ts.Before(start) {
			continue
		}
		if !end.IsZero() && r.ts.After(end) {
			continue
		}
		k := key(r)
		s, ok := totals[k]
		if !ok {
			s = &cost.GlobalSummary{Key: k}
			totals[k] = s
			agents[k] = map[string]struct{}{}
		}
		s.TotalUSD += r.cost
		s.RecordCount++
		agents[k][r.agent] = struct{}{}
	}
	out := make([]*cost.GlobalSummary, 0, len(totals))
	for k, s := range totals {
		s.AgentCount = int64(len(agents[k]))
		out = append(out, s)
		_ = k
	}
	// Deterministic order: highest total first, ties broken by key.
	sortSummaries(out)
	return out
}

func sortSummaries(xs []*cost.GlobalSummary) {
	// simple insertion sort — n is tiny in tests
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0; j-- {
			a, b := xs[j-1], xs[j]
			if a.TotalUSD > b.TotalUSD || (a.TotalUSD == b.TotalUSD && a.Key <= b.Key) {
				break
			}
			xs[j-1], xs[j] = b, a
		}
	}
}

func (f *fakeGlobalStore) SumByWorkspace(_ context.Context, start, end time.Time) ([]*cost.GlobalSummary, error) {
	return f.sum(func(r fakeRecord) string { return r.workspace }, start, end), nil
}

func (f *fakeGlobalStore) SumByProject(_ context.Context, start, end time.Time) ([]*cost.GlobalSummary, error) {
	return f.sum(func(r fakeRecord) string { return r.project }, start, end), nil
}

type fakeRegistry struct{ entries []workspace.RegistryEntry }

func (f fakeRegistry) List() []workspace.RegistryEntry { return f.entries }

func serveGlobalCosts(t *testing.T, store handlers.GlobalCostStore, reg handlers.WorkspaceRegistryLister) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	handlers.NewGlobalCostHandler(store, reg).Register(mux)
	return httptest.NewServer(mux)
}

func decodeResponse(t *testing.T, resp *http.Response) handlers.Response {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var out handlers.Response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

// TestGlobalCosts covers the full GET /api/global/costs surface:
//   - default 30-day window
//   - explicit date range
//   - groupBy=workspace labels from registry
//   - groupBy=project uses the path
//   - bad date & bad groupBy return 400
//   - wrong method returns 405
func TestGlobalCosts(t *testing.T) {
	now := time.Now().UTC()

	store := &fakeGlobalStore{}
	// Workspace "alpha" at /home/u/alpha, "beta" at /home/u/beta
	store.add("alpha", "/home/u/alpha", "a1", 2.50, now.Add(-24*time.Hour))
	store.add("alpha", "/home/u/alpha", "a2", 1.25, now.Add(-48*time.Hour))
	store.add("beta", "/home/u/beta", "b1", 10.00, now.Add(-5*time.Hour))
	// An old record outside the default 30-day window
	store.add("beta", "/home/u/beta", "b2", 999.00, now.Add(-60*24*time.Hour))

	reg := fakeRegistry{entries: []workspace.RegistryEntry{
		{Path: "/home/u/alpha", Name: "alpha", Alias: "a"},
		{Path: "/home/u/beta", Name: "beta"},
	}}

	ts := serveGlobalCosts(t, store, reg)
	defer ts.Close()

	t.Run("default window groupBy=workspace", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/global/costs")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		body := decodeResponse(t, resp)

		if body.GroupBy != "workspace" {
			t.Errorf("groupBy = %q, want workspace", body.GroupBy)
		}
		if len(body.Rows) != 2 {
			t.Fatalf("rows = %d, want 2 (old record should be excluded)", len(body.Rows))
		}
		// Highest total first: beta @ 10.00
		if body.Rows[0].Key != "beta" {
			t.Errorf("rows[0].Key = %q, want beta", body.Rows[0].Key)
		}
		if body.Rows[0].Total != 10.00 {
			t.Errorf("rows[0].Total = %v, want 10.00", body.Rows[0].Total)
		}
		if body.Rows[0].Label != "beta" {
			t.Errorf("rows[0].Label = %q, want beta (from registry)", body.Rows[0].Label)
		}
		if body.Rows[0].AgentCount != 1 {
			t.Errorf("rows[0].AgentCount = %d, want 1", body.Rows[0].AgentCount)
		}
		// alpha sums 3.75 across 2 agents
		if body.Rows[1].Key != "alpha" || body.Rows[1].AgentCount != 2 {
			t.Errorf("rows[1] = %+v, want alpha with 2 agents", body.Rows[1])
		}
		if body.Range.Start == "" || body.Range.End == "" {
			t.Errorf("range not populated: %+v", body.Range)
		}
	})

	t.Run("explicit date range groupBy=project", func(t *testing.T) {
		start := now.Add(-36 * time.Hour).Format(time.RFC3339)
		end := now.Format(time.RFC3339)
		resp, err := http.Get(ts.URL + "/api/global/costs?start=" + start + "&end=" + end + "&groupBy=project")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		body := decodeResponse(t, resp)

		if body.GroupBy != "project" {
			t.Errorf("groupBy = %q, want project", body.GroupBy)
		}
		// Only the -24h alpha record and -5h beta record fall in window.
		if len(body.Rows) != 2 {
			t.Fatalf("rows = %d, want 2", len(body.Rows))
		}
		// Project key is the path
		if body.Rows[0].Key != "/home/u/beta" {
			t.Errorf("rows[0].Key = %q, want /home/u/beta", body.Rows[0].Key)
		}
	})

	t.Run("YYYY-MM-DD date accepted", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/global/costs?start=2020-01-01&groupBy=workspace")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200 for YYYY-MM-DD", resp.StatusCode)
		}
		_ = resp.Body.Close()
	})

	t.Run("bad start date returns 400", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/global/costs?start=not-a-date")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		_ = resp.Body.Close()
	})

	t.Run("bad end date returns 400", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/global/costs?end=13/45/99")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		_ = resp.Body.Close()
	})

	t.Run("invalid groupBy returns 400", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/global/costs?groupBy=model")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		_ = resp.Body.Close()
	})

	t.Run("POST returns 405", func(t *testing.T) {
		resp, err := http.Post(ts.URL+"/api/global/costs", "application/json", nil)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", resp.StatusCode)
		}
		_ = resp.Body.Close()
	})
}

// TestGlobalCostsNilStore confirms the handler returns 503 if misconfigured.
func TestGlobalCostsNilStore(t *testing.T) {
	ts := serveGlobalCosts(t, nil, nil)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/global/costs")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}
