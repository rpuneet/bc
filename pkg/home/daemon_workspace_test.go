package home

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishDaemonWorkspaceRoundTrips(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	ws := t.TempDir()

	if err := PublishDaemonWorkspace(ws); err != nil {
		t.Fatalf("PublishDaemonWorkspace: %v", err)
	}

	if got := LastDaemonWorkspace(); got != ws {
		t.Errorf("LastDaemonWorkspace() = %q, want %q", got, ws)
	}
}

// A daemon serving no workspace has to be distinguishable from no daemon having
// run: the app must not adopt a workspace nobody chose.
func TestPublishingNoWorkspaceReadsBackAsNone(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())

	if err := PublishDaemonWorkspace(""); err != nil {
		t.Fatalf("PublishDaemonWorkspace: %v", err)
	}

	if got := LastDaemonWorkspace(); got != "" {
		t.Errorf("LastDaemonWorkspace() = %q, want empty", got)
	}
}

func TestLastDaemonWorkspaceWithNoRecord(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())

	if got := LastDaemonWorkspace(); got != "" {
		t.Errorf("LastDaemonWorkspace() = %q, want empty when nothing was published", got)
	}
}

// Adopting a workspace that has since been moved or deleted reproduces the same
// invisible-session mismatch from the other side, so a stale record counts as no
// record.
func TestLastDaemonWorkspaceIgnoresAVanishedDirectory(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	gone := filepath.Join(t.TempDir(), "moved-away")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := PublishDaemonWorkspace(gone); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	if got := LastDaemonWorkspace(); got != "" {
		t.Errorf("LastDaemonWorkspace() = %q, want empty for a directory that is gone", got)
	}
}

// A file where the workspace should be is not a workspace.
func TestLastDaemonWorkspaceIgnoresANonDirectory(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PublishDaemonWorkspace(f); err != nil {
		t.Fatal(err)
	}

	if got := LastDaemonWorkspace(); got != "" {
		t.Errorf("LastDaemonWorkspace() = %q, want empty for a path that is not a directory", got)
	}
}

// Restarting into a different workspace has to replace the record, not append to
// it, or the app adopts whichever one it happens to read first.
func TestPublishDaemonWorkspaceReplacesThePreviousOne(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	first, second := t.TempDir(), t.TempDir()

	if err := PublishDaemonWorkspace(first); err != nil {
		t.Fatal(err)
	}
	if err := PublishDaemonWorkspace(second); err != nil {
		t.Fatal(err)
	}

	if got := LastDaemonWorkspace(); got != second {
		t.Errorf("LastDaemonWorkspace() = %q, want the most recent %q", got, second)
	}
}
