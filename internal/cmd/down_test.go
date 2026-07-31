package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/home"
)

// The daemon is user-scoped: `mycel down` no longer requires a repo (or
// any particular CWD) and lost its --workspace flag.

func TestDownHasNoWorkspaceFlag(t *testing.T) {
	if f := downCmd.Flags().Lookup("workspace"); f != nil {
		t.Error("down should not have a --workspace flag (daemon is user-scoped)")
	}
}

func TestDownIsCWDFree(t *testing.T) {
	// Isolated ~/.mycel with a stale pid file; run from a plain non-git
	// temp dir. down must succeed, report the daemon as not running, and
	// clean up the pid file — no repo required.
	t.Setenv("MYCEL_HOME", t.TempDir())

	runDir, err := home.EnsureRunDir()
	if err != nil {
		t.Fatalf("ensure run dir: %v", err)
	}
	pidPath := filepath.Join(runDir, "daemon.pid")
	// A non-numeric pid makes `kill` fail safely: the "not running"
	// path runs without ever signaling a real process.
	if writeErr := os.WriteFile(pidPath, []byte("stale-not-a-pid\n"), 0o600); writeErr != nil {
		t.Fatalf("write pid file: %v", writeErr)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	tmpDir := t.TempDir() // plain dir, not a git repo
	if chdirErr := os.Chdir(tmpDir); chdirErr != nil {
		t.Fatalf("failed to chdir: %v", chdirErr)
	}
	defer func() { _ = os.Chdir(origDir) }()

	stdout, _, err := executeIntegrationCmd("down")
	if err != nil {
		t.Fatalf("down should be CWD-free and not require a repo, got: %v", err)
	}
	if !strings.Contains(stdout, "Stopping mycel") {
		t.Errorf("expected 'Stopping mycel' banner, got: %s", stdout)
	}
	if !strings.Contains(stdout, "not running") {
		t.Errorf("expected stale daemon reported as not running, got: %s", stdout)
	}
	if _, statErr := os.Stat(pidPath); !os.IsNotExist(statErr) {
		t.Error("stale daemon.pid should be removed by down")
	}
}
