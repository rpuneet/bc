package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/mcp"
)

func newTestMCPStore(t *testing.T) *mcp.Store {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(dir + "/bc.db")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	store, err := mcp.NewStore(d, "sqlite")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestMCPHandler_PatchEnv_Updates(t *testing.T) {
	store := newTestMCPStore(t)
	if err := store.Add(&mcp.ServerConfig{
		Name:      "github",
		Transport: mcp.TransportStdio,
		Command:   "npx",
		Env:       map[string]string{"STALE": "old"},
		Enabled:   true,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := NewMCPHandler(store)
	body, _ := json.Marshal(map[string]any{
		"env": map[string]string{"GITHUB_TOKEN": "ghp_new", "DEBUG": "1"},
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/mcp/github", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.Register(mux)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got mcp.ServerConfig
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Env["GITHUB_TOKEN"] != "ghp_new" || got.Env["DEBUG"] != "1" {
		t.Errorf("env not updated: %+v", got.Env)
	}
	if _, lingering := got.Env["STALE"]; lingering {
		t.Error("PATCH should replace env wholesale, not merge")
	}
}

func TestMCPHandler_PatchEnv_DropsEmptyValues(t *testing.T) {
	store := newTestMCPStore(t)
	if err := store.Add(&mcp.ServerConfig{
		Name: "slack", Transport: mcp.TransportStdio, Command: "npx",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := NewMCPHandler(store)
	body, _ := json.Marshal(map[string]any{
		"env": map[string]string{"KEEP": "yes", "GONE": ""},
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/mcp/slack", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.Register(mux)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	cfg, _ := store.Get("slack")
	if cfg.Env["KEEP"] != "yes" {
		t.Errorf("KEEP lost: %+v", cfg.Env)
	}
	if _, present := cfg.Env["GONE"]; present {
		t.Errorf("empty-value key not dropped: %+v", cfg.Env)
	}
}

func TestMCPHandler_PatchEnv_UnknownServer(t *testing.T) {
	h := NewMCPHandler(newTestMCPStore(t))
	body, _ := json.Marshal(map[string]any{"env": map[string]string{}})
	req := httptest.NewRequest(http.MethodPatch, "/api/mcp/ghost", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.Register(mux)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("code = %d, want 404", rec.Code)
	}
}

func TestMCPHandler_PatchEnv_MissingEnvField(t *testing.T) {
	store := newTestMCPStore(t)
	_ = store.Add(&mcp.ServerConfig{Name: "linear", Transport: mcp.TransportStdio, Command: "x"})
	h := NewMCPHandler(store)
	req := httptest.NewRequest(http.MethodPatch, "/api/mcp/linear", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.Register(mux)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}
