// degraded_test.go — issue #3240: silent service degradation must become
// loud and diagnosable. Covers the /api/health degraded shape and the
// Degraded map populated by BuildServices when a store fails to
// initialize (warn-and-continue sites).
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbpkg "github.com/rpuneet/mycel/pkg/db"
)

// TestAPIHealthDegradedShape verifies that /api/health reports
// {"status":"degraded","degraded":{...}} when services failed to
// initialize, and keeps the plain "ok" shape when healthy.
func TestAPIHealthDegradedShape(t *testing.T) {
	cfg := Config{Addr: "127.0.0.1:0", CORS: true}
	svc := Services{Degraded: map[string]string{
		"notify": "notify store unavailable: no shared database",
		"events": "event log unavailable: disk full",
	}}
	ts := httptest.NewServer(New(cfg, svc, nil, nil).Handler())
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degraded is still serving)", resp.StatusCode)
	}
	var body struct {
		Degraded map[string]string `json:"degraded"`
		Status   string            `json:"status"`
		DB       string            `json:"db"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "degraded" {
		t.Errorf("status = %q, want degraded", body.Status)
	}
	if len(body.Degraded) != 2 {
		t.Fatalf("degraded = %v, want 2 entries", body.Degraded)
	}
	if !strings.Contains(body.Degraded["notify"], "notify store unavailable") {
		t.Errorf("degraded.notify = %q, reason missing", body.Degraded["notify"])
	}
}

// TestAPIHealthOKShapeUnchanged verifies the healthy response keeps the
// original {"status":"ok",...} shape with no degraded key.
func TestAPIHealthOKShapeUnchanged(t *testing.T) {
	cfg := Config{Addr: "127.0.0.1:0", CORS: true}
	ts := httptest.NewServer(New(cfg, Services{}, nil, nil).Handler())
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if _, present := body["degraded"]; present {
		t.Errorf("healthy response must not include a degraded key: %v", body)
	}
}

// TestBuildServicesGlobalDBFailure verifies that when the
// single global database cannot open (broken MYCEL_HOME), the factory
// fails loudly instead of silently coming up with nil stores. With the
// per-repo databases gone, a dead mycel.db is fatal for the
// service bundle — home init itself needs the roles table.
func TestBuildServicesGlobalDBFailure(t *testing.T) {
	t.Setenv("MYCEL_SECRET_PASSPHRASE", "unit-test")

	// Plant a regular FILE at the MYCEL_HOME path so MkdirAll (and
	// therefore the global mycel.db open) fails.
	tmp := t.TempDir()
	homeFile := filepath.Join(tmp, "mycel-home")
	if err := os.WriteFile(homeFile, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("plant home file: %v", err)
	}
	t.Setenv("MYCEL_HOME", homeFile)
	t.Cleanup(func() { _ = dbpkg.CloseGlobal() })

	wsDir := t.TempDir()
	gitInitDir(t, wsDir)
	if _, err := BuildServices(context.Background(), &Globals{}, wsDir); err == nil {
		t.Fatal("expected BuildServices to fail when the global database cannot open")
	}
}

// TestDegradedMapSurfacedByHealth verifies the Degraded map recorded by
// the factory is the same map /api/health surfaces on the wired server.
func TestDegradedMapSurfacedByHealth(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	t.Setenv("MYCEL_SECRET_PASSPHRASE", "unit-test")
	t.Cleanup(func() { _ = dbpkg.CloseGlobal() })

	wsDir := t.TempDir()
	gitInitDir(t, wsDir)
	svc, err := BuildServices(context.Background(), &Globals{}, wsDir)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer svc.Close() //nolint:errcheck

	reason := "notify store unavailable: injected for surfacing test"
	svc.Degraded["notify"] = reason

	ts := httptest.NewServer(New(Config{Addr: "127.0.0.1:0", CORS: true}, *svc, nil, nil).Handler())
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body struct {
		Degraded map[string]string `json:"degraded"`
		Status   string            `json:"status"`
	}
	if decErr := json.NewDecoder(resp.Body).Decode(&body); decErr != nil {
		t.Fatal(decErr)
	}
	if body.Status != "degraded" {
		t.Errorf("status = %q, want degraded", body.Status)
	}
	if body.Degraded["notify"] != reason {
		t.Errorf("/api/health dropped the injected reason: %#v", body.Degraded)
	}
}
