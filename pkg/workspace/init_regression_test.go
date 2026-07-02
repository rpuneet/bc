package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInit_FailsWhenRegistrySaveFails locks the fix for #3173.
// Historically Init swallowed the registry Save error and returned a
// "success" workspace that every subsequent command rejected as "not
// in a bc workspace" because the registry file was never written.
// Now Init must surface the failure loudly.
func TestInit_FailsWhenRegistrySaveFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BC_HOME", filepath.Join(home, ".bc"))

	// Pre-create the registry file's parent directory as a read-only
	// path so the rename step in Registry.Save fails.
	if err := os.MkdirAll(filepath.Join(home, ".bc"), 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Undo the restriction on cleanup so t.TempDir can clean.
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(home, ".bc"), 0o700) }) //nolint:gosec // cleanup only

	proj := t.TempDir()
	gitInitDir(t, proj)

	_, err := Init(proj)
	if err == nil {
		t.Fatal("Init: expected error when registry cannot persist, got nil")
	}
}

// TestFind_SelfHealsAfterRegistryLoss reproduces the #3173 scenario:
// a workspace exists on disk (state dir + preferences.json) but the
// registry file was deleted / never wrote. Find must probe the
// deterministic workspace ID for the walked directory, discover the
// state dir, register on the fly, and return the workspace.
func TestFind_SelfHealsAfterRegistryLoss(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BC_HOME", filepath.Join(home, ".bc"))

	proj := t.TempDir()
	gitInitDir(t, proj)

	if _, err := Init(proj); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Simulate a lost registry — user wiped ~/.bc/workspaces.json,
	// permission race on install, whatever.
	if err := os.Remove(RegistryPath()); err != nil {
		t.Fatalf("remove registry: %v", err)
	}

	ws, err := Find(proj)
	if err != nil {
		t.Fatalf("Find after registry loss: %v", err)
	}
	if ws.RootDir != proj {
		t.Errorf("Find returned wrong workspace: got %q want %q", ws.RootDir, proj)
	}

	// The self-heal should have re-written the registry so the next
	// call doesn't need to re-probe.
	if _, stErr := os.Stat(RegistryPath()); stErr != nil {
		t.Errorf("self-heal did not persist registry: %v", stErr)
	}
	reg, regErr := LoadRegistry()
	if regErr != nil {
		t.Fatalf("LoadRegistry after self-heal: %v", regErr)
	}
	if reg.Find(proj) == nil {
		t.Errorf("registry entry not re-created for %s", proj)
	}
}
