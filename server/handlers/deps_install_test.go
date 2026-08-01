package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestInstallCommand(t *testing.T) {
	tests := []struct {
		id     string
		goos   string
		want   string
		wantOK bool
	}{
		{"git", "darwin", "brew install git", true},
		{"git", "linux", "sudo apt-get update && sudo apt-get install -y git", true},
		{"tmux", "darwin", "brew install tmux", true},
		{"tmux", "linux", "sudo apt-get update && sudo apt-get install -y tmux", true},
		{"docker", "darwin", "", false},
		{"docker", "linux", "", false},
		{"claude", "darwin", "npx -y @anthropic-ai/claude-code", true},
		{"codex", "linux", "npm install -g @openai/codex", true},
		{"cursor", "darwin", "", false}, // hint is a bare URL — not auto-installable
		{"nonesuch", "linux", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.id+"/"+tt.goos, func(t *testing.T) {
			got, ok := installCommand(tt.id, tt.goos)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tt.wantOK, got)
			}
			if got != tt.want {
				t.Errorf("cmd = %q, want %q", got, tt.want)
			}
		})
	}
}

// withStubRunner swaps installRunner for a command that ignores the resolved
// line and runs `sh -c stub` instead, restoring the original on cleanup.
func withStubRunner(t *testing.T, stub string) {
	t.Helper()
	orig := installRunner
	installRunner = func(ctx context.Context, _ string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", stub)
	}
	t.Cleanup(func() { installRunner = orig })
}

func postInstall(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	NewDepsInstallHandler().Register(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/deps/install", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestDepsInstallStream(t *testing.T) {
	withStubRunner(t, "printf 'line one\\nline two\\n'")

	rec := postInstall(t, `{"id":"git"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var logs []string
	var doneCode = -999
	var sawStart bool
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
			cmd, _ := ev["command"].(string)
			if !strings.Contains(cmd, "git") {
				t.Errorf("start command = %q, want the resolved git install line", cmd)
			}
		case "log":
			if line, ok := ev["line"].(string); ok {
				logs = append(logs, line)
			}
		case "done":
			if code, ok := ev["code"].(float64); ok {
				doneCode = int(code)
			}
		case "error":
			t.Fatalf("unexpected error event: %v", ev["error"])
		}
	}

	if !sawStart {
		t.Error("missing start event")
	}
	if doneCode != 0 {
		t.Errorf("done code = %d, want 0", doneCode)
	}
	joined := strings.Join(logs, "|")
	if !strings.Contains(joined, "line one") || !strings.Contains(joined, "line two") {
		t.Errorf("logs = %v, want both output lines", logs)
	}
}

func TestDepsInstallNonZeroExit(t *testing.T) {
	withStubRunner(t, "echo oops; exit 3")

	rec := postInstall(t, `{"id":"tmux"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code":3`) {
		t.Errorf("body missing exit code 3: %s", rec.Body.String())
	}
}

func TestDepsInstallUnknownID(t *testing.T) {
	rec := postInstall(t, `{"id":"nonesuch"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an id with no installer", rec.Code)
	}
}

func TestDepsInstallEmptyID(t *testing.T) {
	rec := postInstall(t, `{"id":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for empty id", rec.Code)
	}
}

func TestDepsInstallMethodGuard(t *testing.T) {
	mux := http.NewServeMux()
	NewDepsInstallHandler().Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/deps/install", nil)
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestDepsInstallLoopbackGuard(t *testing.T) {
	mux := http.NewServeMux()
	NewDepsInstallHandler().Register(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/deps/install", strings.NewReader(`{"id":"git"}`))
	req.RemoteAddr = "10.0.0.5:1234"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a non-loopback caller", rec.Code)
	}
}
