package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rpuneet/mycel/pkg/workspace"
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
	gitInitDir(t, wsDir)
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

// TestWorkspaceScopeQueryParam verifies the ?workspace=<id> query param
// resolves a registered (non-active) workspace and stashes its services
// in the request context.
func TestWorkspaceScopeQueryParam(t *testing.T) {
	mgr, _, _ := newScopeTestManager(t)

	// Register a second (non-active) workspace.
	tmpDir := t.TempDir()
	wsDir2 := filepath.Join(tmpDir, "ws2")
	if err := os.MkdirAll(wsDir2, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	gitInitDir(t, wsDir2)
	if _, err := workspace.Init(wsDir2); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_ = mgr.Registry().RegisterWithAlias(wsDir2, "ws2", "")
	id2 := workspace.ComputeWorkspaceID(wsDir2)

	var gotID string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID = WorkspaceIDFromContext(r.Context())
	})
	h := WorkspaceScope(inner, mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/agents?workspace="+id2, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if gotID != id2 {
		t.Errorf("ctx workspace id = %q, want %q", gotID, id2)
	}
}

// TestWorkspaceScopeHeader verifies X-BC-Workspace overrides the active
// workspace scope.
func TestWorkspaceScopeHeader(t *testing.T) {
	mgr, _, _ := newScopeTestManager(t)

	tmpDir := t.TempDir()
	wsDir2 := filepath.Join(tmpDir, "ws2")
	if err := os.MkdirAll(wsDir2, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	gitInitDir(t, wsDir2)
	if _, err := workspace.Init(wsDir2); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_ = mgr.Registry().RegisterWithAlias(wsDir2, "ws2", "")
	id2 := workspace.ComputeWorkspaceID(wsDir2)

	var gotID string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID = WorkspaceIDFromContext(r.Context())
	})
	h := WorkspaceScope(inner, mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	req.Header.Set("X-BC-Workspace", id2)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if gotID != id2 {
		t.Errorf("ctx workspace id = %q, want %q", gotID, id2)
	}
}

// TestWorkspaceScopeUnknownWorkspace returns 404 for an unregistered id.
func TestWorkspaceScopeUnknownWorkspace(t *testing.T) {
	mgr, _, _ := newScopeTestManager(t)

	inner := &dummyHandler{}
	h := WorkspaceScope(inner, mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/agents?workspace=deadbeef1234", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestWorkspaceScopeSelfRoutePassThrough ensures /api/workspaces/... self
// routes go straight through to the mux (registry self-routes) without
// any per-request scope injection.
func TestWorkspaceScopeSelfRoutePassThrough(t *testing.T) {
	mgr, id, _ := newScopeTestManager(t)

	for _, path := range []string{
		"/api/workspaces",
		"/api/workspaces/" + id,
		"/api/workspaces/" + id + "/activate",
		"/api/workspaces/discover/local",
		"/api/workspaces/clone",
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

// TestWorkspaceScopeDefaultsToActive ensures flat /api/ paths with no
// explicit hint resolve to the active workspace.
func TestWorkspaceScopeDefaultsToActive(t *testing.T) {
	mgr, activeID, _ := newScopeTestManager(t)

	var gotID string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID = WorkspaceIDFromContext(r.Context())
	})
	h := WorkspaceScope(inner, mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if gotID != activeID {
		t.Errorf("ctx workspace id = %q, want active %q", gotID, activeID)
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
}
