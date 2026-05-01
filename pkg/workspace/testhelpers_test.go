package workspace

import (
	"os/exec"
	"testing"
)

// gitInitTempDir returns a temporary directory that has been initialized
// as a git repository. workspace.Init and workspace.Load require their
// rootDir to be a git checkout (validated via the .git directory), so
// tests that exercise those code paths must run git init first.
//
// The temp dir is cleaned up automatically by t.TempDir().
func gitInitTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitInit(t, dir)
	return dir
}

// gitInit runs `git init -q` inside dir. It uses t.Context() so the
// command is canceled if the test is interrupted.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init in %s failed: %v\n%s", dir, err, out)
	}
}
