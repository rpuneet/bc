package home

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFind_ResolvesViaGitRootWalk locks the registry-free resolution
// path: after Open, the ONLY global state is the flat ~/.mycel tree
// (prefs.json + mycel.db) — there is no workspaces/ tree, no
// workspaces.json, and no .bc/ marker in the project dir. Find must
// resolve the repo (and any subdirectory of it) purely by walking up to
// the nearest git root and loading the global state.
func TestFind_ResolvesViaGitRootWalk(t *testing.T) {
	home := setTestHome(t)
	proj := newTestRepo(t)

	if _, err := Open(proj); err != nil {
		t.Fatalf("Open: %v", err)
	}

	// No registry file may exist — the registry is gone.
	if _, err := os.Stat(filepath.Join(home, "workspaces.json")); !os.IsNotExist(err) {
		t.Fatalf("workspaces.json should not exist, stat err = %v", err)
	}
	// No per-workspace state tree — state is flat under ~/.mycel.
	if _, err := os.Stat(filepath.Join(home, "workspaces")); !os.IsNotExist(err) {
		t.Fatalf("workspaces/ tree should not exist, stat err = %v", err)
	}
	// No .bc/ marker in the project dir — the repo stays pristine.
	if _, err := os.Stat(filepath.Join(proj, ".bc")); !os.IsNotExist(err) {
		t.Fatalf(".bc marker should not exist in project dir, stat err = %v", err)
	}

	h, err := Find(proj)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if h.RootDir != proj {
		t.Errorf("Find returned wrong root: got %q want %q", h.RootDir, proj)
	}

	// Subdirectories resolve to the same anchor repo via the walk.
	sub := filepath.Join(proj, "a", "b")
	if mkErr := os.MkdirAll(sub, 0o750); mkErr != nil {
		t.Fatalf("mkdir: %v", mkErr)
	}
	h, err = Find(sub)
	if err != nil {
		t.Fatalf("Find from subdir: %v", err)
	}
	if h.RootDir != proj {
		t.Errorf("Find from subdir: got %q want %q", h.RootDir, proj)
	}
}
