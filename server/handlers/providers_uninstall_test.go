package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/provider"
)

// countingInstallProvider counts IsInstalled/Version invocations so tests can
// assert the TTL cache in fetchInstallStatus actually avoids re-exec'ing on
// every request instead of just returning a plausible-looking value.
type countingInstallProvider struct {
	name            string
	version         string
	isInstalledHits atomic.Int32
	versionHits     atomic.Int32
	installed       bool
}

func (f *countingInstallProvider) Name() string        { return f.name }
func (f *countingInstallProvider) Description() string { return "fake" }
func (f *countingInstallProvider) Command() string     { return f.name }
func (f *countingInstallProvider) Binary() string      { return f.name }
func (f *countingInstallProvider) InstallHint() string { return "npm install -g " + f.name }
func (f *countingInstallProvider) BuildCommand(provider.CommandOpts) string {
	return f.name
}
func (f *countingInstallProvider) IsInstalled(context.Context) bool {
	f.isInstalledHits.Add(1)
	return f.installed
}
func (f *countingInstallProvider) Version(context.Context) string {
	f.versionHits.Add(1)
	return f.version
}

// TestFetchInstallStatus_CachedWithinTTL exercises the cache added to
// buildProviderInfo (server/handlers/providers.go): a provider's
// IsInstalled/Version must be exec'd once per TTL window, not once per
// GET /api/providers request, mirroring the existing fetchModels cache.
func TestFetchInstallStatus_CachedWithinTTL(t *testing.T) {
	fake := &countingInstallProvider{name: "counted", installed: true, version: "1.2.3"}
	reg := provider.NewRegistry()
	reg.Register(fake)
	mux := http.NewServeMux()
	NewProviderHandler(reg, nil, nil, nil).Register(mux)

	for range 3 {
		req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
	}

	if got := fake.isInstalledHits.Load(); got != 1 {
		t.Errorf("IsInstalled called %d times across 3 requests within TTL, want 1", got)
	}
	if got := fake.versionHits.Load(); got != 1 {
		t.Errorf("Version called %d times across 3 requests within TTL, want 1", got)
	}
}

// TestFetchInstallStatus_ReportsInstalledAndVersion is a sanity check that
// the cached path still returns real data end to end.
func TestFetchInstallStatus_ReportsInstalledAndVersion(t *testing.T) {
	fake := &countingInstallProvider{name: "counted2", installed: true, version: "9.9.9"}
	reg := provider.NewRegistry()
	reg.Register(fake)
	mux := http.NewServeMux()
	NewProviderHandler(reg, nil, nil, nil).Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var infos []ProviderInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &infos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(infos) != 1 || !infos[0].Installed || infos[0].Version != "9.9.9" {
		t.Fatalf("infos = %+v, want installed=true version=9.9.9", infos)
	}
}

func newUninstallMux(p provider.Provider, h *home.Home) http.Handler {
	reg := provider.NewRegistry()
	reg.Register(p)
	mux := http.NewServeMux()
	NewProviderHandler(reg, nil, nil, h).Register(mux)
	return mux
}

// TestProviderUninstall_RefusesDefaultProvider covers the "required
// provider" guard: uninstall must refuse the provider currently configured
// as providers.default, since a running daemon needs it to spawn agents.
func TestProviderUninstall_RefusesDefaultProvider(t *testing.T) {
	h := &home.Home{Config: &home.Config{Providers: home.ProvidersConfig{Default: "fakenpm"}}}
	mux := newUninstallMux(&fakeUpdateProvider{name: "fakenpm", installHint: "npm install -g fake-cli", version: "1.0.0"}, h)

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakenpm/uninstall", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for the default provider: %s", rec.Code, rec.Body.String())
	}
}

// TestProviderUninstall_NonDefaultRunsRealCommand covers the happy path: a
// provider that is not the configured default, with an npm install hint,
// gets a derived "npm uninstall -g" command streamed as NDJSON.
func TestProviderUninstall_NonDefaultRunsRealCommand(t *testing.T) {
	orig := installRunner
	installRunner = func(ctx context.Context, _ string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "printf 'removing…\\n'")
	}
	t.Cleanup(func() { installRunner = orig })

	h := &home.Home{Config: &home.Config{Providers: home.ProvidersConfig{Default: "claude"}}}
	mux := newUninstallMux(&fakeUpdateProvider{name: "fakenpm", installHint: "npm install -g fake-cli", version: "1.0.0"}, h)

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakenpm/uninstall", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var sawStart, sawDoneZero bool
	sc := bufio.NewScanner(strings.NewReader(rec.Body.String()))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("bad NDJSON line %q: %v", line, err)
		}
		switch ev["type"] {
		case "start":
			sawStart = true
			if cmd, _ := ev["command"].(string); cmd != "npm uninstall -g fake-cli" {
				t.Errorf("start command = %q, want derived npm uninstall command", cmd)
			}
		case "done":
			if code, ok := ev["code"].(float64); ok && code == 0 {
				sawDoneZero = true
			}
		case "error":
			t.Fatalf("unexpected error event: %v", ev["error"])
		}
	}
	if !sawStart {
		t.Error("missing start event")
	}
	if !sawDoneZero {
		t.Error("missing done event with code 0")
	}
}

// TestProviderUninstall_NoAutomaticUninstaller covers an install hint that
// doesn't map to an unambiguous uninstall command (curl-piped script):
// uninstall must refuse rather than guessing a destructive command.
func TestProviderUninstall_NoAutomaticUninstaller(t *testing.T) {
	h := &home.Home{Config: &home.Config{Providers: home.ProvidersConfig{Default: "claude"}}}
	mux := newUninstallMux(&fakeUpdateProvider{
		name:        "fakecurl",
		installHint: "curl -fsSL https://example.com/install.sh | sh",
		version:     "1.0.0",
	}, h)

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakecurl/uninstall", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a non-derivable uninstall hint", rec.Code)
	}
}

// TestProviderUninstall_LoopbackGuard confirms uninstall shares update's
// loopback-only gate.
func TestProviderUninstall_LoopbackGuard(t *testing.T) {
	h := &home.Home{Config: &home.Config{Providers: home.ProvidersConfig{Default: "claude"}}}
	mux := newUninstallMux(&fakeUpdateProvider{name: "fakenpm", installHint: "npm install -g fake-cli", version: "1.0.0"}, h)

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakenpm/uninstall", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a non-loopback caller", rec.Code)
	}
}
