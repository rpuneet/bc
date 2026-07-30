package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// gitInitDir adds a minimal .git/ directory to an existing path so it
// satisfies the git-repo check enforced by Open and Load.
func gitInitDir(t testing.TB, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("create .git in %s: %v", dir, err)
	}
}

// setTestHome points MYCEL_HOME at a fresh temp dir for this test so no
// test ever touches the real ~/.mycel, and returns that home path.
// pkg/db caches connections per path, so distinct homes stay isolated.
func setTestHome(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("MYCEL_HOME", home)
	return home
}

// newTestRepo creates a temp dir that passes the git-repo check.
func newTestRepo(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	gitInitDir(t, dir)
	return dir
}

// openTestWorkspace bootstraps a workspace in an isolated MYCEL_HOME
// anchored on a fresh git repo. Returns the workspace and the repo dir.
func openTestWorkspace(t testing.TB) (*Workspace, string) {
	t.Helper()
	setTestHome(t)
	dir := newTestRepo(t)
	ws, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return ws, dir
}
