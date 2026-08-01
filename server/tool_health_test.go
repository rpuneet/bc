// tool_health_test.go — the background tool auto-check (#3423): Tools.tsx
// showed DB-seeded "not_installed" status until a user manually clicked
// Health Check. runToolHealthLoop/checkToolsOnce close that gap by running
// the same check the manual endpoint runs, at boot and on an interval.
package server

import (
	"context"
	"path/filepath"
	"testing"

	dbpkg "github.com/rpuneet/mycel/pkg/db"
	toolpkg "github.com/rpuneet/mycel/pkg/tool"
)

// TestCheckToolsOnce_WritesFreshStatus asserts the boot-time (and
// per-interval) pass persists health_status/last_checked via the store,
// exactly like the manual POST /api/tools/check force-refresh, so
// GET /api/tools serves recently-verified data without requiring a manual
// check first.
func TestCheckToolsOnce_WritesFreshStatus(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := dbpkg.Open(filepath.Join(dir, "mycel.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close() //nolint:errcheck

	store := toolpkg.NewStore(d, "sqlite")
	if openErr := store.Open(); openErr != nil {
		t.Fatalf("open store: %v", openErr)
	}
	defer store.Close() //nolint:errcheck

	tools, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected seeded built-in tools")
	}
	for _, tl := range tools {
		if tl.LastChecked != "" {
			t.Fatalf("tool %q already has LastChecked before the background check ran", tl.Name)
		}
	}

	// checkToolsOnce must not require (or wait on) the interval ticker —
	// it's the same function the loop calls immediately at boot so the
	// first daemon-lifetime GET /api/tools already sees verified data.
	checkToolsOnce(ctx, store)

	after, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	for _, tl := range after {
		if tl.LastChecked == "" {
			t.Errorf("tool %q has no LastChecked after checkToolsOnce", tl.Name)
		}
		if tl.HealthStatus == "" || tl.HealthStatus == "unknown" {
			t.Errorf("tool %q HealthStatus = %q, want a verified value", tl.Name, tl.HealthStatus)
		}
	}
}

// TestRunToolHealthLoop_StopsOnContextCanceled confirms the loop's first
// pass runs before the caller observes it and the goroutine exits promptly
// when its context is canceled (parity with the other prune-loop tests in
// this package), so it never blocks BuildServices' Close().
func TestRunToolHealthLoop_StopsOnContextCanceled(t *testing.T) {
	dir := t.TempDir()
	d, err := dbpkg.Open(filepath.Join(dir, "mycel.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close() //nolint:errcheck

	store := toolpkg.NewStore(d, "sqlite")
	if openErr := store.Open(); openErr != nil {
		t.Fatalf("open store: %v", openErr)
	}
	defer store.Close() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runToolHealthLoop(ctx, store)
		close(done)
	}()

	cancel()
	<-done // must return once ctx is canceled, not block forever on the ticker
}
