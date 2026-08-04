package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/rpuneet/mycel/pkg/home"
)

func TestSystemInfoReportsWorkspace(t *testing.T) {
	dir := t.TempDir()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	// StatsHandler only reads RootDir — no need to bootstrap a full Home.
	h := &home.Home{RootDir: abs}
	sh := NewStatsHandler(nil, nil, nil, h, nil)
	mux := http.NewServeMux()
	sh.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/system/info", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["has_workspace"] != true {
		t.Fatalf("has_workspace = %v, want true", body["has_workspace"])
	}
	if body["workspace"] != abs {
		t.Fatalf("workspace = %v, want %q", body["workspace"], abs)
	}
}

func TestSystemInfoReportsNoWorkspace(t *testing.T) {
	h := &home.Home{RootDir: ""}
	sh := NewStatsHandler(nil, nil, nil, h, nil)
	mux := http.NewServeMux()
	sh.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/system/info", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["has_workspace"] != false {
		t.Fatalf("has_workspace = %v, want false", body["has_workspace"])
	}
	if ws, _ := body["workspace"].(string); ws != "" {
		t.Fatalf("workspace = %q, want empty", ws)
	}
}
