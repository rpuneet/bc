package cost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestOpenGlobalStore_FreshLedger: opening the global store at the
// canonical ~/.mycel/costs.db path creates a fresh, working ledger.
func TestOpenGlobalStore_FreshLedger(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("MYCEL_HOME", "")
	ctx := context.Background()

	canonicalPath := filepath.Join(tmp, ".mycel", "costs.db")
	s, err := OpenGlobalStore(canonicalPath)
	if err != nil {
		t.Fatalf("open canonical ledger: %v", err)
	}
	defer func() { _ = s.Close() }()

	if _, recErr := s.Record(ctx, "zen-zebra", "", "claude-fable-5", 100, 50, 0.42); recErr != nil {
		t.Fatalf("record: %v", recErr)
	}

	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cost_records`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("cost_records = %d, want 1", n)
	}
	if _, err := os.Stat(canonicalPath); err != nil {
		t.Fatalf("canonical ledger not created: %v", err)
	}
}
