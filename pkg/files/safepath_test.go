package files

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSafeJoin_Success(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hi"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub", "deep"), 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"file at root", "hello.txt", filepath.Join(root, "hello.txt")},
		{"empty", "", root},
		{"dot", ".", root},
		{"subdir", "sub", filepath.Join(root, "sub")},
		{"nested", "sub/deep", filepath.Join(root, "sub", "deep")},
		{"trailing slash", "sub/", filepath.Join(root, "sub")},
		{"clean redundant", "sub/./deep", filepath.Join(root, "sub", "deep")},
		{"non-existent but inside", "new/file.txt", filepath.Join(root, "new", "file.txt")},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeJoin(root, tt.in)
			if err != nil {
				t.Fatalf("SafeJoin(%q, %q) error: %v", root, tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("SafeJoin(%q, %q) = %q, want %q", root, tt.in, got, tt.want)
			}
		})
	}
}

func TestSafeJoin_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	// Create a sibling dir that lives next to root — a successful escape
	// from root would let the caller read files out of sibling.
	parent := filepath.Dir(root)
	sibling := filepath.Join(parent, "sibling-secret")
	if err := os.MkdirAll(sibling, 0750); err != nil {
		t.Fatalf("mkdir sibling: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sibling) }) //nolint:errcheck

	cases := []struct {
		name string
		in   string
	}{
		{"single dotdot", ".."},
		{"dotdot file", "../secret.txt"},
		{"deep dotdot", "a/b/../../../etc/passwd"},
		{"sibling escape", "../" + filepath.Base(sibling)},
		{"leading dotdot with slash", "../../foo"},
		{"mixed separators", "sub/../../escape"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeJoin(root, tt.in)
			if err == nil {
				t.Fatalf("SafeJoin(%q, %q) = %q, want error", root, tt.in, got)
			}
			if !errors.Is(err, ErrPathEscape) {
				t.Fatalf("got error %v, want ErrPathEscape", err)
			}
		})
	}
}

func TestSafeJoin_RejectsAbsolute(t *testing.T) {
	root := t.TempDir()

	abs := "/etc/passwd"
	if runtime.GOOS == "windows" {
		abs = `C:\Windows\System32`
	}

	if _, err := SafeJoin(root, abs); err == nil {
		t.Fatalf("SafeJoin(%q, %q) succeeded, want error", root, abs)
	} else if !errors.Is(err, ErrPathEscape) {
		t.Fatalf("got error %v, want ErrPathEscape", err)
	}
}

func TestSafeJoin_RejectsNUL(t *testing.T) {
	root := t.TempDir()
	_, err := SafeJoin(root, "foo\x00bar")
	if err == nil {
		t.Fatal("SafeJoin with NUL should error")
	}
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("got error %v, want ErrInvalidPath", err)
	}
}

func TestSafeJoin_EmptyRoot(t *testing.T) {
	if _, err := SafeJoin("", "foo"); err == nil {
		t.Fatal("empty root should error")
	}
}

func TestSafeJoin_RejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("oops"), 0600); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	// A symlink INSIDE root that points to outside root.
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// Accessing through the symlink should be rejected.
	if got, err := SafeJoin(root, "escape/secret.txt"); err == nil {
		t.Fatalf("SafeJoin through symlink succeeded with %q, want error", got)
	} else if !errors.Is(err, ErrPathEscape) {
		t.Fatalf("got error %v, want ErrPathEscape", err)
	}

	// The link itself is also outside once resolved.
	if _, err := SafeJoin(root, "escape"); err == nil {
		t.Fatal("SafeJoin of escaping symlink succeeded, want error")
	}
}

func TestSafeJoin_AllowsInternalSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin on Windows")
	}
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(real, "ok.txt"), []byte("k"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(root, "alias")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got, err := SafeJoin(root, "alias/ok.txt")
	if err != nil {
		t.Fatalf("internal symlink rejected: %v", err)
	}
	// The returned path is the lexical join, not the resolved target —
	// callers can os.ReadFile it either way.
	want := filepath.Join(root, "alias", "ok.txt")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSafeJoin_ResolvedRootWithSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin on Windows")
	}
	// On macOS, t.TempDir() often lives under /var which is itself a
	// symlink to /private/var. SafeJoin must compare resolved forms so
	// normal access still works on such systems.
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	if err := os.WriteFile(file, []byte("hi"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := SafeJoin(root, "a.txt")
	if err != nil {
		t.Fatalf("SafeJoin: %v", err)
	}
	if !strings.HasSuffix(got, "a.txt") {
		t.Fatalf("unexpected result %q", got)
	}
}
