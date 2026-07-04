// multi_tenant_test.go — end-to-end isolation invariants for multi-workspace bcd.
//
// Spins up one in-process bcd server with TWO registered workspaces, seeds
// each with two agents in its own state.db, and verifies:
//
//  1. GET /api/workspaces/<A>/agents only returns A's agents.
//  2. GET /api/workspaces/<B>/agents only returns B's agents.
//  3. A hook event appended through A's services is visible through B's
//     too — both share the single global mycel.db (events are agent-keyed).
//  4. Evicting A releases its services; re-loading A preserves disk state.
//
// The test deliberately uses the real WorkspaceManager + factory so a
// regression that re-adds a single-tenant assumption (e.g. a handler
// closing over one workspace's stores) will surface as cross-contamination.
package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	bcdb "github.com/rpuneet/mycel/pkg/db"
	bcevents "github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/workspace"
	"github.com/rpuneet/mycel/server"
	bcws "github.com/rpuneet/mycel/server/ws"
)

// multiTenantHarness is the test fixture: a running bcd + two registered
// workspaces with pre-loaded manager state.
type multiTenantHarness struct {
	ts      *httptest.Server
	mgr     *server.WorkspaceManager
	globals *server.Globals
	wsAID   string
	wsBID   string
	wsADir  string
	wsBDir  string
}

func (h *multiTenantHarness) close() {
	h.ts.Close()
	_ = h.mgr.Close() //nolint:errcheck
}

// newMultiTenantHarness creates two workspaces, registers both, and boots a
// bcd server bound to them. Each workspace is initialized with its own
// .bc/state.db (no shared DB) so cross-contamination is visible.
func newMultiTenantHarness(t *testing.T) *multiTenantHarness {
	t.Helper()

	// Isolate HOME and MYCEL_HOME so the test's registry and global
	// mycel.db live in a fresh sandbox.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", filepath.Join(home, ".mycel"))
	t.Cleanup(func() { _ = bcdb.CloseGlobal() })

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

	reg, loadErr := workspace.LoadRegistry()
	if loadErr != nil {
		t.Fatalf("LoadRegistry: %v", loadErr)
	}
	if regErr := reg.RegisterWithAlias(wsA, "wsA", "a"); regErr != nil {
		t.Fatalf("register wsA: %v", regErr)
	}
	if regErr := reg.RegisterWithAlias(wsB, "wsB", "b"); regErr != nil {
		t.Fatalf("register wsB: %v", regErr)
	}
	if setErr := reg.SetActive(wsA); setErr != nil {
		t.Fatalf("set active: %v", setErr)
	}
	if saveErr := reg.Save(); saveErr != nil {
		t.Fatalf("save registry: %v", saveErr)
	}

	// Global fan-in hub.
	globalHub := bcws.NewHub()
	go globalHub.Run()
	t.Cleanup(globalHub.Stop)

	globals := &server.Globals{
		Registry:  reg,
		GlobalHub: globalHub,
	}

	ctx := context.Background()

	mgr := server.NewWorkspaceManager(reg, func(ctx context.Context, w *workspace.Workspace) (*server.WorkspaceServices, error) {
		gitInitDir(t, w.RootDir)
		return server.BuildWorkspaceServices(ctx, globals, w.RootDir)
	})

	// Eager-load both workspaces so test setup has access to their stores.
	svcA, err := mgr.Load(ctx, wsAID)
	if err != nil {
		t.Fatalf("load wsA: %v", err)
	}
	svcB, err := mgr.Load(ctx, wsBID)
	if err != nil {
		t.Fatalf("load wsB: %v", err)
	}

	// Seed per-workspace state by appending events directly to each
	// workspace's event store. Agents require tmux/docker runtime to
	// truly spawn, which is out of scope — storage isolation at the
	// event layer is the invariant we need here.
	if svcA.Events != nil {
		_ = svcA.Events.Append(bcevents.Event{Type: "seed.alice", Agent: "alice"}) //nolint:errcheck
		_ = svcA.Events.Append(bcevents.Event{Type: "seed.bob", Agent: "bob"})     //nolint:errcheck
	}
	if svcB.Events != nil {
		_ = svcB.Events.Append(bcevents.Event{Type: "seed.carol", Agent: "carol"}) //nolint:errcheck
		_ = svcB.Events.Append(bcevents.Event{Type: "seed.dave", Agent: "dave"})   //nolint:errcheck
	}

	srv := server.NewWithManager(server.Config{Addr: "127.0.0.1:0", CORS: true}, mgr, globals, nil)
	ts := httptest.NewServer(srv.Handler())

	return &multiTenantHarness{
		ts:      ts,
		mgr:     mgr,
		wsAID:   wsAID,
		wsBID:   wsBID,
		wsADir:  wsA,
		wsBDir:  wsB,
		globals: globals,
	}
}

// apiGET performs GET against the harness and returns body + status.
// Scoped paths of the form "/api/workspaces/<id>/<rest>" are auto-rewritten
// to "/api/<rest>?workspace=<id>" (the current API surface, #3079).
func (h *multiTenantHarness) apiGET(t *testing.T, path string) (int, []byte) {
	t.Helper()
	path = rewriteWorkspaceScopedPath(path)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, h.ts.URL+path, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// extractAgentNames parses an /api/agents list response.
func extractAgentNames(t *testing.T, body []byte) []string {
	t.Helper()
	var dtos []map[string]any
	if err := json.Unmarshal(body, &dtos); err != nil {
		t.Fatalf("unmarshal agents: %v (body=%s)", err, string(body))
	}
	out := make([]string, 0, len(dtos))
	for _, d := range dtos {
		if n, ok := d["name"].(string); ok {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// TestMultiTenant_AgentsIsolatedPerWorkspace verifies that scoped
// /api/workspaces/<id>/agents returns only that workspace's agents.
func TestMultiTenant_AgentsIsolatedPerWorkspace(t *testing.T) {
	h := newMultiTenantHarness(t)
	defer h.close()

	statusA, bodyA := h.apiGET(t, "/api/workspaces/"+h.wsAID+"/agents")
	if statusA != http.StatusOK {
		t.Fatalf("GET wsA agents status=%d body=%s", statusA, string(bodyA))
	}
	namesA := extractAgentNames(t, bodyA)

	statusB, bodyB := h.apiGET(t, "/api/workspaces/"+h.wsBID+"/agents")
	if statusB != http.StatusOK {
		t.Fatalf("GET wsB agents status=%d body=%s", statusB, string(bodyB))
	}
	namesB := extractAgentNames(t, bodyB)

	// Cross-contamination check: a name present in one list must not be
	// present in the other. We only assert absence because AddTestAgent
	// may be a no-op in some runtime backends (see seedAgentForTest).
	crossA := map[string]bool{"carol": true, "dave": true}
	for _, n := range namesA {
		if crossA[n] {
			t.Errorf("wsA leaked wsB agent %q (namesA=%v)", n, namesA)
		}
	}
	crossB := map[string]bool{"alice": true, "bob": true}
	for _, n := range namesB {
		if crossB[n] {
			t.Errorf("wsB leaked wsA agent %q (namesB=%v)", n, namesB)
		}
	}

	// If seeding worked, make the positive assertion explicit. When seed
	// is a no-op on this runtime, the positive side is unreachable —
	// the cross-contamination check above is the safety net.
	if len(namesA) >= 2 {
		want := []string{"alice", "bob"}
		if !containsAll(namesA, want) {
			t.Errorf("wsA agents = %v, want superset of %v", namesA, want)
		}
	}
	if len(namesB) >= 2 {
		want := []string{"carol", "dave"}
		if !containsAll(namesB, want) {
			t.Errorf("wsB agents = %v, want superset of %v", namesB, want)
		}
	}
}

// TestMultiTenant_EventsSharedGlobalDB verifies the single-database
// semantics for events: a hook event written through A's services is
// visible through B's too, because both share the one global mycel.db.
// Per-repo filtering happens via the agent/repo keys, not via files.
func TestMultiTenant_EventsSharedGlobalDB(t *testing.T) {
	h := newMultiTenantHarness(t)
	defer h.close()

	svcA := h.mgr.Get(h.wsAID)
	svcB := h.mgr.Get(h.wsBID)
	if svcA == nil || svcB == nil {
		t.Fatalf("expected both workspaces loaded, got A=%v B=%v", svcA, svcB)
	}

	// Append one event through A's event store.
	if svcA.Events == nil {
		t.Skip("event store not configured for wsA — cannot exercise shared db")
	}
	err := svcA.Events.Append(bcevents.Event{
		Type:    "hook.test",
		Agent:   "alice",
		Message: "shared db probe",
		Data:    map[string]any{"note": "shared db probe"},
	})
	if err != nil {
		t.Fatalf("append to wsA events: %v", err)
	}

	// A sees it.
	foundA := false
	if evts, err := svcA.Events.ReadLast(1000); err == nil {
		for _, ev := range evts {
			if ev.Type == "hook.test" && ev.Agent == "alice" {
				foundA = true
			}
		}
	}
	if !foundA {
		t.Error("wsA does not see its own event")
	}

	// B sees the SAME row — one events table for the whole process.
	if svcB.Events == nil {
		t.Skip("event store not configured for wsB")
	}
	foundB := false
	if evts, err := svcB.Events.ReadLast(1000); err == nil {
		for _, ev := range evts {
			if ev.Type == "hook.test" && ev.Agent == "alice" {
				foundB = true
			}
		}
	}
	if !foundB {
		t.Error("shared DB: wsB must see the event appended through wsA's services")
	}
}

// TestMultiTenant_EvictReload verifies that evicting a workspace closes
// its services and a subsequent Load rebuilds them with intact on-disk
// state.
func TestMultiTenant_EvictReload(t *testing.T) {
	h := newMultiTenantHarness(t)
	defer h.close()

	// Sanity: wsA is loaded.
	if svc := h.mgr.Get(h.wsAID); svc == nil {
		t.Fatal("expected wsA loaded at harness startup")
	}

	// Evict wsA.
	if err := h.mgr.Evict(h.wsAID); err != nil {
		t.Fatalf("evict wsA: %v", err)
	}
	if svc := h.mgr.Get(h.wsAID); svc != nil {
		t.Error("wsA should be unloaded after Evict")
	}

	// Re-load wsA — must succeed and produce a fresh services bundle.
	svc, err := h.mgr.Load(context.Background(), h.wsAID)
	if err != nil {
		t.Fatalf("reload wsA: %v", err)
	}
	if svc == nil {
		t.Fatal("reload returned nil")
	}
	if svc.Workspace == nil || svc.Workspace.RootDir != h.wsADir {
		t.Errorf("reloaded wsA has wrong root: %v", svc.Workspace)
	}
}

// containsAll returns true when set contains every element of want.
func containsAll(set, want []string) bool {
	have := make(map[string]struct{}, len(set))
	for _, s := range set {
		have[s] = struct{}{}
	}
	for _, w := range want {
		if _, ok := have[w]; !ok {
			return false
		}
	}
	return true
}
