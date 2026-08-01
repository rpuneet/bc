package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/provider"
)

func providerRunMux() http.Handler {
	reg := provider.NewRegistry()
	reg.Register(provider.NewClaudeProvider())
	mux := http.NewServeMux()
	NewProviderHandler(reg, nil, nil, nil).Register(mux)
	return mux
}

func runReq(command string) *http.Request {
	body, _ := json.Marshal(map[string]string{"command": command})
	req := httptest.NewRequest(http.MethodPost, "/api/providers/claude/run", strings.NewReader(string(body)))
	req.RemoteAddr = "127.0.0.1:5555"
	return req
}

// TestProviderRunRunnable executes an allowlisted, non-interactive, no-arg
// command and checks the exact argv came from the curated table.
func TestProviderRunRunnable(t *testing.T) {
	orig := providerRunRunner
	t.Cleanup(func() { providerRunRunner = orig })

	var gotBin string
	var gotArgs []string
	providerRunRunner = func(_ context.Context, bin string, args []string) (string, int, bool, bool, error) {
		gotBin, gotArgs = bin, args
		return "claude 1.2.3\n", 0, false, false, nil
	}

	rec := httptest.NewRecorder()
	providerRunMux().ServeHTTP(rec, runReq("version"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if gotBin != "claude" || strings.Join(gotArgs, " ") != "--version" {
		t.Fatalf("argv = %q %q, want claude [--version]", gotBin, gotArgs)
	}
	var resp struct {
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.Output, "claude 1.2.3") {
		t.Fatalf("output = %q", resp.Output)
	}
}

// TestProviderRunRejectsInteractiveAndArgs asserts the allowlist gate: only
// runnable entries execute; interactive, arg-taking, and unknown commands are
// refused without ever invoking the runner.
func TestProviderRunRejectsInteractiveAndArgs(t *testing.T) {
	orig := providerRunRunner
	t.Cleanup(func() { providerRunRunner = orig })
	ran := false
	providerRunRunner = func(_ context.Context, _ string, _ []string) (string, int, bool, bool, error) {
		ran = true
		return "", 0, false, false, nil
	}

	mux := providerRunMux()
	for _, name := range []string{
		"resume",  // interactive + args
		"mcp add", // requires args
		"nope",    // not in the allowlist
		"",        // empty
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, runReq(name))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("command %q: status = %d, want 400", name, rec.Code)
		}
	}
	if ran {
		t.Fatalf("runner executed a non-runnable command — allowlist bypassed")
	}
}

// TestProviderRunArgvIsConstant asserts the executed argv comes only from the
// curated Commands() literal — a crafted command field (extra tokens, shell
// metacharacters, or a name that merely resembles a real one) can never alter
// or extend the argv. The request only ever *selects* an allowlisted entry.
func TestProviderRunArgvIsConstant(t *testing.T) {
	orig := providerRunRunner
	t.Cleanup(func() { providerRunRunner = orig })

	var calls int
	var gotBin string
	var gotArgs []string
	providerRunRunner = func(_ context.Context, bin string, args []string) (string, int, bool, bool, error) {
		calls++
		gotBin, gotArgs = bin, args
		return "ok", 0, false, false, nil
	}

	mux := providerRunMux()

	// Crafted selectors that must NOT match any curated entry Name → 400, no exec.
	for _, crafted := range []string{
		"version; rm -rf /",
		"version --dangerous",
		"version\n--evil",
		"VERSION",
		"claude --version",
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, runReq(crafted))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("crafted %q: status = %d, want 400 (no match)", crafted, rec.Code)
		}
	}
	if calls != 0 {
		t.Fatalf("runner executed for a crafted selector — argv could be influenced by the request")
	}

	// The exact allowlisted name runs the exact constant argv, nothing more.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, runReq("version"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotBin != "claude" || strings.Join(gotArgs, " ") != "--version" {
		t.Fatalf("argv = %q %q, want exactly claude [--version]", gotBin, gotArgs)
	}
}

// TestProviderRunNonZeroExit returns the CLI's exit code without failing the
// request — a non-zero exit is a legitimate command result, not a 5xx.
func TestProviderRunNonZeroExit(t *testing.T) {
	orig := providerRunRunner
	t.Cleanup(func() { providerRunRunner = orig })
	providerRunRunner = func(_ context.Context, _ string, _ []string) (string, int, bool, bool, error) {
		return "not logged in\n", 3, false, false, nil
	}

	rec := httptest.NewRecorder()
	providerRunMux().ServeHTTP(rec, runReq("version"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		ExitCode int  `json:"exit_code"`
		TimedOut bool `json:"timed_out"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ExitCode != 3 || resp.TimedOut {
		t.Fatalf("exit_code = %d timed_out = %v, want 3 / false", resp.ExitCode, resp.TimedOut)
	}
}

// TestProviderRunTruncated propagates the runner's truncation flag.
func TestProviderRunTruncated(t *testing.T) {
	orig := providerRunRunner
	t.Cleanup(func() { providerRunRunner = orig })
	providerRunRunner = func(_ context.Context, _ string, _ []string) (string, int, bool, bool, error) {
		return strings.Repeat("x", providerRunOutputCap), 0, false, true, nil
	}

	rec := httptest.NewRecorder()
	providerRunMux().ServeHTTP(rec, runReq("version"))
	var resp struct {
		Output    string `json:"output"`
		Truncated bool   `json:"truncated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Output) != providerRunOutputCap || !resp.Truncated {
		t.Fatalf("output len = %d, truncated = %v; want cap / true", len(resp.Output), resp.Truncated)
	}
}

// TestProviderRunTimedOut surfaces a timeout as timed_out=true (not a bogus
// exit code) so the UI can explain the cause.
func TestProviderRunTimedOut(t *testing.T) {
	orig := providerRunRunner
	t.Cleanup(func() { providerRunRunner = orig })
	providerRunRunner = func(_ context.Context, _ string, _ []string) (string, int, bool, bool, error) {
		return "", -1, true, false, nil
	}

	rec := httptest.NewRecorder()
	providerRunMux().ServeHTTP(rec, runReq("version"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		TimedOut bool `json:"timed_out"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.TimedOut {
		t.Fatalf("timed_out = false, want true")
	}
}

// TestProviderRunLoopbackGuard rejects non-loopback callers.
func TestProviderRunLoopbackGuard(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"command": "version"})
	req := httptest.NewRequest(http.MethodPost, "/api/providers/claude/run", strings.NewReader(string(body)))
	req.RemoteAddr = "203.0.113.9:8080"
	rec := httptest.NewRecorder()
	providerRunMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// TestProviderRunUnknownProvider 404s for an unregistered provider.
func TestProviderRunUnknownProvider(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"command": "version"})
	req := httptest.NewRequest(http.MethodPost, "/api/providers/ghost/run", strings.NewReader(string(body)))
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	providerRunMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
