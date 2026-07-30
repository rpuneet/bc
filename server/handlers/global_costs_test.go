package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/provider"
)

// newTestCostService builds a source-direct cost.Service over a
// throwaway home dir. Tests seed data by writing Claude Code JSONL
// transcripts under <home>/.claude/projects/ (see seedUsage).
func newTestCostService(t *testing.T) (*cost.Service, string) {
	t.Helper()
	home := t.TempDir()
	svc := cost.NewService(provider.DefaultRegistry, cost.Options{
		Home:      home,
		AgentsDir: filepath.Join(home, "agents"),
	}, nil)
	return svc, home
}

// usageCounter makes every seeded entry unique so the CostReader's
// (session, timestamp, model, tokens) dedup never collapses fixtures.
var usageCounter int

// seedUsage writes one Claude Code JSONL transcript line whose "cwd" is
// the repo the entry is attributed to. Returns the USD cost the claude
// provider prices it at (claude-sonnet-4: $3/M input, $15/M output).
func seedUsage(t *testing.T, home, repo string, inputTokens, outputTokens int64, ts time.Time) float64 {
	t.Helper()
	usageCounter++
	session := fmt.Sprintf("s-%d", usageCounter)
	line := fmt.Sprintf(`{"type":"assistant","sessionId":%q,"timestamp":%q,"cwd":%q,"message":{"model":"claude-sonnet-4-20250514","usage":{"input_tokens":%d,"output_tokens":%d,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		session, ts.UTC().Format(time.RFC3339), repo, inputTokens, outputTokens)

	path := filepath.Join(home, ".claude", "projects", "p", session+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return float64(inputTokens)*3.0/1e6 + float64(outputTokens)*15.0/1e6
}

func approxEq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestGlobalCosts_NilServiceReturns503(t *testing.T) {
	h := NewGlobalCostsHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/global/costs", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503", rec.Code)
	}
}

func TestGlobalCosts_MethodNotAllowed(t *testing.T) {
	svc, _ := newTestCostService(t)
	h := NewGlobalCostsHandler(svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/global/costs", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("code = %d, want 405", rec.Code)
	}
}

func TestGlobalCosts_InvalidGroupBy(t *testing.T) {
	svc, _ := newTestCostService(t)
	h := NewGlobalCostsHandler(svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/global/costs?groupBy=garbage", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

func TestGlobalCosts_InvalidStart(t *testing.T) {
	svc, _ := newTestCostService(t)
	h := NewGlobalCostsHandler(svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/global/costs?start=not-a-date", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

func TestGlobalCosts_GroupByRepo(t *testing.T) {
	svc, home := newTestCostService(t)
	now := time.Now()
	alphaUSD := seedUsage(t, home, "/repos/alpha", 300_000, 20_000, now)
	alphaUSD += seedUsage(t, home, "/repos/alpha", 100_000, 10_000, now.Add(time.Minute))
	betaUSD := seedUsage(t, home, "/repos/beta", 500_000, 40_000, now)
	orphanUSD := seedUsage(t, home, "", 10_000, 1_000, now) // unattributed

	if betaUSD <= alphaUSD || alphaUSD <= orphanUSD {
		t.Fatalf("fixture ordering broken: beta=%v alpha=%v orphan=%v", betaUSD, alphaUSD, orphanUSD)
	}

	h := NewGlobalCostsHandler(svc)
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
	if rep.GroupBy != "repo" {
		t.Errorf("groupBy = %q, want repo", rep.GroupBy)
	}
	if len(rep.Rows) != 3 {
		t.Fatalf("rows = %d, want 3; got %+v", len(rep.Rows), rep.Rows)
	}
	// Highest total first — /repos/beta, labeled by basename.
	if rep.Rows[0].Key != "/repos/beta" || rep.Rows[0].Label != "beta" {
		t.Errorf("first row = %+v, want /repos/beta labeled beta", rep.Rows[0])
	}
	if !approxEq(rep.Rows[0].Total, betaUSD) {
		t.Errorf("first row total = %v, want %v", rep.Rows[0].Total, betaUSD)
	}
	// /repos/alpha second (two entries summed).
	if rep.Rows[1].Key != "/repos/alpha" || !approxEq(rep.Rows[1].Total, alphaUSD) {
		t.Errorf("second row = %+v, want /repos/alpha at %v", rep.Rows[1], alphaUSD)
	}
	// Unattributed last.
	if rep.Rows[2].Key != "unattributed" || rep.Rows[2].Label != "Unattributed" {
		t.Errorf("third row = %+v, want unattributed", rep.Rows[2])
	}
	if !approxEq(rep.Rows[2].Total, orphanUSD) {
		t.Errorf("third row total = %v, want %v", rep.Rows[2].Total, orphanUSD)
	}
}

func TestGlobalCosts_GroupByProject(t *testing.T) {
	svc, home := newTestCostService(t)
	now := time.Now()
	alphaUSD := seedUsage(t, home, "/repos/alpha", 100_000, 5_000, now)
	betaUSD := seedUsage(t, home, "/repos/beta", 600_000, 50_000, now)

	h := NewGlobalCostsHandler(svc)
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
	// Rows sorted by total desc: beta first, keyed by project label.
	if rep.Rows[0].Label != "beta" || !approxEq(rep.Rows[0].Total, betaUSD) {
		t.Errorf("first row = %+v, want beta at %v", rep.Rows[0], betaUSD)
	}
	if rep.Rows[1].Label != "alpha" || !approxEq(rep.Rows[1].Total, alphaUSD) {
		t.Errorf("second row = %+v, want alpha at %v", rep.Rows[1], alphaUSD)
	}
}

func TestGlobalCosts_StartBoundsRespected(t *testing.T) {
	svc, home := newTestCostService(t)
	seedUsage(t, home, "/repos/ws", 100_000, 5_000, time.Now())

	// Start far in the future — no entries should match. Build a proper
	// url.Values so the '+' offset doesn't get decoded as a space.
	future := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	h := NewGlobalCostsHandler(svc)
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
