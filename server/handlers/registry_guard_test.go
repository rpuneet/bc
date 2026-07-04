package handlers_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain snapshots the user's global ~/.bc/workspaces.json before the
// test binary runs and fails the whole suite if any test caused new
// entries to be appended. This is a safety net behind setupWorkspace's
// MYCEL_HOME sandboxing — if a future test bypasses the helper and calls
// workspace.Init() directly without a MYCEL_HOME override, the snapshot
// hash will change and we catch it here instead of silently polluting
// the real registry.
func TestMain(m *testing.M) {
	before, beforePath := snapshotUserRegistry()

	// Default sandbox: pin MYCEL_HOME (and therefore the single global
	// mycel.db) to a throwaway dir; individual tests may override it.
	var testHome string
	if home, err := os.MkdirTemp("", "mycel-test-home-*"); err == nil {
		testHome = home
		_ = os.Setenv("MYCEL_HOME", home)
	}

	code := m.Run()
	if testHome != "" {
		_ = os.RemoveAll(testHome)
	}

	if beforePath != "" {
		after, _ := snapshotUserRegistry()
		if before != after {
			fmt.Fprintf(os.Stderr,
				"REGISTRY LEAK: user's %s was mutated by tests "+
					"(hash changed). A test probably called "+
					"workspace.Init() without sandboxing MYCEL_HOME.\n",
				beforePath)
			if code == 0 {
				code = 1
			}
		}
	}

	os.Exit(code)
}

// snapshotUserRegistry reads the user's *real* ~/.bc/workspaces.json
// (intentionally NOT via workspace.BCHome(), which respects MYCEL_HOME
// overrides that individual tests set) and returns its SHA-256. Returns
// ("", "") when the file is absent — in which case we skip the check.
func snapshotUserRegistry() (string, string) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", ""
	}
	path := filepath.Join(home, ".bc", "workspaces.json")
	// #nosec G304 -- reading user's own config by a fixed relative path
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), path
}
