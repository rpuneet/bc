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

func TestDiscoverLocalHandler(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("MYCEL_HOME", filepath.Join(tmp, ".mycel"))
	// Build a fake filesystem: two git repos under one root.
	for _, name := range []string{"alpha", "fresh"} {
		if err := os.MkdirAll(filepath.Join(tmp, "src", name, ".git"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	h := NewDiscoveryHandler()
	mux := http.NewServeMux()
	h.Register(mux)

	body, _ := json.Marshal(map[string]any{"root": filepath.Join(tmp, "src"), "depth": 2})
	req := httptest.NewRequest(http.MethodPost, "/api/repos/discover/local", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Candidates []workspace.Candidate `json:"candidates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(response.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(response.Candidates))
	}
}

func TestDiscoverLocalMissingRoot(t *testing.T) {
	h := NewDiscoveryHandler()
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/repos/discover/local", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestGithubAuthStatusDisconnected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	h := NewDiscoveryHandler()
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/github", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Connected bool `json:"connected"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Connected {
		t.Error("should report disconnected")
	}
}

func TestGithubAuthSetAndDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Stub api.github.com so ValidateGithubToken accepts testtoken.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer ok" {
			_, _ = w.Write([]byte(`{"login":"puneet"}`)) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	restore := workspace.SetGithubAPIBase(srv.URL)
	defer restore()

	h := NewDiscoveryHandler()
	mux := http.NewServeMux()
	h.Register(mux)

	// POST with good token succeeds.
	body, _ := json.Marshal(map[string]string{"token": "ok"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/github", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d body=%s", rec.Code, rec.Body.String())
	}

	// GET reports connected.
	req = httptest.NewRequest(http.MethodGet, "/api/auth/github", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var status struct{ Connected bool }
	_ = json.Unmarshal(rec.Body.Bytes(), &status) //nolint:errcheck
	if !status.Connected {
		t.Error("should be connected")
	}

	// POST with bad token fails with 401.
	body, _ = json.Marshal(map[string]string{"token": "nope"})
	req = httptest.NewRequest(http.MethodPost, "/api/auth/github", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("bad token POST status = %d, want 401", rec.Code)
	}

	// DELETE clears.
	req = httptest.NewRequest(http.MethodDelete, "/api/auth/github", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("DELETE status = %d, want 204", rec.Code)
	}
	if workspace.GithubConnected() {
		t.Error("should be disconnected after DELETE")
	}
}

func TestDiscoverGithubUnauthenticated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	h := NewDiscoveryHandler()
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/repos/discover/github", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestCloneValidatesInput(t *testing.T) {
	h := NewDiscoveryHandler()
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/repos/clone", bytes.NewReader([]byte(`{"url":""}`)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
