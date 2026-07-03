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
	t.Setenv("BC_HOME", "")
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

// TestReadDaemonAddrFile_PrefersCanonicalOverLegacy pins the split-brain
// fix: when both ~/.mycel/daemon.addr (written by `mycel up`) and a
// stale ~/.bc/daemon.addr exist, the CLI must read the canonical one.
func TestReadDaemonAddrFile_PrefersCanonicalOverLegacy(t *testing.T) {
	tmp := setTestHome(t)
	mkdirAll(t, filepath.Join(tmp, ".mycel"))
	mkdirAll(t, filepath.Join(tmp, ".bc"))
	writeFile(t, filepath.Join(tmp, ".mycel", "daemon.addr"), "http://127.0.0.1:8080\n")
	writeFile(t, filepath.Join(tmp, ".bc", "daemon.addr"), "http://127.0.0.1:9999\n")

	if got := readDaemonAddrFile(); got != "http://127.0.0.1:8080" {
		t.Errorf("readDaemonAddrFile() = %q, want canonical ~/.mycel addr", got)
	}
}

// TestReadDaemonAddrFile_LegacyFallback: ~/.mycel exists but has no
// daemon.addr — a pre-rename ~/.bc/daemon.addr must still be honored.
func TestReadDaemonAddrFile_LegacyFallback(t *testing.T) {
	tmp := setTestHome(t)
	mkdirAll(t, filepath.Join(tmp, ".mycel"))
	mkdirAll(t, filepath.Join(tmp, ".bc"))
	writeFile(t, filepath.Join(tmp, ".bc", "daemon.addr"), "http://127.0.0.1:9999\n")

	if got := readDaemonAddrFile(); got != "http://127.0.0.1:9999" {
		t.Errorf("readDaemonAddrFile() = %q, want legacy ~/.bc addr", got)
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

// TestDefaultSocketPath_LegacyFallback: when only the legacy
// ~/.bc/bcd.sock exists on disk it is returned; otherwise the canonical
// <mycel home>/bcd.sock path is used.
func TestDefaultSocketPath_LegacyFallback(t *testing.T) {
	tmp := setTestHome(t)
	mkdirAll(t, filepath.Join(tmp, ".mycel"))
	mkdirAll(t, filepath.Join(tmp, ".bc"))

	legacySock := filepath.Join(tmp, ".bc", "bcd.sock")
	writeFile(t, legacySock, "")
	if got := DefaultSocketPath(); got != legacySock {
		t.Errorf("DefaultSocketPath() = %q, want legacy %q", got, legacySock)
	}

	// Once the canonical socket exists it wins.
	canonSock := filepath.Join(tmp, ".mycel", "bcd.sock")
	writeFile(t, canonSock, "")
	if got := DefaultSocketPath(); got != canonSock {
		t.Errorf("DefaultSocketPath() = %q, want canonical %q", got, canonSock)
	}
}

// TestDefaultSocketPath_CanonicalDefault: with no sockets on disk the
// canonical mycel-home path is returned.
func TestDefaultSocketPath_CanonicalDefault(t *testing.T) {
	tmp := setTestHome(t)
	mkdirAll(t, filepath.Join(tmp, ".mycel"))

	want := filepath.Join(tmp, ".mycel", "bcd.sock")
	if got := DefaultSocketPath(); got != want {
		t.Errorf("DefaultSocketPath() = %q, want %q", got, want)
	}
}
