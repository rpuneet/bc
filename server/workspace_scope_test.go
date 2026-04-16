package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rpuneet/bc/pkg/workspace"
)

// dummyHandler records the path it was invoked with so tests can assert the
// rewrite actually happened.
type dummyHandler struct{ seen string }

func (d *dummyHandler) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	d.seen = r.URL.Path
}

func newScopeTestManager(t *testing.T) (*WorkspaceManager, string, string) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	wsDir := filepath.Join(tmpDir, "ws")
	if err := os.MkdirAll(wsDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := workspace.Init(wsDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	id := workspace.ComputeWorkspaceID(wsDir)

	reg := &workspace.Registry{}
	_ = reg.RegisterWithAlias(wsDir, "ws", "")
	_ = reg.SetActive(wsDir)

	mgr := NewWorkspaceManager(reg, func(ctx context.Context, ws *workspace.Workspace) (*WorkspaceServices, error) {
		return &WorkspaceServices{closer: func() error { return nil }}, nil
	})
	if _, err := mgr.LoadActive(context.Background()); err != nil {
		t.Fatalf("LoadActive: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() }) //nolint:errcheck
	return mgr, id, wsDir
}

// TestWorkspaceScopeRewriteActive verifies that a scoped URL for the active
// workspace is rewritten to the legacy /api/<rest> form.
func TestWorkspaceScopeRewriteActive(t *testing.T) {
	mgr, id, _ := newScopeTestManager(t)

	inner := &dummyHandler{}
	h := WorkspaceScope(inner, mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+id+"/agents", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if inner.seen != "/api/agents" {
		t.Errorf("URL not rewritten: got %q want %q", inner.seen, "/api/agents")
	}
	if dep := rec.Header().Get("Deprecation"); dep != "" {
		t.Errorf("scoped route should not carry Deprecation header, got %q", dep)
	}
}

// TestWorkspaceScopeNonActiveReturns501 verifies that scoping into a
// registered-but-not-active workspace returns 501.
func TestWorkspaceScopeNonActiveReturns501(t *testing.T) {
	mgr, _, _ := newScopeTestManager(t)

	// Register a second workspace (not active).
	tmpDir := t.TempDir()
	wsDir2 := filepath.Join(tmpDir, "ws2")
	if err := os.MkdirAll(wsDir2, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := workspace.Init(wsDir2); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_ = mgr.Registry().RegisterWithAlias(wsDir2, "ws2", "")
	id2 := workspace.ComputeWorkspaceID(wsDir2)

	inner := &dummyHandler{}
	h := WorkspaceScope(inner, mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+id2+"/agents", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
	if inner.seen != "" {
		t.Errorf("inner handler should not run for non-active, saw %q", inner.seen)
	}
}

// TestWorkspaceScopeUnknownWorkspace returns 404.
func TestWorkspaceScopeUnknownWorkspace(t *testing.T) {
	mgr, _, _ := newScopeTestManager(t)

	inner := &dummyHandler{}
	h := WorkspaceScope(inner, mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/deadbeef1234/agents", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestWorkspaceScopeSelfRoutePassThrough ensures /api/workspaces/{id} and
// /api/workspaces/{id}/activate go straight through to the mux (registry
// self-routes) without URL rewrite.
func TestWorkspaceScopeSelfRoutePassThrough(t *testing.T) {
	mgr, id, _ := newScopeTestManager(t)

	for _, path := range []string{
		"/api/workspaces/" + id,
		"/api/workspaces/" + id + "/activate",
	} {
		inner := &dummyHandler{}
		h := WorkspaceScope(inner, mgr)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if inner.seen != path {
			t.Errorf("self route %q rewrote to %q", path, inner.seen)
		}
	}
}

// TestWorkspaceScopeLegacyDeprecation ensures legacy /api/ routes get
// Deprecation + Sunset headers.
func TestWorkspaceScopeLegacyDeprecation(t *testing.T) {
	mgr, _, _ := newScopeTestManager(t)

	inner := &dummyHandler{}
	h := WorkspaceScope(inner, mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get("Deprecation") != "true" {
		t.Errorf("Deprecation header missing")
	}
	if rec.Header().Get("Sunset") == "" {
		t.Errorf("Sunset header missing")
	}
	if inner.seen != "/api/agents" {
		t.Errorf("legacy path rewritten: %q", inner.seen)
	}
}

// TestWorkspaceScopeNonAPIPassThrough ensures non-API paths are untouched.
func TestWorkspaceScopeNonAPIPassThrough(t *testing.T) {
	mgr, _, _ := newScopeTestManager(t)

	inner := &dummyHandler{}
	h := WorkspaceScope(inner, mgr)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if inner.seen != "/" {
		t.Errorf("non-API path affected: %q", inner.seen)
	}
	if rec.Header().Get("Deprecation") != "" {
		t.Errorf("non-API should not get Deprecation")
	}
}
