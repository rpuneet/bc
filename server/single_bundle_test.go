// single_bundle_test.go — boot coverage for single-tenant bcd: one
// Services bundle built by BuildServices serves the flat /api surface,
// /api/repos lists the anchor repo, and the retired multi-tenant routes
// (/api/workspaces, scoped /_mcp/ws/…) are gone (JSON 404, not SPA).
package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/server"
)

func getJSON(t *testing.T, ts *httptest.Server, path string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, body
}

func TestSingleBundleBoot(t *testing.T) {
	svc := buildTestBundle(t)
	srv := server.New(server.Config{Addr: "127.0.0.1:0", CORS: true}, svc, nil, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// The bundle is online: /api/agents answers.
	if status, body := getJSON(t, ts, "/api/agents"); status != http.StatusOK {
		t.Errorf("/api/agents = %d body=%s", status, body)
	}

	// /api/repos returns the anchor repo as default.
	status, body := getJSON(t, ts, "/api/repos")
	if status != http.StatusOK {
		t.Fatalf("/api/repos = %d body=%s", status, body)
	}
	var repos struct {
		Default string `json:"default"`
		Repos   []struct {
			Path string `json:"path"`
		} `json:"repos"`
	}
	if err := json.Unmarshal(body, &repos); err != nil {
		t.Fatalf("unmarshal /api/repos: %v (%s)", err, body)
	}
	if svc.WS == nil || repos.Default != svc.WS.RootDir {
		t.Errorf("default repo = %q, want bundle root %q", repos.Default, svc.WS.RootDir)
	}
	found := false
	for _, r := range repos.Repos {
		if r.Path == repos.Default {
			found = true
		}
	}
	if !found {
		t.Errorf("anchor repo missing from list: %+v", repos.Repos)
	}

	// The multi-tenant surface is gone: /api/workspaces 404s with JSON
	// (not the SPA fallback), and the scoped MCP dispatch path is dead.
	for _, path := range []string{"/api/workspaces", "/api/workspaces/abc123/agents", "/_mcp/ws/abc123/agent/sse", "/api/workspace", "/api/workspace/status", "/api/workspace/up", "/api/workspace/down", "/api/workspace/roles"} {
		status, body := getJSON(t, ts, path)
		if status != http.StatusNotFound {
			t.Errorf("%s = %d, want 404 (multi-tenant surface must be gone); body=%s", path, status, body)
		}
	}
}
