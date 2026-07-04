package server

import (
	"os"
	"testing"
)

// TestMain pins MYCEL_HOME to a throwaway dir so the single global
// database created by workspace Init/Load and BuildWorkspaceServices
// in tests never touches the developer's real ~/.mycel. Tests that
// need a specific home still override it per-test via t.Setenv.
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
