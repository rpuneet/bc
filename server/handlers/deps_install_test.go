package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/tool"
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

// resolveTestStore returns an open tools store seeded with one registered CLI
// tool that carries both an install and an upgrade command.
func resolveTestStore(t *testing.T) *tool.Store {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "mycel.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	s := tool.NewStore(d, "sqlite")
	if err := s.Open(); err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Add(context.Background(), &tool.Tool{
		Name:       "acmecli",
		Type:       tool.ToolTypeCLI,
		Command:    "acmecli",
		InstallCmd: "brew install acmecli",
		UpgradeCmd: "brew upgrade acmecli",
		Enabled:    true,
	}); err != nil {
		t.Fatalf("seed tool: %v", err)
	}
	return s
}

func TestResolveCommand(t *testing.T) {
	h := NewDepsInstallHandler().SetToolStore(resolveTestStore(t))
	ctx := context.Background()

	tests := []struct {
		name, id, mode, want string
		wantOK               bool
	}{
		// Vetted table still wins for install of a table item.
		{"vetted git install", "git", "install", "", true},
		// Registered CLI tool: install uses install_cmd (not in the table).
		{"tool install_cmd", "acmecli", "install", "brew install acmecli", true},
		// Registered CLI tool: update prefers upgrade_cmd.
		{"tool upgrade_cmd", "acmecli", "update", "brew upgrade acmecli", true},
		// Unknown id with no installer.
		{"unknown", "nonesuch", "install", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := h.resolveCommand(ctx, tt.id, tt.mode)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tt.wantOK, got)
			}
			// For the git case we only assert non-empty (the exact line is
			// OS-dependent and covered by TestInstallCommand).
			if tt.want != "" && got != tt.want {
				t.Errorf("cmd = %q, want %q", got, tt.want)
			}
			if tt.wantOK && got == "" {
				t.Errorf("expected a non-empty command for %q", tt.id)
			}
		})
	}
}

// A nil tool store must degrade to the vetted table only, never panic.
func TestResolveCommandNilStore(t *testing.T) {
	h := NewDepsInstallHandler()
	if _, ok := h.resolveCommand(context.Background(), "gh", "install"); ok {
		t.Error("expected no installer for gh without a tool store")
	}
	if _, ok := h.resolveCommand(context.Background(), "git", "install"); !ok {
		t.Error("expected the vetted git installer to still resolve")
	}
}
