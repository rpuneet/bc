// legacy_scope_test.go — coverage for the un-scoped UI → /w/<active>/...
// redirect middleware.
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

// bootLegacyScope produces a working mgr + next handler so each test
// focuses on the assertion rather than wiring.
func bootLegacyScope(t *testing.T, withActive bool) (http.Handler, http.Handler) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BC_HOME", filepath.Join(tmp, ".bc"))

	reg, err := workspace.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
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
	}

	hub := bcws.NewHub()
	go hub.Run()
	t.Cleanup(hub.Stop)

	globals := &Globals{Registry: reg, GlobalHub: hub}
	mgr := NewWorkspaceManager(reg, func(ctx context.Context, w *workspace.Workspace) (*WorkspaceServices, error) {
		return BuildWorkspaceServices(ctx, globals, w.RootDir)
	})
	t.Cleanup(func() { _ = mgr.Close() })

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("inner")) //nolint:errcheck // test
	})
	return LegacyUIScope(inner, mgr), inner
}

func TestLegacyUIScope_RedirectsTopLevelPages(t *testing.T) {
	h, _ := bootLegacyScope(t, true)

	cases := []string{"/live", "/agents", "/channels", "/metrics", "/tools", "/workspace"}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusMovedPermanently {
				t.Fatalf("status = %d, want 301", rec.Code)
			}
			loc := rec.Header().Get("Location")
			if !strings.HasPrefix(loc, "/w/") || !strings.HasSuffix(loc, path) {
				t.Errorf("Location = %q, want /w/<id>%s", loc, path)
			}
			if rec.Header().Get("Deprecation") != "true" {
				t.Error("missing Deprecation: true header")
			}
			if rec.Header().Get("Sunset") == "" {
				t.Error("missing Sunset header")
			}
		})
	}
}

func TestLegacyUIScope_PreservesSubpathAndQuery(t *testing.T) {
	h, _ := bootLegacyScope(t, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/agents/zen-zebra/config?tab=mcp", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/agents/zen-zebra/config") {
		t.Errorf("Location = %q missing subpath", loc)
	}
	if !strings.HasSuffix(loc, "?tab=mcp") {
		t.Errorf("Location = %q missing query", loc)
	}
}

func TestLegacyUIScope_PassesThroughWhenNoActiveWorkspace(t *testing.T) {
	h, _ := bootLegacyScope(t, false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (pass through to SPA)", rec.Code)
	}
	if rec.Body.String() != "inner" {
		t.Errorf("body = %q, want inner handler response", rec.Body.String())
	}
}

func TestLegacyUIScope_IgnoresAlreadyScopedAndAPI(t *testing.T) {
	h, _ := bootLegacyScope(t, true)

	for _, path := range []string{
		"/",
		"/w/abcdef/live",
		"/api/agents",
		"/_mcp/zen-zebra/sse",
		"/assets/index-abc.js",
		"/static/img.png",
		"/favicon.ico",
		"/healthz",
		"/some.css",
		"/bundle.js",
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			h.ServeHTTP(rec, req)
			if rec.Code == http.StatusMovedPermanently {
				t.Fatalf("path %q was unexpectedly redirected to %q", path, rec.Header().Get("Location"))
			}
		})
	}
}

func TestLegacyUIScope_IgnoresNonGetMethods(t *testing.T) {
	h, _ := bootLegacyScope(t, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/agents", nil)
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusMovedPermanently {
		t.Fatalf("POST /agents was redirected; middleware must only redirect GET/HEAD")
	}
}
