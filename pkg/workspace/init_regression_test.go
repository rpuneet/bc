package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFind_ResolvesViaStateDirProbe locks the registry-free resolution
// path: after Init, the ONLY marker for a workspace is its global state
// dir at <MycelHome>/workspaces/<ComputeWorkspaceID(repo)>/ — there is
// no workspaces.json and no .bc/ marker in the project dir. Find must
// resolve the repo (and any subdirectory of it) purely by hashing the
// walked path and probing for preferences.json.
func TestFind_ResolvesViaStateDirProbe(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", filepath.Join(home, ".mycel"))

	proj := t.TempDir()
	gitInitDir(t, proj)

	if _, err := Init(proj); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// No registry file may exist — the registry is gone.
	if _, err := os.Stat(filepath.Join(home, ".mycel", "workspaces.json")); !os.IsNotExist(err) {
		t.Fatalf("workspaces.json should not exist, stat err = %v", err)
	}
	// No .bc/ marker in the project dir — the repo stays pristine.
	if _, err := os.Stat(filepath.Join(proj, ".bc")); !os.IsNotExist(err) {
		t.Fatalf(".bc marker should not exist in project dir, stat err = %v", err)
	}

	ws, err := Find(proj)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if ws.RootDir != proj {
		t.Errorf("Find returned wrong workspace: got %q want %q", ws.RootDir, proj)
	}

	// Subdirectories resolve to the same workspace via the walk.
	sub := filepath.Join(proj, "a", "b")
	if mkErr := os.MkdirAll(sub, 0o750); mkErr != nil {
		t.Fatalf("mkdir: %v", mkErr)
	}
	ws, err = Find(sub)
	if err != nil {
		t.Fatalf("Find from subdir: %v", err)
	}
	if ws.RootDir != proj {
		t.Errorf("Find from subdir: got %q want %q", ws.RootDir, proj)
	}
}
