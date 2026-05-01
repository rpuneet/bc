package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// gitInitDir adds a minimal .git/ directory to an existing path so it
// satisfies the git-repo check enforced by Init and Load.
func gitInitDir(t testing.TB, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("create .git in %s: %v", dir, err)
	}
}
