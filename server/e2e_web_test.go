// Package server_test — web UI smoke tests for bcd HTTP API (Phase 3).
//
// These tests verify that the bcd server correctly serves the embedded web UI
// (SPA fallback, static files) and that all API endpoints the web UI depends on
// return valid responses. Uses httptest with real server infrastructure.
package server_test

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/cost"
	bcdb "github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/events"
	pkgmcp "github.com/rpuneet/mycel/pkg/mcp"
	"github.com/rpuneet/mycel/pkg/tool"
	"github.com/rpuneet/mycel/pkg/workspace"
	"github.com/rpuneet/mycel/server"
)

// newE2EServerWithWebUI creates a bcd server with a synthetic web UI filesystem
// for testing SPA serving behavior. The filesystem contains a minimal
// index.html and a static asset.
func newE2EServerWithWebUI(t *testing.T) *e2eServer {
	t.Helper()

	dir := t.TempDir()
	// workspace.Load requires a git repo
	if out, err := exec.CommandContext(context.Background(), "git", "init", dir).CombinedOutput(); err != nil { //nolint:gosec // dir is a t.TempDir(), not user input
		t.Fatalf("git init: %v\n%s", err, out)
	}
	t.Setenv("MYCEL_HOME", t.TempDir())
	stateDir, sdErr := workspace.GlobalStateDir(dir)
	if sdErr != nil {
		t.Fatal(sdErr)
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "roles"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "agents"), 0750); err != nil {
		t.Fatal(err)
	}

	cfg := `{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`
	if err := os.WriteFile(filepath.Join(stateDir, workspace.PreferencesFileName), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("workspace load: %v", err)
	}

	// Per-workspace database via the registry (production path).
	wsDB, wsDriver, dbErr := bcdb.Global(nil)
	if dbErr != nil {
		t.Fatalf("open workspace db: %v", dbErr)
	}
	t.Cleanup(func() { _ = bcdb.CloseGlobal() })

	hub := ws_hub(t)
	mgr := agent.NewWorkspaceManager(ws.StateDir(), ws.RootDir)
	_ = mgr.LoadState()
	agentSvc := agent.NewAgentService(mgr, hub, nil)

	var costStore *cost.Store
	cs := cost.NewStore(ws.RootDir)
	if err := cs.Open(); err == nil {
		costStore = cs
		t.Cleanup(func() { _ = cs.Close() })
	}

	var mcpStore *pkgmcp.Store
	if ms, err := pkgmcp.NewStore(wsDB, wsDriver); err == nil {
		mcpStore = ms
		t.Cleanup(func() { _ = ms.Close() })
	}

	var toolStore *tool.Store
	ts := tool.NewStore(wsDB, wsDriver)
	if err := ts.Open(); err == nil {
		toolStore = ts
		t.Cleanup(func() { _ = ts.Close() })
	}

	var eventLog events.EventStore
	if el, err := events.NewSQLiteLog(wsDB); err == nil {
		eventLog = el
		t.Cleanup(func() { _ = el.Close() })
	}

	svc := server.Services{
		Agents:   agentSvc,
		Costs:    costStore,
		MCP:      mcpStore,
		Tools:    toolStore,
		EventLog: eventLog,
		WS:       ws,
	}

	// Synthetic web UI filesystem for SPA testing
	staticFiles := syntheticWebUI()

	srvCfg := server.Config{Addr: "127.0.0.1:0", CORS: true}
	srv := server.New(srvCfg, svc, hub, staticFiles)
	ts2 := httptest.NewServer(srv.Handler())
	t.Cleanup(ts2.Close)

	return &e2eServer{Server: ts2, ws: ws}
}

// syntheticWebUI returns an in-memory filesystem that mimics a built web UI.
func syntheticWebUI() fs.FS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<!DOCTYPE html><html><head><title>bc</title></head><body><div id=\"root\"></div></body></html>"),
		},
		"assets/app.js": &fstest.MapFile{
			Data: []byte("console.log('bc')"),
		},
		"assets/style.css": &fstest.MapFile{
			Data: []byte("body { margin: 0; }"),
		},
	}
}

// getRaw performs a GET request and returns the raw response (caller must close body).
func (s *e2eServer) getRaw(t *testing.T, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.URL+path, nil)
	if err != nil {
		t.Fatalf("create request GET %s: %v", path, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

// ─── Web UI Serving ──────────────────────────────────────────────────────────

// TestE2E_WebUI_ServesIndex verifies GET / returns 200 with HTML content
// when a web UI filesystem is provided.
func TestE2E_WebUI_ServesIndex(t *testing.T) {
	s := newE2EServerWithWebUI(t)

	resp := s.getRaw(t, "/", nil)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /: want 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("GET /: want Content-Type containing text/html, got %q", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<div id=\"root\">") {
		t.Fatal("GET /: response body does not contain expected HTML content")
	}
}

// TestE2E_WebUI_SPAFallback verifies that non-API routes that don't match
// static files return HTML (SPA client-side routing) instead of 404.
func TestE2E_WebUI_SPAFallback(t *testing.T) {
	s := newE2EServerWithWebUI(t)

	routes := []string{"/dashboard", "/agents", "/settings"}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			resp := s.getRaw(t, route, nil)
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: want 200, got %d", route, resp.StatusCode)
			}

			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, "text/html") {
				t.Fatalf("GET %s: want Content-Type containing text/html, got %q", route, ct)
			}

			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), "<div id=\"root\">") {
				t.Fatalf("GET %s: SPA fallback did not serve index.html", route)
			}
		})
	}
}

// TestE2E_WebUI_LegacyWorkspacePathServesSPA verifies that legacy
// /w/<hash>/<page> URLs serve index.html directly, WITHOUT a server-side
// redirect. Browsers that cached the old /agents -> /w/<hash>/agents 301
// would otherwise loop forever (cached 301 -> server 301 -> cached 301).
func TestE2E_WebUI_LegacyWorkspacePathServesSPA(t *testing.T) {
	s := newE2EServerWithWebUI(t)

	// Client that does NOT follow redirects, so any 301/302 fails loudly.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.URL+"/w/abc123def456/agents", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /w/abc123def456/agents: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 (SPA served in place, no redirect), got %d (Location=%q)",
			resp.StatusCode, resp.Header.Get("Location"))
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<div id=\"root\">") {
		t.Fatalf("legacy /w/ path did not serve index.html; body=%q", string(body)[:min(len(body), 120)])
	}
}

// TestE2E_WebUI_StaticAssets verifies that static assets are served with
// correct content types.
func TestE2E_WebUI_StaticAssets(t *testing.T) {
	s := newE2EServerWithWebUI(t)

	tests := []struct {
		path        string
		wantCT      string
		wantContain string
	}{
		{"/index.html", "text/html", "<div id=\"root\">"},
		{"/assets/app.js", "javascript", "console.log"},
		{"/assets/style.css", "css", "body"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp := s.getRaw(t, tt.path, nil)
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: want 200, got %d", tt.path, resp.StatusCode)
			}

			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, tt.wantCT) {
				t.Fatalf("GET %s: want Content-Type containing %q, got %q", tt.path, tt.wantCT, ct)
			}

			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), tt.wantContain) {
				t.Fatalf("GET %s: response body does not contain %q", tt.path, tt.wantContain)
			}
		})
	}
}

// ─── API Surface for Web UI ──────────────────────────────────────────────────

// TestE2E_WebUI_APIEndpointsReturnJSON verifies all major API endpoints the
// web UI depends on return 200 with JSON content type.
func TestE2E_WebUI_APIEndpointsReturnJSON(t *testing.T) {
	s := newE2EServer(t)

	tests := []struct {
		path   string
		wantCT string
	}{
		{"/api/agents", "application/json"},
		{"/api/costs", "application/json"},
		{"/api/roles", "application/json"},
		{"/api/doctor", "application/json"},
		{"/health", "application/json"},
		{"/health/ready", "application/json"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp := s.getRaw(t, tt.path, nil)
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: want 200, got %d", tt.path, resp.StatusCode)
			}

			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, tt.wantCT) {
				t.Fatalf("GET %s: want Content-Type containing %q, got %q", tt.path, tt.wantCT, ct)
			}
		})
	}
}

// ─── SSE Endpoint ────────────────────────────────────────────────────────────

// TestE2E_WebUI_SSEEndpoint verifies the SSE event stream endpoint accepts
// connections and returns the correct content type.
func TestE2E_WebUI_SSEEndpoint(t *testing.T) {
	s := newE2EServer(t)

	resp := s.getRaw(t, "/api/events", map[string]string{
		"Accept": "text/event-stream",
	})
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/events: want 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("GET /api/events: want Content-Type containing text/event-stream, got %q", ct)
	}
}

// ─── CORS (web-specific) ────────────────────────────────────────────────────

// TestE2E_WebUI_CORSHeaders verifies CORS headers are present on API
// responses (not just OPTIONS preflight, which is tested in e2e_test.go).
func TestE2E_WebUI_CORSHeaders(t *testing.T) {
	s := newE2EServer(t)

	// Verify CORS headers on a regular GET (not just OPTIONS preflight).
	// The e2e_test.go TestE2E_CORS_Headers covers OPTIONS; this covers
	// actual API responses that the web UI will receive.
	paths := []string{"/api/agents", "/api/roles", "/health"}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			resp := s.getRaw(t, path, map[string]string{
				"Origin": "http://localhost:3000",
			})
			defer func() { _ = resp.Body.Close() }()

			origin := resp.Header.Get("Access-Control-Allow-Origin")
			if origin != "*" {
				t.Fatalf("GET %s: want Access-Control-Allow-Origin=*, got %q", path, origin)
			}
		})
	}
}

// ─── Full Web Workflow ───────────────────────────────────────────────────────

// TestE2E_WebUI_FullWorkflow exercises a complete web UI workflow:
// health check → verify agents → verify server still healthy.
func TestE2E_WebUI_FullWorkflow(t *testing.T) {
	s := newE2EServer(t)

	// 1. GET /health → verify the server reports healthy
	code, healthBody := s.get(t, "/health")
	if code != http.StatusOK {
		t.Fatalf("health: want 200, got %d", code)
	}
	if healthBody["status"] == nil || healthBody["status"] == "" {
		t.Fatalf("health status: want non-empty, got %v", healthBody["status"])
	}

	// 2. GET /api/agents → verify agents endpoint works
	code, _ = s.get(t, "/api/agents")
	if code != http.StatusOK {
		t.Fatalf("agents list: want 200, got %d", code)
	}

	// 3. GET /health → verify the server is still healthy
	code, _ = s.get(t, "/health")
	if code != http.StatusOK {
		t.Fatalf("health (final): want 200, got %d", code)
	}
}
