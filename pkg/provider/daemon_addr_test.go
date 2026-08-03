package provider

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// resolveAddr runs DaemonAddrShell the way a reporter does — under /bin/sh, with
// the environment an agent session has — and returns what it resolved to. The
// expression is only ever evaluated by a shell, so testing the Go string would
// test nothing that matters: quoting, the pipeline, and the fallback order are
// all shell behavior.
func resolveAddr(t *testing.T, mycelHome, envAddr string) string {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", `addr=`+DaemonAddrShell+`; printf '%s' "$addr"`)
	cmd.Env = []string{
		"HOME=" + t.TempDir(), // never the real home, whatever the case under test
		"PATH=" + os.Getenv("PATH"),
	}
	if mycelHome != "" {
		cmd.Env = append(cmd.Env, "MYCEL_HOME="+mycelHome)
	}
	if envAddr != "" {
		cmd.Env = append(cmd.Env, "MYCEL_DAEMON_ADDR="+envAddr)
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("resolving the daemon address failed: %v (output %q)", err, out)
	}
	return string(out)
}

// homeWithAddrFile builds a mycel home whose run/daemon.addr holds contents,
// which is what a running daemon leaves behind.
func homeWithAddrFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	runDir := filepath.Join(dir, "run")
	if err := os.MkdirAll(runDir, 0o750); err != nil {
		t.Fatalf("create run dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "daemon.addr"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write daemon.addr: %v", err)
	}
	return dir
}

// TestDaemonAddrShellPrefersPublishedAddress is the regression test for #3510:
// an agent created while the daemon listened on one port carries that port in
// its session environment forever, so a reporter that trusts the environment
// POSTs to a dead port for the rest of the agent's life. The address the running
// daemon published has to win.
func TestDaemonAddrShellPrefersPublishedAddress(t *testing.T) {
	home := homeWithAddrFile(t, "http://127.0.0.1:9374\n")

	got := resolveAddr(t, home, "http://0.0.0.0:8080")

	if got != "http://127.0.0.1:9374" {
		t.Errorf("resolved %q, want the published address http://127.0.0.1:9374 — a stale session env must not win", got)
	}
}

func TestDaemonAddrShellFallsBack(t *testing.T) {
	tests := []struct {
		name    string
		home    func(t *testing.T) string
		envAddr string
		want    string
		wantWhy string
	}{
		{
			name:    "no published address falls back to the environment",
			home:    func(t *testing.T) string { return t.TempDir() },
			envAddr: "http://10.0.0.5:9374",
			want:    "http://10.0.0.5:9374",
			wantWhy: "an agent that does not share the daemon's home — a container, another host — has only the exported address",
		},
		{
			name:    "nothing at all falls back to the default",
			home:    func(t *testing.T) string { return t.TempDir() },
			envAddr: "",
			want:    "http://127.0.0.1:9374",
			wantWhy: "the default port is the last resort",
		},
		{
			name:    "an empty published address is not an address",
			home:    func(t *testing.T) string { return homeWithAddrFile(t, "") },
			envAddr: "http://10.0.0.5:9374",
			want:    "http://10.0.0.5:9374",
			wantWhy: "a truncated or half-written addr file must not resolve to the empty string",
		},
		{
			name:    "a blank line is not an address either",
			home:    func(t *testing.T) string { return homeWithAddrFile(t, "\n") },
			envAddr: "http://10.0.0.5:9374",
			want:    "http://10.0.0.5:9374",
			wantWhy: "grep . rejects a blank first line",
		},
		{
			name:    "only the first line is read",
			home:    func(t *testing.T) string { return homeWithAddrFile(t, "http://127.0.0.1:9374\ntrailing junk\n") },
			envAddr: "http://0.0.0.0:8080",
			want:    "http://127.0.0.1:9374",
			wantWhy: "extra lines must not be appended to the address",
		},
		{
			name:    "a published address with no trailing newline still reads",
			home:    func(t *testing.T) string { return homeWithAddrFile(t, "http://127.0.0.1:9999") },
			envAddr: "",
			want:    "http://127.0.0.1:9999",
			wantWhy: "the daemon is not required to newline-terminate what it publishes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveAddr(t, tt.home(t), tt.envAddr); got != tt.want {
				t.Errorf("resolved %q, want %q — %s", got, tt.want, tt.wantWhy)
			}
		})
	}
}

// TestDaemonAddrShellSurvivesClaudeQuoting guards the constraint that is easy to
// break and impossible to notice: claude's reporter embeds this expression inside
// a single-quoted bash -c command, so a single quote here would end that command
// rather than substitute into it.
func TestDaemonAddrShellSurvivesClaudeQuoting(t *testing.T) {
	if strings.Contains(DaemonAddrShell, "'") {
		t.Fatalf("DaemonAddrShell contains a single quote, which breaks claude's single-quoted bash -c command: %s", DaemonAddrShell)
	}

	home := homeWithAddrFile(t, "http://127.0.0.1:9374\n")
	// Exactly how claude_hooks.go nests it: single-quoted outer command.
	script := `bash -c 'addr=` + DaemonAddrShell + `; printf "%s" "$addr"'`
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH"), "MYCEL_HOME=" + home}

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("nested in a single-quoted bash -c, resolution failed: %v (output %q)", err, out)
	}
	if string(out) != "http://127.0.0.1:9374" {
		t.Errorf("nested resolution gave %q, want http://127.0.0.1:9374", out)
	}
}

// TestReportersResolveTheAddressAtCallTime pins that no provider bakes a literal
// address into the hook config it writes. A baked address is correct only until
// the daemon restarts on another port, and nothing reports the breakage: the hook
// fires, curl fails, and the agent merely looks quiet.
func TestReportersResolveTheAddressAtCallTime(t *testing.T) {
	for _, tt := range []struct {
		provider string
		config   string
	}{
		{"cursor", cursorReporterScript},
		{"agy", agyHookCommand("UserPromptSubmit", "working", "{}")},
	} {
		t.Run(tt.provider, func(t *testing.T) {
			if !strings.Contains(tt.config, "run/daemon.addr") {
				t.Errorf("%s's reporter never consults the published address, so it cannot survive a daemon restart:\n%s", tt.provider, tt.config)
			}
		})
	}
}
