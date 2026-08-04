package home

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// mkRepo creates dir/.git so ScanLocal treats dir as a candidate repo.
func mkRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
}

func TestScanLocalFindsRepos(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, filepath.Join(root, "a"))
	mkRepo(t, filepath.Join(root, "b"))
	// nested under depth=2 from root
	mkRepo(t, filepath.Join(root, "sub", "c"))
	// excluded
	mkRepo(t, filepath.Join(root, "node_modules", "bad"))

	out, err := ScanLocal(context.Background(), ScanOptions{Root: root, Depth: 3})
	if err != nil {
		t.Fatalf("ScanLocal: %v", err)
	}
	names := map[string]bool{}
	for _, c := range out {
		names[c.Name] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !names[want] {
			t.Errorf("missing candidate %q", want)
		}
	}
	if names["bad"] {
		t.Error("node_modules subrepo should have been skipped")
	}
}

func TestScanLocalHasMycelFlag(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, filepath.Join(root, "has"))
	if err := os.MkdirAll(filepath.Join(root, "has", ".mycel"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	out, err := ScanLocal(context.Background(), ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("ScanLocal: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("candidates = %d, want 1", len(out))
	}
	if !out[0].HasMycel {
		t.Error("HasMycel should be true")
	}
}

func TestScanLocalDepthRespected(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, filepath.Join(root, "l1", "l2", "l3", "deep"))

	// Depth 1 should not see the deep repo.
	out, err := ScanLocal(context.Background(), ScanOptions{Root: root, Depth: 1})
	if err != nil {
		t.Fatalf("ScanLocal: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected 0 candidates at depth 1, got %d", len(out))
	}

	// Depth 5 should see it.
	out, err = ScanLocal(context.Background(), ScanOptions{Root: root, Depth: 5})
	if err != nil {
		t.Fatalf("ScanLocal: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 candidate at depth 5, got %d", len(out))
	}
}

func TestScanLocalBadRoot(t *testing.T) {
	if _, err := ScanLocal(context.Background(), ScanOptions{Root: ""}); err == nil {
		t.Error("empty root should error")
	}
	if _, err := ScanLocal(context.Background(), ScanOptions{Root: "/this/path/does/not/exist/hopefully"}); err == nil {
		t.Error("missing root should error")
	}
}

func TestScanLocalSkipsTCCProtectedHomeFolders(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)

	mkRepo(t, filepath.Join(root, "Music", "album-repo"))
	mkRepo(t, filepath.Join(root, "Pictures", "photo-repo"))
	mkRepo(t, filepath.Join(root, "Downloads", "dl-repo"))
	mkRepo(t, filepath.Join(root, "Projects", "real"))
	// A project literally named Music under Projects must still be found.
	mkRepo(t, filepath.Join(root, "Projects", "Music", "band-repo"))

	out, err := ScanLocal(context.Background(), ScanOptions{Root: root, Depth: 4})
	if err != nil {
		t.Fatalf("ScanLocal: %v", err)
	}
	names := map[string]bool{}
	for _, c := range out {
		names[c.Name] = true
	}
	if !names["real"] {
		t.Error("expected Projects/real")
	}
	if !names["band-repo"] {
		t.Error("expected Projects/Music/band-repo — TCC skip is home-scoped only")
	}
	for _, bad := range []string{"album-repo", "photo-repo", "dl-repo"} {
		if names[bad] {
			t.Errorf("$HOME TCC folder leaked candidate %q", bad)
		}
	}
}

func TestGithubURLFromRemote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"git@github.com:foo/bar.git", "https://github.com/foo/bar"},
		{"git@github.com:foo/bar", "https://github.com/foo/bar"},
		{"https://github.com/foo/bar.git", "https://github.com/foo/bar"},
		{"https://github.com/foo/bar", "https://github.com/foo/bar"},
		{"git://github.com/foo/bar.git", "https://github.com/foo/bar"},
		{"https://gitlab.com/foo/bar.git", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := githubURLFromRemote(c.in)
		if got != c.want {
			t.Errorf("githubURLFromRemote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
