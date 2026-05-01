package server_test

import (
	"os/exec"
	"testing"
)

// gitInitWorkspaceDir returns a t.TempDir() that has been initialized as
// a git repository. workspace.Init / workspace.Load require the rootDir
// to be a git checkout (validated via the .git directory).
func gitInitWorkspaceDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitInitDir(t, dir)
	return dir
}

// gitInitDir runs `git init -q` inside dir.
func gitInitDir(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init in %s failed: %v\n%s", dir, err, out)
	}
}
