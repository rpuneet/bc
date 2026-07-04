package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// chdir switches the working directory for the test and restores it on cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func TestFindGitRoot(t *testing.T) {
	tmpDir := t.TempDir()
	repo := filepath.Join(tmpDir, "repo")
	nested := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	gitInitDir(t, repo)

	// From a nested dir, findGitRoot walks up to the repo root.
	chdir(t, nested)
	got := findGitRoot()
	wantRepo, _ := filepath.EvalSymlinks(repo)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != wantRepo {
		t.Errorf("findGitRoot() = %q, want %q", gotResolved, wantRepo)
	}
}

func TestResolveUpWorkspace_AdoptsGitRoot(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MYCEL_HOME", filepath.Join(tmpDir, "home-mycel"))
	t.Setenv("BC_WORKSPACE", "")

	repo := filepath.Join(tmpDir, "repo")
	nested := filepath.Join(repo, "sub")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	gitInitDir(t, repo)

	// An uninitialized git repo is adopted as the workspace root.
	chdir(t, nested)
	got := resolveUpWorkspace()
	wantRepo, _ := filepath.EvalSymlinks(repo)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != wantRepo {
		t.Errorf("resolveUpWorkspace() = %q, want git root %q", gotResolved, wantRepo)
	}
}

func TestResolveUpWorkspace_NoRepoEmptyRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MYCEL_HOME", filepath.Join(tmpDir, "home-mycel"))
	t.Setenv("BC_WORKSPACE", "")

	// Not a git repo, empty registry → no workspace ("" means the
	// server boots workspace-less and repos are added via the web UI).
	plain := filepath.Join(tmpDir, "plain")
	if err := os.MkdirAll(plain, 0o750); err != nil {
		t.Fatal(err)
	}
	chdir(t, plain)
	if got := resolveUpWorkspace(); got != "" {
		t.Errorf("resolveUpWorkspace() = %q, want empty (workspace-less boot)", got)
	}
}

func TestUpCmd_DefaultAddr(t *testing.T) {
	f := upCmd.Flags().Lookup("addr")
	if f == nil {
		t.Fatal("addr flag not found")
	}
	if f.DefValue != "127.0.0.1:9374" {
		t.Errorf("got %q, want %q", f.DefValue, "127.0.0.1:9374")
	}
}

func TestNormalizeAddr(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{":8080", "127.0.0.1:8080"},
		{":9374", "127.0.0.1:9374"},
		{"0.0.0.0:9374", "0.0.0.0:9374"},
		{"127.0.0.1:9374", "127.0.0.1:9374"},
		{"localhost:9374", "localhost:9374"},
		{"notaport", "notaport"}, // no host:port → returned as-is
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeAddr(tt.input)
			if got != tt.want {
				t.Errorf("normalizeAddr(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
