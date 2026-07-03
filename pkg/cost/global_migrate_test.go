package cost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestOpenGlobalStore_CopiesLegacyLedger: when the canonical
// ~/.mycel/costs.db does not exist but the pre-rename ~/.bc/costs.db
// does, opening the global store copies the legacy ledger over once so
// historical spend survives the home-dir rename.
func TestOpenGlobalStore_CopiesLegacyLedger(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("MYCEL_HOME", "")
	t.Setenv("BC_HOME", "")
	ctx := context.Background()

	// Seed a legacy ledger with one record.
	legacyPath := filepath.Join(tmp, ".bc", "costs.db")
	legacy, err := OpenGlobalStore(legacyPath)
	if err != nil {
		t.Fatalf("open legacy ledger: %v", err)
	}
	if _, recErr := legacy.Record(ctx, "zen-zebra", "", "claude-fable-5", 100, 50, 0.42); recErr != nil {
		t.Fatalf("record: %v", recErr)
	}
	if closeErr := legacy.Close(); closeErr != nil {
		t.Fatalf("close legacy: %v", closeErr)
	}

	// Open at the canonical path — the legacy file must be copied in.
	canonicalPath := filepath.Join(tmp, ".mycel", "costs.db")
	s, err := OpenGlobalStore(canonicalPath)
	if err != nil {
		t.Fatalf("open canonical ledger: %v", err)
	}
	defer func() { _ = s.Close() }()

	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cost_records`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("cost_records = %d, want 1 migrated record", n)
	}
	if _, err := os.Stat(canonicalPath); err != nil {
		t.Fatalf("canonical ledger not created: %v", err)
	}
	// Legacy file stays in place (read-only copy, not a destructive move).
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy ledger removed: %v", err)
	}
}

// TestOpenGlobalStore_NoCopyWhenCanonicalExists: an existing canonical
// ledger is never overwritten by the legacy one.
func TestOpenGlobalStore_NoCopyWhenCanonicalExists(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("MYCEL_HOME", "")
	t.Setenv("BC_HOME", "")
	ctx := context.Background()

	// Canonical ledger created first, with two records.
	canonicalPath := filepath.Join(tmp, ".mycel", "costs.db")
	canon, err := OpenGlobalStore(canonicalPath)
	if err != nil {
		t.Fatalf("open canonical: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, recErr := canon.Record(ctx, "new-agent", "", "m", 1, 1, 0.01); recErr != nil {
			t.Fatalf("record canonical: %v", recErr)
		}
	}
	if closeErr := canon.Close(); closeErr != nil {
		t.Fatalf("close canonical: %v", closeErr)
	}

	// A legacy ledger appearing later must not clobber it.
	legacy, err := OpenGlobalStore(filepath.Join(tmp, ".bc", "costs.db"))
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	if _, recErr := legacy.Record(ctx, "old-agent", "", "m", 1, 1, 0.01); recErr != nil {
		t.Fatalf("record legacy: %v", recErr)
	}
	if closeErr := legacy.Close(); closeErr != nil {
		t.Fatalf("close legacy: %v", closeErr)
	}

	// Reopen: the canonical content must be untouched.
	s, err := OpenGlobalStore(canonicalPath)
	if err != nil {
		t.Fatalf("reopen canonical: %v", err)
	}
	defer func() { _ = s.Close() }()
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cost_records`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("cost_records = %d, want 2 (canonical must not be clobbered)", n)
	}
}
