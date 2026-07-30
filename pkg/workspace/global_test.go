package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobalPathsRespectMycelHome(t *testing.T) {
	dir := setTestHome(t)

	cases := []struct {
		fn   func() (string, error)
		name string
		rel  string
	}{
		{GlobalTemplatesDir, "templates", "templates"},
		{GlobalSecretsVault, "secrets", "secrets.vault"},
		{GlobalMCPConfig, "mcp", "mcps.json"},
		{GlobalToolsConfig, "tools", "tools.json"},
		{AgentsDir, "agents", "agents"},
		{AppsDir, "apps", "apps"},
		{GlobalLogsDir, "logs", "logs"},
		{RunDir, "run", "run"},
		{PrefsPath, "prefs", "prefs.json"},
		{DaemonPidPath, "daemon-pid", filepath.Join("run", "daemon.pid")},
		{DaemonAddrPath, "daemon-addr", filepath.Join("run", "daemon.addr")},
		{DaemonLogPath, "daemon-log", filepath.Join("logs", "daemon.log")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.fn()
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			want := filepath.Join(dir, tc.rel)
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestAgentDirLayout(t *testing.T) {
	dir := setTestHome(t)

	agentDir, err := AgentDir("eng-01")
	if err != nil {
		t.Fatalf("AgentDir: %v", err)
	}
	if want := filepath.Join(dir, "agents", "eng-01"); agentDir != want {
		t.Errorf("AgentDir = %q, want %q", agentDir, want)
	}

	subs := []struct {
		fn  func(string) (string, error)
		sub string
	}{
		{AgentWorktreeDir, "worktree"},
		{AgentSessionDir, "session"},
		{AgentLogsDir, "logs"},
		{AgentTmpDir, "tmp"},
	}
	for _, tc := range subs {
		got, err := tc.fn("eng-01")
		if err != nil {
			t.Fatalf("%s: %v", tc.sub, err)
		}
		if want := filepath.Join(agentDir, tc.sub); got != want {
			t.Errorf("%s dir = %q, want %q", tc.sub, got, want)
		}
	}
}

// TestAgentDirRejectsUnsafeNames: name-keyed entity dirs must not allow
// traversal out of ~/.mycel/agents/.
func TestAgentDirRejectsUnsafeNames(t *testing.T) {
	setTestHome(t)

	for _, name := range []string{"", "../escape", "/abs", "a/../../b"} {
		if _, err := AgentDir(name); err == nil {
			t.Errorf("AgentDir(%q) accepted an unsafe name", name)
		}
		if _, err := AgentWorktreeDir(name); err == nil {
			t.Errorf("AgentWorktreeDir(%q) accepted an unsafe name", name)
		}
	}
}

func TestEnsureMycelHomeCreatesStructure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mycel-home")
	t.Setenv("MYCEL_HOME", dir)

	if err := EnsureMycelHome(); err != nil {
		t.Fatalf("EnsureMycelHome: %v", err)
	}

	for _, sub := range []string{"", "agents", "apps", "templates", "logs", "run"} {
		p := filepath.Join(dir, sub)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("%q not created: %v", p, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q is not a directory", p)
		}
	}

	// Idempotent
	if err := EnsureMycelHome(); err != nil {
		t.Fatalf("second EnsureMycelHome: %v", err)
	}
}

func TestEnsureRunAndLogsDirs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mycel-home")
	t.Setenv("MYCEL_HOME", dir)

	run, err := EnsureRunDir()
	if err != nil {
		t.Fatalf("EnsureRunDir: %v", err)
	}
	if want := filepath.Join(dir, "run"); run != want {
		t.Errorf("EnsureRunDir = %q, want %q", run, want)
	}
	if info, statErr := os.Stat(run); statErr != nil || !info.IsDir() {
		t.Errorf("run dir not created: %v", statErr)
	}

	logs, err := EnsureGlobalLogsDir()
	if err != nil {
		t.Fatalf("EnsureGlobalLogsDir: %v", err)
	}
	if want := filepath.Join(dir, "logs"); logs != want {
		t.Errorf("EnsureGlobalLogsDir = %q, want %q", logs, want)
	}
	if info, statErr := os.Stat(logs); statErr != nil || !info.IsDir() {
		t.Errorf("logs dir not created: %v", statErr)
	}
}

func TestEnsureGlobalDirCreatesWithSafeMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bc-home")
	t.Setenv("MYCEL_HOME", dir)

	home, err := EnsureGlobalDir()
	if err != nil {
		t.Fatalf("EnsureGlobalDir: %v", err)
	}
	if home != dir {
		t.Fatalf("home %q != requested %q", home, dir)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat home: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("home is not a directory")
	}
	// On Unix we can assert the mode bits. On Windows the mode is not
	// enforced the same way, so guard against it.
	perm := info.Mode().Perm()
	if perm != 0 && perm&0o777 > 0o750 {
		t.Errorf("home perms %o wider than 0750", perm)
	}
}

func TestEnsureGlobalDirIdempotent(t *testing.T) {
	setTestHome(t)

	if _, err := EnsureGlobalDir(); err != nil {
		t.Fatalf("first EnsureGlobalDir: %v", err)
	}
	if _, err := EnsureGlobalDir(); err != nil {
		t.Fatalf("second EnsureGlobalDir: %v", err)
	}
}

func TestGlobalPathsPlaceUnderMycelHome(t *testing.T) {
	dir := setTestHome(t)

	for _, fn := range []func() (string, error){
		GlobalTemplatesDir, GlobalSecretsVault, GlobalMCPConfig, GlobalToolsConfig,
		AgentsDir, AppsDir, GlobalLogsDir, RunDir, PrefsPath,
		DaemonPidPath, DaemonAddrPath, DaemonLogPath,
	} {
		p, err := fn()
		if err != nil {
			t.Fatalf("resolve path: %v", err)
		}
		if !strings.HasPrefix(p, dir+string(filepath.Separator)) && p != dir {
			t.Errorf("path %q escapes bc home %q", p, dir)
		}
	}
}
