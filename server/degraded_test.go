// degraded_test.go — issue #3240: silent service degradation must become
// loud and diagnosable. Covers the /api/health degraded shape and the
// Degraded map populated by BuildWorkspaceServices when a store fails to
// initialize (warn-and-continue sites).
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bcdb "github.com/rpuneet/mycel/pkg/db"
)

// TestAPIHealthDegradedShape verifies that /api/health reports
// {"status":"degraded","degraded":{...}} when services failed to
// initialize, and keeps the plain "ok" shape when healthy.
func TestAPIHealthDegradedShape(t *testing.T) {
	cfg := Config{Addr: "127.0.0.1:0", CORS: true}
	svc := Services{Degraded: map[string]string{
		"notify": "notify store unavailable: no shared database",
		"cron":   "cron store unavailable: disk full",
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

// TestBuildWorkspaceServicesDegradedNotify verifies that when the notify
// store cannot open (no shared database — the exact failure behind the
// 2026-07-03 Slack-delivery outage), the factory records the reason in
// Degraded instead of only logging a warning.
func TestBuildWorkspaceServicesDegradedNotify(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")

	// Force the shared DB to be absent so notify's OpenStore fails,
	// restoring whatever a previous test installed.
	prevDB, prevDriver := bcdb.Shared(), bcdb.SharedDriver()
	bcdb.SetShared(nil, "")
	t.Cleanup(func() { bcdb.SetShared(prevDB, prevDriver) })

	wsDir := t.TempDir()
	gitInitDir(t, wsDir)
	svc, err := BuildWorkspaceServices(context.Background(), &Globals{}, wsDir)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer svc.Close() //nolint:errcheck

	if svc.Notify != nil {
		t.Fatal("expected nil Notify service without a shared database")
	}
	reason, ok := svc.Degraded["notify"]
	if !ok || reason == "" {
		t.Fatalf("Degraded[notify] not populated: %#v", svc.Degraded)
	}
	if !strings.Contains(reason, "notify store unavailable") {
		t.Errorf("Degraded[notify] = %q, want it to explain the store failure", reason)
	}

	// The projection consumed by handlers must carry the map through.
	view := workspaceViewFromServices(svc)
	if view.Degraded["notify"] != reason {
		t.Errorf("workspaceViewFromServices dropped Degraded: %#v", view.Degraded)
	}
	flat := servicesFromWorkspace(svc)
	if flat.Degraded["notify"] != reason {
		t.Errorf("servicesFromWorkspace dropped Degraded: %#v", flat.Degraded)
	}
}
