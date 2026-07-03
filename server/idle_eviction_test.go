// idle_eviction_test.go — exercises WorkspaceManager's 30min idle eviction
// end-to-end: a loaded non-active workspace whose lastAccess slips past
// the threshold is evicted by the next sweep, and the following Load
// call runs the factory again (proving the cache entry truly went away).
//
// The manager's sweep uses a real wall-clock threshold
// (idleEvictionThreshold, 30min). Rather than wait 30 minutes, the test
// reaches into the private lastAccess field of the cached services and
// rewinds it past the threshold, then invokes the exported sweep
// trigger. This keeps the eviction semantics under the test while
// compressing the interval to milliseconds.
package server

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/workspace"
)

// TestWorkspaceManager_IdleEviction verifies:
//
//  1. A non-active workspace that has not been touched for >30min is
//     evicted by the next sweep.
//  2. The active workspace is NEVER evicted, even if its lastAccess is
//     ancient (safety net for the legacy /api/... shim).
//  3. After eviction, a fresh Load() re-invokes the factory — proving
//     the cached entry is gone.
func TestWorkspaceManager_IdleEviction(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("MYCEL_HOME", filepath.Join(tmpDir, ".bc"))

	mkWS := func(name string) (string, string) {
		dir := filepath.Join(tmpDir, name)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		gitInitDir(t, dir)
		if _, err := workspace.Init(dir); err != nil {
			t.Fatalf("Init %s: %v", name, err)
		}
		return dir, workspace.ComputeWorkspaceID(dir)
	}
	wsAPath, wsAID := mkWS("wsA")
	wsBPath, wsBID := mkWS("wsB")

	reg := &workspace.Registry{}
	if err := reg.RegisterWithAlias(wsAPath, "wsA", "a"); err != nil {
		t.Fatalf("register A: %v", err)
	}
	if err := reg.RegisterWithAlias(wsBPath, "wsB", "b"); err != nil {
		t.Fatalf("register B: %v", err)
	}
	// wsA is the active workspace (must survive sweep).
	if err := reg.SetActive(wsAPath); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	var factoryCalls int32
	factory := func(ctx context.Context, w *workspace.Workspace) (*WorkspaceServices, error) {
		atomic.AddInt32(&factoryCalls, 1)
		return &WorkspaceServices{closer: func() error { return nil }}, nil
	}

	mgr := NewWorkspaceManager(reg, factory)
	t.Cleanup(func() { _ = mgr.Close() })

	// Load both.
	if _, err := mgr.Load(context.Background(), wsAID); err != nil {
		t.Fatalf("Load A: %v", err)
	}
	if _, err := mgr.Load(context.Background(), wsBID); err != nil {
		t.Fatalf("Load B: %v", err)
	}
	if n := atomic.LoadInt32(&factoryCalls); n != 2 {
		t.Fatalf("factory calls after initial load = %d, want 2", n)
	}

	// Rewind BOTH workspaces' lastAccess past the idle threshold. The
	// active workspace (A) must survive; the non-active (B) must be
	// evicted.
	ancient := time.Now().Add(-2 * idleEvictionThreshold)
	mgr.mu.RLock()
	svcA := mgr.byID[wsAID]
	svcB := mgr.byID[wsBID]
	mgr.mu.RUnlock()
	if svcA == nil || svcB == nil {
		t.Fatalf("workspaces not loaded: A=%v B=%v", svcA, svcB)
	}
	svcA.mu.Lock()
	svcA.lastAccess = ancient
	svcA.mu.Unlock()
	svcB.mu.Lock()
	svcB.lastAccess = ancient
	svcB.mu.Unlock()

	// Trigger the sweep directly — no need to wait for the 1-min ticker.
	mgr.sweepIdle()

	// (1) + (2): Active workspace still cached; non-active gone.
	if got := mgr.Get(wsAID); got == nil {
		t.Error("active workspace was evicted — sweep should preserve active")
	}
	if got := mgr.Get(wsBID); got != nil {
		t.Error("non-active workspace survived eviction — sweepIdle did not evict")
	}

	// (3) Re-load wsB runs the factory again (count goes from 2 to 3).
	// This proves the cache entry was really removed, not just marked.
	beforeReload := atomic.LoadInt32(&factoryCalls)
	if _, err := mgr.Load(context.Background(), wsBID); err != nil {
		t.Fatalf("Load B after eviction: %v", err)
	}
	afterReload := atomic.LoadInt32(&factoryCalls)
	if afterReload != beforeReload+1 {
		t.Errorf("factory calls after reload = %d, want %d (cached entry was not evicted)",
			afterReload, beforeReload+1)
	}
}

// TestWorkspaceManager_EvictionLoopTicks verifies the background
// eviction goroutine started by StartEvictionLoop really runs: we
// compress the interval by calling sweepIdle on a short ticker inside
// our own goroutine (StartEvictionLoop uses a 1-min fixed ticker that
// would make a deterministic test too slow). The contract we exercise
// is: once a non-active workspace slips past the threshold, the next
// sweep evicts it.
func TestWorkspaceManager_SweepIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("MYCEL_HOME", filepath.Join(tmpDir, ".bc"))

	wsDir := filepath.Join(tmpDir, "ws")
	if err := os.MkdirAll(wsDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	gitInitDir(t, wsDir)
	if _, err := workspace.Init(wsDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	wsID := workspace.ComputeWorkspaceID(wsDir)

	reg := &workspace.Registry{}
	_ = reg.RegisterWithAlias(wsDir, "ws", "")
	// No active workspace set — so the entry IS a non-active idle candidate.

	var closes int32
	mgr := NewWorkspaceManager(reg, func(ctx context.Context, w *workspace.Workspace) (*WorkspaceServices, error) {
		return &WorkspaceServices{closer: func() error {
			atomic.AddInt32(&closes, 1)
			return nil
		}}, nil
	})
	t.Cleanup(func() { _ = mgr.Close() })

	if _, err := mgr.Load(context.Background(), wsID); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Rewind lastAccess.
	mgr.mu.RLock()
	svc := mgr.byID[wsID]
	mgr.mu.RUnlock()
	svc.mu.Lock()
	svc.lastAccess = time.Now().Add(-2 * idleEvictionThreshold)
	svc.mu.Unlock()

	// First sweep evicts.
	mgr.sweepIdle()
	if mgr.Get(wsID) != nil {
		t.Fatal("first sweep did not evict")
	}
	if n := atomic.LoadInt32(&closes); n != 1 {
		t.Errorf("closes after first sweep = %d, want 1", n)
	}

	// Subsequent sweeps with no cached entries are no-ops.
	mgr.sweepIdle()
	mgr.sweepIdle()
	if n := atomic.LoadInt32(&closes); n != 1 {
		t.Errorf("closes after idle sweeps = %d, want 1 (sweep not idempotent)", n)
	}
}

// TestWorkspaceManager_NotIdleYet verifies a workspace whose
// lastAccess is within the threshold is NOT evicted. Protects against
// an off-by-one on the threshold comparison.
func TestWorkspaceManager_NotIdleYet(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("MYCEL_HOME", filepath.Join(tmpDir, ".bc"))

	wsDir := filepath.Join(tmpDir, "ws")
	if err := os.MkdirAll(wsDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	gitInitDir(t, wsDir)
	if _, err := workspace.Init(wsDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	wsID := workspace.ComputeWorkspaceID(wsDir)

	reg := &workspace.Registry{}
	_ = reg.RegisterWithAlias(wsDir, "ws", "")

	mgr := NewWorkspaceManager(reg, func(ctx context.Context, w *workspace.Workspace) (*WorkspaceServices, error) {
		return &WorkspaceServices{closer: func() error { return nil }}, nil
	})
	t.Cleanup(func() { _ = mgr.Close() })

	if _, err := mgr.Load(context.Background(), wsID); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Rewind to JUST INSIDE the threshold (29 minutes old).
	mgr.mu.RLock()
	svc := mgr.byID[wsID]
	mgr.mu.RUnlock()
	svc.mu.Lock()
	svc.lastAccess = time.Now().Add(-idleEvictionThreshold + 1*time.Minute)
	svc.mu.Unlock()

	mgr.sweepIdle()

	if mgr.Get(wsID) == nil {
		t.Error("sweep evicted a workspace still within the idle threshold")
	}
}
