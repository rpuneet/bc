package workspace

import (
	"path/filepath"
	"testing"
)

// TestMycelHome_CanonicalDefault: MycelHome always returns ~/.mycel
// when MYCEL_HOME is unset — no legacy fallbacks.
func TestMycelHome_CanonicalDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", "")

	got, err := MycelHome()
	if err != nil {
		t.Fatalf("MycelHome: %v", err)
	}
	want := filepath.Join(home, ".mycel")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestMycelHome_EnvOverride: MYCEL_HOME wins when set.
func TestMycelHome_EnvOverride(t *testing.T) {
	home := t.TempDir()
	custom := filepath.Join(home, "custom")
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", custom)

	got, err := MycelHome()
	if err != nil {
		t.Fatalf("MycelHome: %v", err)
	}
	if got != custom {
		t.Errorf("got %s, want %s", got, custom)
	}
}
