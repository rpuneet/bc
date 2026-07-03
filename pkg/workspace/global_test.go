package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDir(t *testing.T) {
	dir := t.TempDir()
	withMycelHome(t, dir)

	t.Run("valid-id", func(t *testing.T) {
		got, err := DataDir("abcdef123456")
		if err != nil {
			t.Fatalf("DataDir: %v", err)
		}
		want := filepath.Join(dir, "workspaces", "abcdef123456")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("empty-id", func(t *testing.T) {
		if _, err := DataDir(""); err == nil {
			t.Fatal("expected error for empty id, got nil")
		}
	})

	t.Run("matches-ComputeWorkspaceID", func(t *testing.T) {
		absPath := filepath.Join(dir, "some", "project")
		id := ComputeWorkspaceID(absPath)
		got, err := DataDir(id)
		if err != nil {
			t.Fatalf("DataDir: %v", err)
		}
		if !strings.HasSuffix(got, filepath.Join("workspaces", id)) {
			t.Errorf("expected suffix workspaces/%s, got %q", id, got)
		}
	})
}

// withMycelHome sets MYCEL_HOME for the duration of the test and restores it.
func withMycelHome(t *testing.T, dir string) {
	t.Helper()
	prev, had := os.LookupEnv("MYCEL_HOME")
	t.Setenv("MYCEL_HOME", dir)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("MYCEL_HOME", prev)
		} else {
			_ = os.Unsetenv("MYCEL_HOME")
		}
	})
}

func TestGlobalPathsRespectMycelHome(t *testing.T) {
	dir := t.TempDir()
	withMycelHome(t, dir)

	cases := []struct {
		name string
		fn   func() (string, error)
		rel  string
	}{
		{"templates", GlobalTemplatesDir, "templates"},
		{"secrets", GlobalSecretsVault, "secrets.vault"},
		{"mcp", GlobalMCPConfig, "mcps.json"},
		{"costs", GlobalCostsDB, "costs.db"},
		{"tools", GlobalToolsConfig, "tools.json"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.fn()
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			want := filepath.Join(dir, tc.rel)
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestEnsureGlobalDirCreatesWithSafeMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bc-home")
	withMycelHome(t, dir)

	home, err := EnsureGlobalDir()
	if err != nil {
		t.Fatalf("EnsureGlobalDir: %v", err)
	}
	if home != dir {
		t.Fatalf("home %q != requested %q", home, dir)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat home: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("home is not a directory")
	}
	// On Unix we can assert the mode bits. On Windows the mode is not
	// enforced the same way, so guard against it.
	perm := info.Mode().Perm()
	if perm != 0 && perm&0o777 > 0o750 {
		t.Errorf("home perms %o wider than 0750", perm)
	}
}

func TestEnsureGlobalDirIdempotent(t *testing.T) {
	dir := t.TempDir()
	withMycelHome(t, dir)

	if _, err := EnsureGlobalDir(); err != nil {
		t.Fatalf("first EnsureGlobalDir: %v", err)
	}
	if _, err := EnsureGlobalDir(); err != nil {
		t.Fatalf("second EnsureGlobalDir: %v", err)
	}
}

func TestGlobalPathsPlaceUnderMycelHome(t *testing.T) {
	dir := t.TempDir()
	withMycelHome(t, dir)

	for _, fn := range []func() (string, error){
		GlobalTemplatesDir, GlobalSecretsVault, GlobalMCPConfig, GlobalCostsDB, GlobalToolsConfig,
	} {
		p, err := fn()
		if err != nil {
			t.Fatalf("resolve path: %v", err)
		}
		if !strings.HasPrefix(p, dir+string(filepath.Separator)) && p != dir {
			t.Errorf("path %q escapes bc home %q", p, dir)
		}
	}
}
