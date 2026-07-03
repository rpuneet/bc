package client

import (
	"os"
	"path/filepath"
	"testing"
)

// setTestHome pins HOME to a temp dir and clears every env var that
// could redirect MycelHome or daemon discovery.
func setTestHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("MYCEL_HOME", "")
	os.Unsetenv("BC_DAEMON_ADDR") //nolint:errcheck
	return tmp
}

func mkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestReadDaemonAddrFile_Canonical: the CLI reads the canonical
// ~/.mycel/daemon.addr written by `mycel up`.
func TestReadDaemonAddrFile_Canonical(t *testing.T) {
	tmp := setTestHome(t)
	mkdirAll(t, filepath.Join(tmp, ".mycel"))
	writeFile(t, filepath.Join(tmp, ".mycel", "daemon.addr"), "http://127.0.0.1:8080\n")

	if got := readDaemonAddrFile(); got != "http://127.0.0.1:8080" {
		t.Errorf("readDaemonAddrFile() = %q, want canonical ~/.mycel addr", got)
	}
}

// TestReadDaemonAddrFile_NoFiles returns "" so discovery falls back to
// the default address.
func TestReadDaemonAddrFile_NoFiles(t *testing.T) {
	setTestHome(t)
	if got := readDaemonAddrFile(); got != "" {
		t.Errorf("readDaemonAddrFile() = %q, want empty", got)
	}
}

// TestDefaultSocketPath_CanonicalDefault: the canonical mycel-home
// socket path is always returned.
func TestDefaultSocketPath_CanonicalDefault(t *testing.T) {
	tmp := setTestHome(t)
	mkdirAll(t, filepath.Join(tmp, ".mycel"))

	want := filepath.Join(tmp, ".mycel", "bcd.sock")
	if got := DefaultSocketPath(); got != want {
		t.Errorf("DefaultSocketPath() = %q, want %q", got, want)
	}
}
