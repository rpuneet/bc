package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPackageManagersDetect stubs the host so the probe is deterministic:
// only "npm" and "brew" resolve on PATH, each reporting a canned version.
func TestPackageManagersDetect(t *testing.T) {
	origLook, origVer := pmLookPath, pmRunVersion
	t.Cleanup(func() { pmLookPath, pmRunVersion = origLook, origVer })

	present := map[string]string{
		"npm":  "10.2.4",
		"brew": "Homebrew 4.2.0",
	}
	pmLookPath = func(binary string) (string, error) {
		if _, ok := present[binary]; ok {
			return "/usr/local/bin/" + binary, nil
		}
		return "", errors.New("not found")
	}
	pmRunVersion = func(_ context.Context, binary, _ string) (string, bool) {
		v, ok := present[binary]
		return v, ok
	}

	h := NewPackageManagersHandler()
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/system/package-managers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got struct {
		OS       string           `json:"os"`
		Arch     string           `json:"arch"`
		Managers []PackageManager `json:"managers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.OS == "" || got.Arch == "" {
		t.Fatalf("os/arch missing: %+v", got)
	}

	byID := map[string]PackageManager{}
	for _, m := range got.Managers {
		byID[m.ID] = m
	}
	// npm is probed on every OS, so it must always appear with our stub.
	npm, ok := byID["npm"]
	if !ok {
		t.Fatalf("npm not detected; managers=%+v", got.Managers)
	}
	if npm.Version != "10.2.4" || !npm.Available || !npm.Searchable {
		t.Fatalf("npm entry wrong: %+v", npm)
	}
	// A manager not on PATH must never be reported.
	if _, ok := byID["pacman"]; ok {
		t.Fatalf("pacman reported despite not being on PATH")
	}
}

// TestPackageManagersMethodGuard rejects non-GET.
func TestPackageManagersMethodGuard(t *testing.T) {
	h := NewPackageManagersHandler()
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/system/package-managers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("POST should be rejected, got %d", rec.Code)
	}
}
