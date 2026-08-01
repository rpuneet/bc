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
	providerRunRunner = func(_ context.Context, bin string, args []string) (string, int, error) {
		gotBin, gotArgs = bin, args
		return "claude 1.2.3\n", 0, nil
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
	providerRunRunner = func(_ context.Context, _ string, _ []string) (string, int, error) {
		ran = true
		return "", 0, nil
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
