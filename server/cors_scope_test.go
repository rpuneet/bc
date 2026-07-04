// cors_scope_test.go — regression test for the CORS + WorkspaceScope
// middleware interaction (originally commit 0e2bed15). The CORS wrapper
// must sit OUTSIDE the WorkspaceScope wrapper so that per-request
// workspace resolution still fires while CORS headers land on the
// response.
//
// This test boots the real middleware chain with CORS enabled, fires a
// workspace-scoped request via the flat /api/<rest>?workspace=<id>
// surface (#3079), and asserts BOTH concerns at once:
//
//  1. The CORS headers (Access-Control-Allow-Origin, Allow-Methods) land
//     on the response — i.e. the CORS wrapper IS in the chain.
//  2. The request is actually dispatched (no 404) — i.e. the scope
//     middleware is NOT bypassed.
package server_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	bcdb "github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/workspace"
	"github.com/rpuneet/mycel/server"
	bcws "github.com/rpuneet/mycel/server/ws"
)

// TestCORS_WorkspaceScope_Coexist boots a bcd server with CORS enabled
// (the default dev flow) and verifies that a scoped URL
// /api/workspaces/{id}/<resource> is still dispatched correctly by the
// WorkspaceScope middleware — i.e. the CORS wrapper does not bypass
// scope.
func TestCORS_WorkspaceScope_Coexist(t *testing.T) {
	// Isolate state.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", filepath.Join(home, ".bc"))
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")

	wsDir := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(wsDir, 0o750); err != nil {
		t.Fatalf("mkdir ws: %v", err)
	}
	gitInitDir(t, wsDir)
	if _, err := workspace.Init(wsDir); err != nil {
		t.Fatalf("workspace.Init: %v", err)
	}
	wsID := workspace.ComputeWorkspaceID(wsDir)

	// BuildWorkspaceServices resolves the global db lazily; just make
	// sure it is released after the test.
	t.Cleanup(func() { _ = bcdb.CloseGlobal() })

	reg, err := workspace.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if regErr := reg.RegisterWithAlias(wsDir, "ws", "w"); regErr != nil {
		t.Fatalf("register: %v", regErr)
	}
	if actErr := reg.SetActive(wsDir); actErr != nil {
		t.Fatalf("SetActive: %v", actErr)
	}

	hub := bcws.NewHub()
	go hub.Run()
	t.Cleanup(hub.Stop)

	globals := &server.Globals{Registry: reg, GlobalHub: hub}

	ctx := context.Background()
	mgr := server.NewWorkspaceManager(reg, func(ctx context.Context, w *workspace.Workspace) (*server.WorkspaceServices, error) {
		gitInitDir(t, w.RootDir)
		return server.BuildWorkspaceServices(ctx, globals, w.RootDir)
	})
	if _, loadErr := mgr.LoadActive(ctx); loadErr != nil {
		t.Fatalf("LoadActive: %v", loadErr)
	}
	t.Cleanup(func() { _ = mgr.Close() })

	// CORS on, explicit origin — the production dev-flow shape.
	srv := server.NewWithManager(server.Config{
		Addr:       "127.0.0.1:0",
		CORS:       true,
		CORSOrigin: "http://localhost:5173",
	}, mgr, globals, nil)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Fire a scoped GET. Under the buggy wiring this returned 404 from
	// the mux because CORS wrapped the raw mux and the scope middleware
	// never saw the request.
	scopedURL := fmt.Sprintf("%s/api/agents?workspace=%s", ts.URL, wsID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scopedURL, nil)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	req.Header.Set("Origin", "http://localhost:5173")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// (1) CORS headers are present — the CORS middleware is definitely
	// in the chain.
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:5173")
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Access-Control-Allow-Methods missing — CORS middleware not in chain")
	}

	// (2) Status is NOT 404 — scope middleware rewrote the URL and the
	// handler was reached. A 200 (agents list) is the common shape here,
	// but anything that is not 404 is sufficient to prove routing: the
	// bug returned exactly 404 because the raw mux had no
	// /api/workspaces/{id}/agents pattern.
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("scoped URL returned 404 — WorkspaceScope was bypassed by CORS wrapper (regression of 0e2bed15)")
	}

	// A well-formed call to /agents returns 200 with a JSON body; if
	// it's anything else we want to know (5xx would indicate a broken
	// handler wiring rather than the CORS bug).
	if resp.StatusCode >= 500 {
		t.Errorf("scoped URL returned %d — handler wiring regression (not the CORS bug)", resp.StatusCode)
	}
}

// TestCORS_Preflight_Scoped verifies OPTIONS preflight for a scoped URL
// returns 204 with the CORS headers set. This exercises both middlewares
// in the preflight flow: CORS short-circuits on OPTIONS with the
// advertised allow-methods, regardless of whether the scope rewrite
// would have succeeded.
func TestCORS_Preflight_Scoped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", filepath.Join(home, ".bc"))
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")

	wsDir := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(wsDir, 0o750); err != nil {
		t.Fatalf("mkdir ws: %v", err)
	}
	gitInitDir(t, wsDir)
	if _, err := workspace.Init(wsDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	wsID := workspace.ComputeWorkspaceID(wsDir)

	t.Cleanup(func() { _ = bcdb.CloseGlobal() })

	reg, err := workspace.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	_ = reg.RegisterWithAlias(wsDir, "ws", "w")
	_ = reg.SetActive(wsDir)

	hub := bcws.NewHub()
	go hub.Run()
	t.Cleanup(hub.Stop)

	globals := &server.Globals{Registry: reg, GlobalHub: hub}
	mgr := server.NewWorkspaceManager(reg, func(ctx context.Context, w *workspace.Workspace) (*server.WorkspaceServices, error) {
		gitInitDir(t, w.RootDir)
		return server.BuildWorkspaceServices(ctx, globals, w.RootDir)
	})
	if _, loadErr := mgr.LoadActive(context.Background()); loadErr != nil {
		t.Fatalf("LoadActive: %v", loadErr)
	}
	t.Cleanup(func() { _ = mgr.Close() })

	srv := server.NewWithManager(server.Config{Addr: "127.0.0.1:0", CORS: true, CORSOrigin: "*"}, mgr, globals, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodOptions,
		fmt.Sprintf("%s/api/agents?workspace=%s", ts.URL, wsID), nil)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin = %q, want *", got)
	}
}
