package server

import (
	"os"
	"path/filepath"
	"testing"
)

// gitInitDir creates a minimal .git/ directory inside dir so workspace.Open
// and workspace.Load (which require a git repo) succeed in tests.
func gitInitDir(t testing.TB, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("create .git in %s: %v", dir, err)
	}
}
