package handlers_test

import (
	"os"
	"testing"
)

// TestMain pins MYCEL_HOME to a throwaway dir by default so the global
// state (prefs.json, the single mycel.db, agents/) created by
// workspace.Open and the stores under test never touches the
// developer's real ~/.mycel. Individual tests override it per-test via
// t.Setenv when they need their own sandbox.
func TestMain(m *testing.M) {
	var testHome string
	if home, err := os.MkdirTemp("", "mycel-test-home-*"); err == nil {
		testHome = home
		_ = os.Setenv("MYCEL_HOME", home)
	}

	code := m.Run()
	if testHome != "" {
		_ = os.RemoveAll(testHome)
	}
	os.Exit(code)
}
