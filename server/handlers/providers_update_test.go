package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/provider"
)

// fakeUpdateProvider is a minimal provider.Provider stub with a fully
// controllable version/install-hint pair, so checkUpdate/update can be
// exercised without shelling out to a real CLI.
type fakeUpdateProvider struct {
	name        string
	installHint string
	version     string
}

func (f *fakeUpdateProvider) Name() string                             { return f.name }
func (f *fakeUpdateProvider) Description() string                      { return "fake" }
func (f *fakeUpdateProvider) Command() string                          { return f.name }
func (f *fakeUpdateProvider) Binary() string                           { return f.name }
func (f *fakeUpdateProvider) InstallHint() string                      { return f.installHint }
func (f *fakeUpdateProvider) BuildCommand(provider.CommandOpts) string { return f.name }
func (f *fakeUpdateProvider) IsInstalled(context.Context) bool         { return f.version != "" }
func (f *fakeUpdateProvider) Version(context.Context) string           { return f.version }

func newFakeUpdateMux(p provider.Provider) http.Handler {
	reg := provider.NewRegistry()
	reg.Register(p)
	mux := http.NewServeMux()
	NewProviderHandler(reg, nil, nil, nil).Register(mux)
	return mux
}

// withNpmRegistryStub points fetchNpmLatestVersion at a local httptest.Server
// that always returns the given version for "/<pkg>/latest", restoring the
// real npm registry base URL on cleanup.
func withNpmRegistryStub(t *testing.T, version string, statusCode int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode != http.StatusOK {
			w.WriteHeader(statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"version":%q}`, version)
	}))
	t.Cleanup(srv.Close)

	origURL := npmRegistryBaseURL
	npmRegistryBaseURL = srv.URL + "/"
	t.Cleanup(func() { npmRegistryBaseURL = origURL })
}

func TestCheckUpdate_RealNpmCompareUpdateAvailable(t *testing.T) {
	withNpmRegistryStub(t, "9.9.9", http.StatusOK)
	mux := newFakeUpdateMux(&fakeUpdateProvider{name: "fakenpm", installHint: "npm install -g fake-cli", version: "1.0.0"})

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakenpm/check-update", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var result UpdateCheck
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.Checked {
		t.Error("Checked = false, want true (npm registry lookup succeeded)")
	}
	if !result.UpdateAvailable {
		t.Error("UpdateAvailable = false, want true (1.0.0 vs 9.9.9)")
	}
	if result.LatestVersion != "9.9.9" {
		t.Errorf("LatestVersion = %q, want 9.9.9", result.LatestVersion)
	}
	if result.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want 1.0.0", result.CurrentVersion)
	}
}

func TestCheckUpdate_RealNpmCompareUpToDate(t *testing.T) {
	withNpmRegistryStub(t, "1.0.0", http.StatusOK)
	mux := newFakeUpdateMux(&fakeUpdateProvider{name: "fakenpm", installHint: "npm install -g fake-cli", version: "1.0.0"})

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakenpm/check-update", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var result UpdateCheck
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.Checked {
		t.Error("Checked = false, want true")
	}
	if result.UpdateAvailable {
		t.Error("UpdateAvailable = true, want false when versions match")
	}
}

// TestCheckUpdate_DecoratedCurrentVersionNoFalsePositive guards the claude
// regression: Version() returns "2.1.205 (Claude Code)" while npm reports the
// bare "2.1.205". normalizeVersion must reduce both to "2.1.205" so the check
// reports up-to-date instead of a perpetual false "update available".
func TestCheckUpdate_DecoratedCurrentVersionNoFalsePositive(t *testing.T) {
	withNpmRegistryStub(t, "2.1.205", http.StatusOK)
	mux := newFakeUpdateMux(&fakeUpdateProvider{name: "fakenpm", installHint: "npm install -g fake-cli", version: "2.1.205 (Claude Code)"})

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakenpm/check-update", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var result UpdateCheck
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.Checked {
		t.Error("Checked = false, want true")
	}
	if result.UpdateAvailable {
		t.Error("UpdateAvailable = true, want false ('2.1.205 (Claude Code)' vs '2.1.205')")
	}
}

// TestNormalizeVersion checks the semver-token reduction directly.
func TestNormalizeVersion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2.1.205 (Claude Code)", "2.1.205"},
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"  2.1.205  ", "2.1.205"},
		{"codex-cli 0.111.0", "0.111.0"},
		{"weird-nonsemver", "weird-nonsemver"},
	}
	for _, c := range cases {
		if got := normalizeVersion(c.in); got != c.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCheckUpdate_NonNpmHintIsHonestlyUnverified covers a provider installed
// via a mechanism with no queryable registry (e.g. cursor's bare download
// URL). checkUpdate must not claim the version is current — Checked stays
// false so the UI shows "can't verify" rather than a false "up to date".
func TestCheckUpdate_NonNpmHintIsHonestlyUnverified(t *testing.T) {
	mux := newFakeUpdateMux(&fakeUpdateProvider{name: "fakeurl", installHint: "https://example.com/install", version: "1.0.0"})

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakeurl/check-update", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var result UpdateCheck
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Checked {
		t.Error("Checked = true, want false — no registry to query for a bare-URL install hint")
	}
	if result.UpdateAvailable {
		t.Error("UpdateAvailable = true, want false when unverified")
	}
	if result.LatestVersion != "" {
		t.Errorf("LatestVersion = %q, want empty when unverified", result.LatestVersion)
	}
}

func TestCheckUpdate_NpmRegistryErrorLeavesUnchecked(t *testing.T) {
	withNpmRegistryStub(t, "", http.StatusInternalServerError)
	mux := newFakeUpdateMux(&fakeUpdateProvider{name: "fakenpm", installHint: "npm install -g fake-cli", version: "1.0.0"})

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakenpm/check-update", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var result UpdateCheck
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Checked {
		t.Error("Checked = true, want false when the registry call fails")
	}
}

func TestCheckUpdate_NotInstalled(t *testing.T) {
	mux := newFakeUpdateMux(&fakeUpdateProvider{name: "fakenpm", installHint: "npm install -g fake-cli", version: ""})

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakenpm/check-update", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a not-installed provider", rec.Code)
	}
}

// TestProviderUpdate_StreamsRealCommand exercises the real streamed update
// path: it stubs installRunner (shared with /api/deps/install) so no process
// actually runs, and asserts the update handler resolves + streams the
// provider's install command as NDJSON, exactly like a real update would.
func TestProviderUpdate_StreamsRealCommand(t *testing.T) {
	orig := installRunner
	installRunner = func(ctx context.Context, _ string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "printf 'updating…\\n'")
	}
	t.Cleanup(func() { installRunner = orig })

	mux := newFakeUpdateMux(&fakeUpdateProvider{name: "fakenpm", installHint: "npm install -g fake-cli", version: "1.0.0"})

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakenpm/update", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var sawStart, sawDoneZero bool
	var logs []string
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
			if cmd, _ := ev["command"].(string); cmd != "npm install -g fake-cli" {
				t.Errorf("start command = %q, want the provider's install hint", cmd)
			}
		case "log":
			if l, ok := ev["line"].(string); ok {
				logs = append(logs, l)
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
	if !strings.Contains(strings.Join(logs, "|"), "updating…") {
		t.Errorf("logs = %v, want stubbed output", logs)
	}
}

// TestProviderUpdate_NonRunnableHintRefused covers a provider with a bare-URL
// install hint (e.g. cursor) — there is nothing to execute, so update must
// refuse honestly rather than trying to "run" a URL.
func TestProviderUpdate_NonRunnableHintRefused(t *testing.T) {
	mux := newFakeUpdateMux(&fakeUpdateProvider{name: "fakeurl", installHint: "https://example.com/install", version: "1.0.0"})

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakeurl/update", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a non-runnable install hint", rec.Code)
	}
}

func TestProviderUpdate_LoopbackGuard(t *testing.T) {
	mux := newFakeUpdateMux(&fakeUpdateProvider{name: "fakenpm", installHint: "npm install -g fake-cli", version: "1.0.0"})

	req := httptest.NewRequest(http.MethodPost, "/api/providers/fakenpm/update", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a non-loopback caller", rec.Code)
	}
}

// TestProviderUpdate_MethodGuard confirms GET .../update isn't routed at all
// (byName's switch only wires POST to the update action, matching every
// other provider sub-action) rather than silently running an update.
func TestProviderUpdate_MethodGuard(t *testing.T) {
	mux := newFakeUpdateMux(&fakeUpdateProvider{name: "fakenpm", installHint: "npm install -g fake-cli", version: "1.0.0"})

	req := httptest.NewRequest(http.MethodGet, "/api/providers/fakenpm/update", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestNpmPackageForHint(t *testing.T) {
	tests := []struct {
		hint   string
		want   string
		wantOK bool
	}{
		{"npm install -g @openai/codex", "@openai/codex", true},
		{"npm i -g foo", "foo", true},
		{"npx -y @anthropic-ai/claude-code", "@anthropic-ai/claude-code", true},
		{"npx some-cli", "some-cli", true},
		{"curl -fsSL https://antigravity.google/install.sh | sh", "", false},
		{"https://cursor.sh", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := npmPackageForHint(tt.hint)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("npmPackageForHint(%q) = (%q, %v), want (%q, %v)", tt.hint, got, ok, tt.want, tt.wantOK)
		}
	}
}
