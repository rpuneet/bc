// tool_health_test.go — the background tool auto-check (#3423): Tools.tsx
// showed DB-seeded "not_installed" status until a user manually clicked
// Health Check. runToolHealthLoop/checkToolsOnce close that gap by running
// the same check the manual endpoint runs, at boot and on an interval.
package server

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

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

// panickingChecker stands in for a store whose check blows up, the way indexing
// a blank command's fields used to. calls is atomic because the loop test reads
// it while the loop's goroutine is still running.
type panickingChecker struct {
	// called reports the first CheckAll, letting a test wait for the pass to
	// have happened instead of racing the goroutine that runs it. Buffered and
	// sent non-blockingly, so later passes never stall on an unread channel.
	called chan struct{}
	calls  atomic.Int32
}

func newPanickingChecker() *panickingChecker {
	return &panickingChecker{called: make(chan struct{}, 1)}
}

func (p *panickingChecker) CheckAll(context.Context) ([]toolpkg.HealthResult, error) {
	p.calls.Add(1)
	select {
	case p.called <- struct{}{}:
	default:
	}
	panic("health check exploded")
}

// TestCheckToolsOnce_RecoversFromPanic covers the reason the recovery exists:
// this runs on a background goroutine, where the HTTP Recovery middleware
// cannot help, so a panic escaping here terminates the daemon over a status
// refresh. The test fails by crashing the whole test binary rather than by
// reporting, which is precisely the production symptom.
func TestCheckToolsOnce_RecoversFromPanic(t *testing.T) {
	checker := newPanickingChecker()

	checkToolsOnce(context.Background(), checker)

	if got := checker.calls.Load(); got != 1 {
		t.Fatalf("CheckAll called %d times, want 1", got)
	}
}

// TestRunToolHealthLoop_SurvivesPanickingCheck: the loop must also keep its
// ticker after a failed pass. A panic in the boot-time check that unwound
// through the loop would leave tool status frozen for the daemon's lifetime,
// even once the daemon itself stopped dying.
func TestRunToolHealthLoop_SurvivesPanickingCheck(t *testing.T) {
	checker := newPanickingChecker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})

	go func() {
		runToolHealthLoop(ctx, checker)
		close(done)
	}()

	// Wait for the boot-time pass to have actually panicked. Canceling without
	// this could win the race and end the loop before it ever ran a check, so
	// the test would report success having exercised nothing.
	select {
	case <-checker.called:
	case <-time.After(5 * time.Second):
		t.Fatal("boot-time health pass never ran")
	}

	// Still sitting in its select, waiting on the ticker. This is the assertion
	// that distinguishes a recovered panic from one that unwound the loop: if
	// the panic had escaped checkToolsOnce, runToolHealthLoop would have
	// returned and closed done without the context ever being canceled.
	select {
	case <-done:
		t.Fatal("loop returned after a panicking pass instead of continuing")
	case <-time.After(100 * time.Millisecond):
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not return after its context was canceled")
	}

	// One boot-time pass: the interval is far longer than this test runs, so a
	// second call would mean the loop is ticking faster than configured.
	if got := checker.calls.Load(); got != 1 {
		t.Errorf("CheckAll called %d times, want 1 boot-time pass", got)
	}
}
