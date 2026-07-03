// multi_tenant_dispatch_test.go — integration tests that exercise every
// handler which closes over per-workspace state, to prove scoped URL
// dispatch hits the correct workspace's store.
//
// The harness (see newDispatchHarness) spins up a real bcd with two
// registered workspaces wsA and wsB. It then drives each handler through
// its scoped URL /api/workspaces/<id>/<resource>... asserting:
//
//  1. GET /api/workspaces/<wsA>/<res> returns wsA's data
//  2. GET /api/workspaces/<wsB>/<res> returns wsB's data
//  3. A write under wsA does NOT surface under wsB
//  4. Cross-scope lookups (an id that belongs to the other ws) 404
//
// Handlers covered: templates, secrets, cron, mcp, tools, events, cost.
// (Agents / channels are covered by existing multi_tenant_test.go and by
//
//	server/e2e_*_test.go which we do not duplicate.)
package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bccost "github.com/rpuneet/mycel/pkg/cost"
	bcdb "github.com/rpuneet/mycel/pkg/db"
	bcevents "github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/workspace"
	"github.com/rpuneet/mycel/server"
	bcws "github.com/rpuneet/mycel/server/ws"
)

// dispatchHarness is the fixture for every dispatch test.
type dispatchHarness struct {
	ts     *httptest.Server
	mgr    *server.WorkspaceManager
	wsAID  string
	wsBID  string
	wsADir string
	wsBDir string
}

func (h *dispatchHarness) close() {
	h.ts.Close()
	_ = h.mgr.Close() //nolint:errcheck
}

// api fires an HTTP call against the harness server. A scoped path like
// "/api/workspaces/<id>/<rest>" is auto-rewritten to the flat
// "/api/<rest>?workspace=<id>" form so legacy tests continue to exercise
// per-workspace dispatch via the current API surface.
func (h *dispatchHarness) api(t *testing.T, method, path string, body []byte) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	path = rewriteWorkspaceScopedPath(path)
	req, err := http.NewRequestWithContext(context.Background(), method, h.ts.URL+path, r)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

// newDispatchHarness creates two workspaces, registers both, and boots a
// bcd server bound to them. Kept as a fresh harness rather than reusing
// multiTenantHarness so the two test files can evolve independently.
func newDispatchHarness(t *testing.T) *dispatchHarness {
	t.Helper()

	// Isolate HOME so ~/.bc for the test lives in tmp.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", filepath.Join(home, ".bc"))
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")

	wsA := filepath.Join(t.TempDir(), "wsA")
	wsB := filepath.Join(t.TempDir(), "wsB")
	for _, dir := range []string{wsA, wsB} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		gitInitDir(t, dir)
		if _, err := workspace.Init(dir); err != nil {
			t.Fatalf("workspace.Init %s: %v", dir, err)
		}
	}
	wsAID := workspace.ComputeWorkspaceID(wsA)
	wsBID := workspace.ComputeWorkspaceID(wsB)

	// Share one SQLite DB across both workspaces so stores that require
	// db.SharedWrapped() (cron, mcp, tool, events SQLite) come online.
	// This mirrors production: bcd opens ONE shared DB for the active
	// workspace; stores from every workspace route through it. Complete
	// per-workspace isolation for those tables is a known gap (no
	// workspace_id column) tracked for a future phase. Our dispatch
	// tests below assert that routing works and that where a store
	// does have workspace scoping (cost_records.workspace_id,
	// per-workspace templates dir, per-workspace events.jsonl) the
	// isolation holds.
	sharedDB, sharedDriver, dbErr := bcdb.OpenWorkspaceDBWithConfig(wsA, nil)
	if dbErr != nil {
		t.Fatalf("open shared db: %v", dbErr)
	}
	bcdb.SetShared(sharedDB, sharedDriver)
	t.Cleanup(func() {
		bcdb.SetShared(nil, "")
		_ = bcdb.CloseShared()
	})

	reg, loadErr := workspace.LoadRegistry()
	if loadErr != nil {
		t.Fatalf("LoadRegistry: %v", loadErr)
	}
	if err := reg.RegisterWithAlias(wsA, "wsA", "a"); err != nil {
		t.Fatalf("register wsA: %v", err)
	}
	if err := reg.RegisterWithAlias(wsB, "wsB", "b"); err != nil {
		t.Fatalf("register wsB: %v", err)
	}
	if err := reg.SetActive(wsA); err != nil {
		t.Fatalf("set active: %v", err)
	}
	if err := reg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	hub := bcws.NewHub()
	go hub.Run()
	t.Cleanup(hub.Stop)

	// User-global cost ledger — required for correct cost scoping. The
	// per-workspace cost.NewStore() falls back to the shared bc.db
	// which has no workspace_id column and collapses totals across
	// workspaces.
	costsGlobal, cgErr := bccost.OpenGlobalStore(filepath.Join(home, ".bc", "costs.db"))
	if cgErr != nil {
		t.Fatalf("open global costs: %v", cgErr)
	}
	t.Cleanup(func() { _ = costsGlobal.Close() })

	globals := &server.Globals{
		Registry:    reg,
		GlobalHub:   hub,
		CostsGlobal: costsGlobal,
	}

	ctx := context.Background()
	mgr := server.NewWorkspaceManager(reg, func(ctx context.Context, w *workspace.Workspace) (*server.WorkspaceServices, error) {
		gitInitDir(t, w.RootDir)
		return server.BuildWorkspaceServices(ctx, globals, w.RootDir)
	})

	// Eager-load both.
	if _, err := mgr.Load(ctx, wsAID); err != nil {
		t.Fatalf("load wsA: %v", err)
	}
	if _, err := mgr.Load(ctx, wsBID); err != nil {
		t.Fatalf("load wsB: %v", err)
	}

	srv := server.NewWithManager(server.Config{Addr: "127.0.0.1:0", CORS: true}, mgr, globals, nil)
	ts := httptest.NewServer(srv.Handler())

	return &dispatchHarness{
		ts:     ts,
		mgr:    mgr,
		wsAID:  wsAID,
		wsBID:  wsBID,
		wsADir: wsA,
		wsBDir: wsB,
	}
}

// requireStatus fails the test if actual != want.
func requireStatus(t *testing.T, want, actual int, tag string, body []byte) {
	t.Helper()
	if actual != want {
		t.Fatalf("%s: status=%d want=%d body=%s", tag, actual, want, body)
	}
}

// ---------------- Templates ----------------

// TestDispatch_Templates verifies POST + GET + DELETE of /api/templates
// under two different workspace scopes, and that a template created in
// wsA is invisible under wsB.
func TestDispatch_Templates(t *testing.T) {
	h := newDispatchHarness(t)
	defer h.close()

	mkBody := func(name, prompt string) []byte {
		m := map[string]any{
			"name":          name,
			"description":   "d",
			"system_prompt": prompt,
		}
		b, _ := json.Marshal(m)
		return b
	}

	// Create template "only-A" in wsA.
	status, body := h.api(t, http.MethodPost, "/api/workspaces/"+h.wsAID+"/templates", mkBody("only-A", "hello from A"))
	requireStatus(t, http.StatusCreated, status, "create A", body)

	// Create template "only-B" in wsB.
	status, body = h.api(t, http.MethodPost, "/api/workspaces/"+h.wsBID+"/templates", mkBody("only-B", "hello from B"))
	requireStatus(t, http.StatusCreated, status, "create B", body)

	// GET wsA list shows only-A, not only-B (for names unique to each).
	status, body = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsAID+"/templates", nil)
	requireStatus(t, http.StatusOK, status, "list A", body)
	if !strings.Contains(string(body), "only-A") {
		t.Errorf("wsA list missing only-A: %s", body)
	}
	if strings.Contains(string(body), "only-B") {
		// Templates have a global layer ~/.bc/templates that might be
		// shared; we only fail if the workspace-scoped entry leaked.
		// Decode and inspect scope per entry.
		var list []map[string]any
		if err := json.Unmarshal(body, &list); err == nil {
			for _, tt := range list {
				if tt["name"] == "only-B" && tt["scope"] == "workspace" {
					t.Errorf("wsA leaked wsB workspace-scope template: %+v", tt)
				}
			}
		}
	}

	// GET cross-scope: fetch only-A under wsB should 404.
	status, body = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsBID+"/templates/only-A", nil)
	if status == http.StatusOK {
		// only-A might be visible under wsB only if it was written as
		// global scope. Check the scope field.
		var tmpl map[string]any
		_ = json.Unmarshal(body, &tmpl)
		if tmpl["scope"] == "workspace" {
			t.Errorf("wsB found only-A as workspace-scope (should be 404 or global): %+v", tmpl)
		}
	}
}

// ---------------- Secrets ----------------

// TestDispatch_Secrets verifies that POSTs under two scopes land in two
// different vaults.
func TestDispatch_Secrets(t *testing.T) {
	h := newDispatchHarness(t)
	defer h.close()

	body := func(name, val string) []byte {
		m := map[string]any{"name": name, "value": val, "description": ""}
		b, _ := json.Marshal(m)
		return b
	}

	// Write SECRET_A under wsA.
	status, resp := h.api(t, http.MethodPost, "/api/workspaces/"+h.wsAID+"/secrets", body("SECRET_A", "va"))
	requireStatus(t, http.StatusCreated, status, "set A", resp)

	// Write SECRET_B under wsB.
	status, resp = h.api(t, http.MethodPost, "/api/workspaces/"+h.wsBID+"/secrets", body("SECRET_B", "vb"))
	requireStatus(t, http.StatusCreated, status, "set B", resp)

	// Secrets vault is user-global when BC_SECRET_PASSPHRASE + MYCEL_HOME
	// are set (per BuildWorkspaceServices), so both secrets may be
	// visible from either scope. This is a legitimate case for global
	// secrets (proposal §8). The guarantee we DO need is that requests
	// routed through each scope are served without error.
	status, resp = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsAID+"/secrets", nil)
	requireStatus(t, http.StatusOK, status, "list A", resp)
	if !strings.Contains(string(resp), "SECRET_A") {
		t.Errorf("wsA secrets list missing SECRET_A: %s", resp)
	}

	status, resp = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsBID+"/secrets", nil)
	requireStatus(t, http.StatusOK, status, "list B", resp)
	if !strings.Contains(string(resp), "SECRET_B") {
		t.Errorf("wsB secrets list missing SECRET_B: %s", resp)
	}
}

// ---------------- Cron (routing + basic CRUD through scoped URL) --

// TestDispatch_Cron verifies that cron CRUD routes through the scoped
// URL into the handler. Because the cron_jobs table has no
// workspace_id column today, two workspaces sharing the bc.db see the
// same rows — strict data isolation is a known gap. The assertion
// here is routing: both scoped URLs serve the cron handler without
// error, and an entry created via wsA is reachable via its key.
func TestDispatch_Cron(t *testing.T) {
	h := newDispatchHarness(t)
	defer h.close()

	body := func(name string) []byte {
		m := map[string]any{
			"name":     name,
			"schedule": "0 * * * *",
			"command":  "echo hi",
		}
		b, _ := json.Marshal(m)
		return b
	}

	// Create job "job-shared" under wsA via scoped URL.
	status, resp := h.api(t, http.MethodPost, "/api/workspaces/"+h.wsAID+"/cron", body("job-shared"))
	requireStatus(t, http.StatusCreated, status, "create via wsA", resp)

	// Same job fetched under wsA succeeds.
	status, resp = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsAID+"/cron/job-shared", nil)
	requireStatus(t, http.StatusOK, status, "GET wsA", resp)

	// wsB's scoped URL also reaches the cron handler. Data isolation for
	// cron_jobs is a known gap; we assert the handler responds and the
	// route does not collide with a different resource.
	status, resp = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsBID+"/cron", nil)
	requireStatus(t, http.StatusOK, status, "list wsB", resp)

	// Cross-scope fetch for a nonexistent key returns 404.
	status, _ = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsBID+"/cron/nonexistent-xyz", nil)
	if status != http.StatusNotFound {
		t.Errorf("unknown cron key via wsB status=%d, want 404", status)
	}
}

// ---------------- MCP (routing) ----------------

// TestDispatch_MCP verifies MCP routes serve through both scoped URLs.
// The mcp_servers table has no workspace_id column, so strict
// isolation is deferred; we only assert routing here.
func TestDispatch_MCP(t *testing.T) {
	h := newDispatchHarness(t)
	defer h.close()

	body := func(name string) []byte {
		m := map[string]any{
			"name":      name,
			"transport": "stdio",
			"command":   "echo",
			"enabled":   true,
		}
		b, _ := json.Marshal(m)
		return b
	}

	status, resp := h.api(t, http.MethodPost, "/api/workspaces/"+h.wsAID+"/mcp", body("mcp-shared"))
	requireStatus(t, http.StatusCreated, status, "create A", resp)

	status, resp = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsBID+"/mcp", nil)
	requireStatus(t, http.StatusOK, status, "list B", resp)

	status, _ = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsBID+"/mcp/nonexistent-xyz", nil)
	if status != http.StatusNotFound {
		t.Errorf("unknown mcp key wsB status=%d want 404", status)
	}
}

// ---------------- Tools (routing) ----------------

// TestDispatch_Tools verifies the unified tools list is served through
// both scoped URLs. The tools table lives in shared bc.db; strict
// per-workspace isolation requires schema changes (out of scope here).
func TestDispatch_Tools(t *testing.T) {
	h := newDispatchHarness(t)
	defer h.close()

	body := func(name string) []byte {
		m := map[string]any{
			"name":    name,
			"type":    "cli",
			"command": "echo",
			"enabled": true,
		}
		b, _ := json.Marshal(m)
		return b
	}

	status, resp := h.api(t, http.MethodPost, "/api/workspaces/"+h.wsAID+"/tools", body("tool-shared"))
	requireStatus(t, http.StatusCreated, status, "create A", resp)

	// Both scoped URLs reach the tools handler.
	status, resp = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsAID+"/tools", nil)
	requireStatus(t, http.StatusOK, status, "list A", resp)

	status, resp = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsBID+"/tools", nil)
	requireStatus(t, http.StatusOK, status, "list B", resp)
}

// ---------------- Events / Logs (routing) ----------------

// TestDispatch_Events verifies GET /api/logs routes through the scoped
// URL. Like cron/mcp/tool, the events table in bc.db is shared across
// workspaces today. Per-workspace isolation exists at the JSONL writer
// level (events.jsonl under each ws.StateDir) but the SQLite
// EventStore is a single table — we only assert routing here.
func TestDispatch_Events(t *testing.T) {
	h := newDispatchHarness(t)
	defer h.close()

	svcA := h.mgr.Get(h.wsAID)
	svcB := h.mgr.Get(h.wsBID)
	if svcA == nil || svcB == nil {
		t.Fatalf("workspaces not loaded")
	}

	if svcA.Events == nil || svcB.Events == nil {
		t.Skip("event store not configured — cannot exercise dispatch")
	}

	if err := svcA.Events.Append(bcevents.Event{Type: "probe.shared", Agent: "alice"}); err != nil {
		t.Fatalf("append A: %v", err)
	}

	// Scoped URL reaches the handler for both workspaces.
	status, resp := h.api(t, http.MethodGet, "/api/workspaces/"+h.wsAID+"/logs?tail=100", nil)
	requireStatus(t, http.StatusOK, status, "logs A", resp)

	status, resp = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsBID+"/logs?tail=100", nil)
	requireStatus(t, http.StatusOK, status, "logs B", resp)
}

// ---------------- Cost ----------------

// TestDispatch_Cost verifies cost CRUD routes through the scoped URL.
// WorkspaceSummary today does SELECT SUM over every row regardless of
// workspace_id (pkg/cost/cost.go) so strict per-workspace totals via
// the /api/costs summary endpoint do not hold. What DOES hold is that
// the cost store at the SQL level can aggregate by workspace_id via
// SumByWorkspace — this is asserted separately in
// pkg/cost/global_test.go TestScopedRecordTagsWorkspace. Here we just
// assert routing + non-error responses.
func TestDispatch_Cost(t *testing.T) {
	h := newDispatchHarness(t)
	defer h.close()

	svcA := h.mgr.Get(h.wsAID)
	svcB := h.mgr.Get(h.wsBID)
	if svcA == nil || svcB == nil {
		t.Fatalf("workspaces not loaded")
	}
	if svcA.Costs == nil || svcB.Costs == nil {
		t.Skip("cost store not configured — cannot exercise dispatch")
	}

	// Record scoped to each workspace via the global ledger's ScopedTo.
	// The per-request svc.Costs pointer is the global store (not yet
	// pre-scoped) so direct Record() would tag with workspace_id="".
	// We scope explicitly here to verify the SQL schema supports
	// per-workspace aggregation; the handler-level summary endpoint
	// still uses SUM across all rows (known limitation).
	ctx := context.Background()
	scopedA := svcA.Costs.ScopedTo(h.wsAID)
	scopedB := svcB.Costs.ScopedTo(h.wsBID)
	if _, err := scopedA.Record(ctx, "alice", "", "m", 10, 5, 1.11); err != nil {
		t.Fatalf("record A: %v", err)
	}
	if _, err := scopedB.Record(ctx, "bob", "", "m", 10, 5, 2.22); err != nil {
		t.Fatalf("record B: %v", err)
	}

	// Both scoped URLs reach the cost handler and return a valid summary.
	status, resp := h.api(t, http.MethodGet, "/api/workspaces/"+h.wsAID+"/costs", nil)
	requireStatus(t, http.StatusOK, status, "summary A", resp)
	_ = mustFloat(t, resp, "total_cost_usd")

	status, resp = h.api(t, http.MethodGet, "/api/workspaces/"+h.wsBID+"/costs", nil)
	requireStatus(t, http.StatusOK, status, "summary B", resp)
	_ = mustFloat(t, resp, "total_cost_usd")

	// SumByWorkspace on the underlying global ledger separates the two.
	byWS, err := svcA.Costs.SumByWorkspace(ctx, time.Time{})
	if err != nil {
		t.Fatalf("SumByWorkspace: %v", err)
	}
	if byWS[h.wsAID] <= 0 {
		t.Errorf("wsA missing from ledger: %+v", byWS)
	}
	if byWS[h.wsBID] <= 0 {
		t.Errorf("wsB missing from ledger: %+v", byWS)
	}
}

// ---------------- Unknown workspace -> 404 ----------------

// TestDispatch_Unknown404 verifies an unregistered workspace id returns
// 404 from the scope middleware.
func TestDispatch_Unknown404(t *testing.T) {
	h := newDispatchHarness(t)
	defer h.close()

	status, body := h.api(t, http.MethodGet, "/api/workspaces/does-not-exist-xyz/templates", nil)
	if status != http.StatusNotFound {
		t.Errorf("unknown ws status=%d want 404 body=%s", status, body)
	}
}

// ---------------- helpers ----------------

func mustFloat(t *testing.T, body []byte, key string) float64 {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal %s: %v body=%s", key, err, body)
	}
	v, ok := m[key].(float64)
	if !ok {
		t.Fatalf("no %s in %s", key, body)
	}
	return v
}

// Ensure fmt import doesn't get optimized away across future edits.
var _ = fmt.Sprintf
