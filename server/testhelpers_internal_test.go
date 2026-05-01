package server

import (
	"os/exec"
	"testing"
)

// gitInitDir runs `git init -q` inside dir. workspace.Init / workspace.Load
// require the rootDir to be a git checkout (validated via the .git directory).
func gitInitDir(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init in %s failed: %v\n%s", dir, err, out)
	}
}
