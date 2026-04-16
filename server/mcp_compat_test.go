// mcp_compat_test.go — coverage for the pre-M6 MCP path rewrite.
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/bc/pkg/workspace"
	bcws "github.com/rpuneet/bc/server/ws"
)

// captureHandler records the URL.Path the middleware forwarded so tests
// can assert the rewrite happened (or didn't) without spinning up a real
// MCP server.
type captureHandler struct {
	gotPath  string
	gotQuery string
}

func (c *captureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.gotPath = r.URL.Path
	c.gotQuery = r.URL.RawQuery
	w.WriteHeader(http.StatusOK)
}

func bootMCPCompat(t *testing.T, withActive bool) (http.Handler, *captureHandler, string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BC_HOME", filepath.Join(tmp, ".bc"))

	reg, err := workspace.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	var activeID string
	if withActive {
		wsDir := t.TempDir()
		if _, initErr := workspace.Init(wsDir); initErr != nil {
			t.Fatalf("workspace.Init: %v", initErr)
		}
		if regErr := reg.RegisterWithAlias(wsDir, "ws", ""); regErr != nil {
			t.Fatalf("register: %v", regErr)
		}
		if actErr := reg.SetActive(wsDir); actErr != nil {
			t.Fatalf("SetActive: %v", actErr)
		}
		if e := reg.GetActive(); e != nil {
			activeID = e.ID
		}
	}

	hub := bcws.NewHub()
	go hub.Run()
	t.Cleanup(hub.Stop)

	globals := &Globals{Registry: reg, GlobalHub: hub}
	mgr := NewWorkspaceManager(reg, func(ctx context.Context, w *workspace.Workspace) (*WorkspaceServices, error) {
		return BuildWorkspaceServices(ctx, globals, w.RootDir)
	})
	t.Cleanup(func() { _ = mgr.Close() })

	cap := &captureHandler{}
	return LegacyMCPCompat(cap, mgr), cap, activeID
}

func TestLegacyMCPCompat_RewritesSSE(t *testing.T) {
	h, cap, activeID := bootMCPCompat(t, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_mcp/zen-zebra/sse", nil)
	h.ServeHTTP(rec, req)

	want := "/_mcp/ws/" + activeID + "/zen-zebra/sse"
	if cap.gotPath != want {
		t.Errorf("rewritten path = %q, want %q", cap.gotPath, want)
	}
	if rec.Header().Get("Deprecation") != "true" {
		t.Error("missing Deprecation header")
	}
	if rec.Header().Get("Sunset") == "" {
		t.Error("missing Sunset header")
	}
}

func TestLegacyMCPCompat_RewritesMessagePostWithQuery(t *testing.T) {
	h, cap, activeID := bootMCPCompat(t, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/_mcp/zen-zebra/message?session=abc", strings.NewReader(`{"method":"ping"}`))
	h.ServeHTTP(rec, req)

	want := "/_mcp/ws/" + activeID + "/zen-zebra/message"
	if cap.gotPath != want {
		t.Errorf("rewritten path = %q, want %q", cap.gotPath, want)
	}
	if cap.gotQuery != "session=abc" {
		t.Errorf("query = %q, want session=abc (should survive rewrite)", cap.gotQuery)
	}
}

func TestLegacyMCPCompat_PassesThroughAlreadyScoped(t *testing.T) {
	h, cap, activeID := bootMCPCompat(t, true)

	alreadyScoped := "/_mcp/ws/" + activeID + "/zen-zebra/sse"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, alreadyScoped, nil)
	h.ServeHTTP(rec, req)

	if cap.gotPath != alreadyScoped {
		t.Errorf("path = %q, want %q (no rewrite)", cap.gotPath, alreadyScoped)
	}
	if rec.Header().Get("Deprecation") == "true" {
		t.Error("Deprecation header set on already-scoped URL")
	}
}

func TestLegacyMCPCompat_PassesThroughUnknownAction(t *testing.T) {
	h, cap, _ := bootMCPCompat(t, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_mcp/zen-zebra/capabilities", nil)
	h.ServeHTTP(rec, req)

	if cap.gotPath != "/_mcp/zen-zebra/capabilities" {
		t.Errorf("path = %q, want unchanged", cap.gotPath)
	}
	if rec.Header().Get("Deprecation") == "true" {
		t.Error("Deprecation header set for non-sse/message action")
	}
}

func TestLegacyMCPCompat_PassesThroughWhenNoActiveWorkspace(t *testing.T) {
	h, cap, _ := bootMCPCompat(t, false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_mcp/zen-zebra/sse", nil)
	h.ServeHTTP(rec, req)

	if cap.gotPath != "/_mcp/zen-zebra/sse" {
		t.Errorf("path = %q, want unchanged (no active workspace)", cap.gotPath)
	}
}

func TestLegacyMCPCompat_IgnoresNonMCPPaths(t *testing.T) {
	h, cap, _ := bootMCPCompat(t, true)

	for _, path := range []string{"/api/agents", "/live", "/assets/x.js", "/"} {
		t.Run(path, func(t *testing.T) {
			cap.gotPath = ""
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			h.ServeHTTP(rec, req)
			if cap.gotPath != path {
				t.Errorf("path = %q, want %q", cap.gotPath, path)
			}
		})
	}
}
