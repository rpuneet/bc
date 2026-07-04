package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rpuneet/mycel/pkg/workspace"
)

// newTestRegistry creates a temp registry with two workspaces and sets one
// active. Uses workspace.LoadRegistry under a temp HOME so Save() writes to
// an isolated location.
func newTestRegistry(t *testing.T) (*workspace.Registry, string, string) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	// Fresh registry per test — MYCEL_HOME is otherwise pinned
	// package-wide by TestMain, which would share aliases across tests.
	t.Setenv("MYCEL_HOME", filepath.Join(tmpDir, ".mycel"))

	a := filepath.Join(tmpDir, "a")
	b := filepath.Join(tmpDir, "b")
	for _, p := range []string{a, b} {
		if err := os.MkdirAll(filepath.Join(p, ".bc"), 0750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	reg, err := workspace.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	_ = reg.RegisterWithAlias(a, "alpha", "al")
	_ = reg.RegisterWithAlias(b, "beta", "")
	_ = reg.SetActive(a)
	return reg, workspace.ComputeWorkspaceID(a), workspace.ComputeWorkspaceID(b)
}

func TestWorkspacesHandlerList(t *testing.T) {
	reg, idA, idB := newTestRegistry(t)
	h := NewWorkspacesHandler(reg, nil)

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Active     string              `json:"active"`
		Workspaces []registryEntryView `json:"workspaces"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Workspaces) != 2 {
		t.Fatalf("workspaces = %d, want 2", len(body.Workspaces))
	}
	if body.Active != idA {
		t.Errorf("active = %q, want %q", body.Active, idA)
	}
	var sawA, sawB bool
	for _, ws := range body.Workspaces {
		switch ws.ID {
		case idA:
			sawA = true
			if !ws.Active {
				t.Error("workspace a should be marked active")
			}
		case idB:
			sawB = true
			if ws.Active {
				t.Error("workspace b should NOT be marked active")
			}
		}
	}
	if !sawA || !sawB {
		t.Error("expected both workspaces in response")
	}
}

func TestWorkspacesHandlerAdd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("MYCEL_HOME", filepath.Join(tmpDir, ".mycel"))

	reg, err := workspace.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	h := NewWorkspacesHandler(reg, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	// Create a real directory so the "does not exist" check passes.
	target := filepath.Join(tmpDir, "new-ws")
	if err := os.MkdirAll(target, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"path": target, "name": "new-ws"})
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var view registryEntryView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if view.ID == "" {
		t.Error("response missing ID")
	}
	if len(reg.List()) != 1 {
		t.Errorf("registry count = %d, want 1", len(reg.List()))
	}
}

func TestWorkspacesHandlerAddMissingPath(t *testing.T) {
	reg := &workspace.Registry{}
	h := NewWorkspacesHandler(reg, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestWorkspacesHandlerDetail(t *testing.T) {
	reg, idA, _ := newTestRegistry(t)
	h := NewWorkspacesHandler(reg, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+idA, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Workspace registryEntryView `json:"workspace"`
		Worktrees []string          `json:"worktrees"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Workspace.ID != idA {
		t.Errorf("ID = %q, want %q", body.Workspace.ID, idA)
	}
}

func TestWorkspacesHandlerDetailNotFound(t *testing.T) {
	reg, _, _ := newTestRegistry(t)
	h := NewWorkspacesHandler(reg, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/deadbeef9999", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestWorkspacesHandlerActivate(t *testing.T) {
	reg, _, idB := newTestRegistry(t)
	h := NewWorkspacesHandler(reg, nil)
	refreshed := false
	h.SetActiveRefreshHook(func() { refreshed = true })
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces/"+idB+"/activate", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !refreshed {
		t.Error("activeRefresh hook not called")
	}
	if active := reg.GetActive(); active == nil || active.ID != idB {
		t.Errorf("active not updated: %+v", active)
	}
}

func TestWorkspacesHandlerUnregister(t *testing.T) {
	reg, _, idB := newTestRegistry(t)
	h := NewWorkspacesHandler(reg, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/workspaces/"+idB, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if reg.FindByID(idB) != nil {
		t.Error("workspace still registered after DELETE")
	}
}

func TestWorkspacesHandlerPatch(t *testing.T) {
	reg, idA, _ := newTestRegistry(t)
	h := NewWorkspacesHandler(reg, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	body, _ := json.Marshal(map[string]any{
		"name":       "renamed",
		"github_url": "https://github.com/u/r",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/workspaces/"+idA, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	entry := reg.FindByID(idA)
	if entry == nil || entry.Name != "renamed" || entry.GithubURL != "https://github.com/u/r" {
		t.Errorf("patch did not apply: %+v", entry)
	}
}
