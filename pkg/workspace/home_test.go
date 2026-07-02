package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMycelHome_LegacyFallback locks the read-only fallback path:
// when only ~/.bc/ exists on disk, MycelHome returns that so a fresh
// binary still finds pre-rename state.
func TestMycelHome_LegacyFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", "")
	t.Setenv("BC_HOME", "")

	legacy := filepath.Join(home, ".bc")
	if err := os.MkdirAll(legacy, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := MycelHome()
	if err != nil {
		t.Fatalf("MycelHome: %v", err)
	}
	if got != legacy {
		t.Errorf("got %s, want legacy %s", got, legacy)
	}
}

// TestMycelHome_CanonicalDefault when nothing exists yet, MycelHome
// returns ~/.mycel/ so a fresh install writes to the new tree.
func TestMycelHome_CanonicalDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", "")
	t.Setenv("BC_HOME", "")

	got, err := MycelHome()
	if err != nil {
		t.Fatalf("MycelHome: %v", err)
	}
	want := filepath.Join(home, ".mycel")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestMigrateLegacyHome renames ~/.bc → ~/.mycel exactly once and
// only when appropriate.
func TestMigrateLegacyHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", "")
	t.Setenv("BC_HOME", "")

	legacy := filepath.Join(home, ".bc")
	target := filepath.Join(home, ".mycel")
	if err := os.MkdirAll(legacy, 0o750); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	sentinel := filepath.Join(legacy, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("preserved"), 0o600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	migrated, err := MigrateLegacyHome()
	if err != nil {
		t.Fatalf("MigrateLegacyHome: %v", err)
	}
	if !migrated {
		t.Fatal("expected migrated=true, got false")
	}
	if _, statErr := os.Stat(legacy); statErr == nil {
		t.Errorf("legacy %s should be gone after migration", legacy)
	}
	if _, statErr := os.Stat(target); statErr != nil {
		t.Errorf("target %s not created: %v", target, statErr)
	}
	body, err := os.ReadFile(filepath.Join(target, "sentinel.txt")) //nolint:gosec // test-controlled path
	if err != nil || string(body) != "preserved" {
		t.Errorf("sentinel not preserved: body=%q err=%v", string(body), err)
	}

	// Second call is a no-op.
	migrated2, err := MigrateLegacyHome()
	if err != nil {
		t.Fatalf("second MigrateLegacyHome: %v", err)
	}
	if migrated2 {
		t.Error("second call should return migrated=false")
	}
}

// TestMigrateLegacyHome_NoOpWhenEnvOverride ensures we don't touch
// ~/.bc when the user has explicitly pointed us elsewhere via env.
func TestMigrateLegacyHome_NoOpWhenEnvOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", filepath.Join(home, "custom"))

	legacy := filepath.Join(home, ".bc")
	if err := os.MkdirAll(legacy, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	migrated, err := MigrateLegacyHome()
	if err != nil {
		t.Fatalf("MigrateLegacyHome: %v", err)
	}
	if migrated {
		t.Error("should not migrate when MYCEL_HOME is set")
	}
	if _, statErr := os.Stat(legacy); statErr != nil {
		t.Error("legacy should be untouched when env override is set")
	}
}
