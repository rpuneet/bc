// cors_single_bundle_test.go — regression test for the CORS middleware
// placement (originally commit 0e2bed15). The CORS wrapper must sit in
// the chain so headers land on API responses, without swallowing routing:
// a real /api/agents request must reach the handler (no 404), and OPTIONS
// preflight must short-circuit with 204.
//
// Formerly this exercised the multi-tenant WorkspaceScope middleware; bcd
// is single-tenant now, so the test boots the one service bundle via
// BuildServices and asserts the same two concerns against flat routes.
package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	bcdb "github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/server"
)

// buildTestBundle boots one Services bundle against a fresh git repo.
func buildTestBundle(t *testing.T) server.Services {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", filepath.Join(home, ".bc"))
	t.Setenv("MYCEL_SECRET_PASSPHRASE", "unit-test")

	wsDir := filepath.Join(t.TempDir(), "h")
	if err := os.MkdirAll(wsDir, 0o750); err != nil {
		t.Fatalf("mkdir h: %v", err)
	}
	gitInitDir(t, wsDir)

	// BuildServices resolves the global db lazily; release it after.
	t.Cleanup(func() { _ = bcdb.CloseGlobal() })

	svc, err := server.BuildServices(context.Background(), &server.Globals{}, wsDir)
	if err != nil {
		t.Fatalf("BuildServices: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return *svc
}

// TestCORS_SingleBundle_Coexist boots a bcd server with CORS enabled (the
// default dev flow) and verifies /api/agents is dispatched with the CORS
// headers present — i.e. the CORS wrapper is in the chain and does not
// bypass routing.
func TestCORS_SingleBundle_Coexist(t *testing.T) {
	svc := buildTestBundle(t)

	// CORS on, explicit origin — the production dev-flow shape.
	srv := server.New(server.Config{
		Addr:       "127.0.0.1:0",
		CORS:       true,
		CORSOrigin: "http://localhost:5173",
	}, svc, nil, nil)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/api/agents", nil)
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

	// (2) Status is NOT 404 — the agents handler was reached.
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("/api/agents returned 404 — routing bypassed by CORS wrapper (regression of 0e2bed15)")
	}
	if resp.StatusCode >= 500 {
		t.Errorf("/api/agents returned %d — handler wiring regression (not the CORS bug)", resp.StatusCode)
	}
}

// TestCORS_Preflight verifies OPTIONS preflight for an API URL returns
// 204 with the CORS headers set.
func TestCORS_Preflight(t *testing.T) {
	svc := buildTestBundle(t)

	srv := server.New(server.Config{Addr: "127.0.0.1:0", CORS: true, CORSOrigin: "*"}, svc, nil, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodOptions, ts.URL+"/api/agents", nil)
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
