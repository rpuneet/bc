package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rpuneet/bc/pkg/workspace"
)

// initTestWorkspace creates a minimal workspace via workspace.Init under
// tmpDir/name and returns its path + stable ID.
func initTestWorkspace(t *testing.T, tmpDir, name string) (string, string) {
	t.Helper()
	wsDir := filepath.Join(tmpDir, name)
	if err := os.MkdirAll(wsDir, 0750); err != nil {
		t.Fatalf("mkdir ws: %v", err)
	}
	gitInitDir(t, wsDir)
	if _, err := workspace.Init(wsDir); err != nil {
		t.Fatalf("workspace.Init: %v", err)
	}
	return wsDir, workspace.ComputeWorkspaceID(wsDir)
}

// TestWorkspaceManagerLoadAndGet verifies lazy-loading semantics.
func TestWorkspaceManagerLoadAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	wsPath, wsID := initTestWorkspace(t, tmpDir, "ws1")

	reg := &workspace.Registry{}
	if err := reg.RegisterWithAlias(wsPath, "ws1", "w1"); err != nil {
		t.Fatalf("register: %v", err)
	}

	var factoryCalls int32
	factory := func(ctx context.Context, ws *workspace.Workspace) (*WorkspaceServices, error) {
		atomic.AddInt32(&factoryCalls, 1)
		return &WorkspaceServices{closer: func() error { return nil }}, nil
	}

	mgr := NewWorkspaceManager(reg, factory)
	defer mgr.Close() //nolint:errcheck

	// Not loaded yet — Get returns nil.
	if got := mgr.Get(wsID); got != nil {
		t.Error("Get before Load should return nil")
	}

	svc, err := mgr.Load(context.Background(), wsID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if svc == nil {
		t.Fatal("Load returned nil")
	}

	// Second call should hit cache; factory must not run again.
	if _, err := mgr.Load(context.Background(), wsID); err != nil {
		t.Fatalf("Load second: %v", err)
	}
	if n := atomic.LoadInt32(&factoryCalls); n != 1 {
		t.Errorf("factory called %d times, want 1", n)
	}

	// Get returns the cached instance.
	if got := mgr.Get(wsID); got == nil {
		t.Error("Get after Load should return services")
	}
}

// TestWorkspaceManagerUnknownID verifies error path for unregistered IDs.
func TestWorkspaceManagerUnknownID(t *testing.T) {
	reg := &workspace.Registry{}
	mgr := NewWorkspaceManager(reg, func(ctx context.Context, ws *workspace.Workspace) (*WorkspaceServices, error) {
		return &WorkspaceServices{closer: func() error { return nil }}, nil
	})
	defer mgr.Close() //nolint:errcheck

	if _, err := mgr.Load(context.Background(), "deadbeef1234"); err == nil {
		t.Error("Load of unregistered ID should error")
	}
	if _, err := mgr.Load(context.Background(), ""); err == nil {
		t.Error("Load with empty ID should error")
	}
}

// TestWorkspaceManagerLoadActive verifies LoadActive follows registry.Active.
func TestWorkspaceManagerLoadActive(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	wsPath, wsID := initTestWorkspace(t, tmpDir, "active")

	reg := &workspace.Registry{}
	_ = reg.RegisterWithAlias(wsPath, "active", "a")
	if err := reg.SetActive("a"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	mgr := NewWorkspaceManager(reg, func(ctx context.Context, ws *workspace.Workspace) (*WorkspaceServices, error) {
		return &WorkspaceServices{closer: func() error { return nil }}, nil
	})
	defer mgr.Close() //nolint:errcheck

	svc, err := mgr.LoadActive(context.Background())
	if err != nil {
		t.Fatalf("LoadActive: %v", err)
	}
	if svc == nil {
		t.Fatal("LoadActive returned nil")
	}
	if mgr.Active() == nil {
		t.Error("Active() should return loaded services")
	}
	// Active ID matches the wsID we expect.
	if entry := reg.GetActive(); entry == nil || entry.ID != wsID {
		t.Errorf("expected active ID %q, got entry=%v", wsID, entry)
	}
}

// TestWorkspaceManagerEvict verifies explicit eviction tears down services.
func TestWorkspaceManagerEvict(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	wsPath, wsID := initTestWorkspace(t, tmpDir, "ws")

	reg := &workspace.Registry{}
	_ = reg.RegisterWithAlias(wsPath, "ws", "")

	var closed int32
	mgr := NewWorkspaceManager(reg, func(ctx context.Context, ws *workspace.Workspace) (*WorkspaceServices, error) {
		return &WorkspaceServices{closer: func() error { atomic.AddInt32(&closed, 1); return nil }}, nil
	})

	if _, err := mgr.Load(context.Background(), wsID); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if err := mgr.Evict(wsID); err != nil {
		t.Fatalf("Evict: %v", err)
	}
	if atomic.LoadInt32(&closed) != 1 {
		t.Error("Evict did not call closer")
	}
	if got := mgr.Get(wsID); got != nil {
		t.Error("Get after Evict should return nil")
	}
	// Second evict is a no-op.
	if err := mgr.Evict(wsID); err != nil {
		t.Errorf("Evict second time: %v", err)
	}
}

// TestWorkspaceManagerClose tears down all loaded workspaces.
func TestWorkspaceManagerClose(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	wsPath1, wsID1 := initTestWorkspace(t, tmpDir, "one")
	wsPath2, wsID2 := initTestWorkspace(t, tmpDir, "two")

	reg := &workspace.Registry{}
	_ = reg.RegisterWithAlias(wsPath1, "one", "")
	_ = reg.RegisterWithAlias(wsPath2, "two", "")

	var closed int32
	mgr := NewWorkspaceManager(reg, func(ctx context.Context, ws *workspace.Workspace) (*WorkspaceServices, error) {
		return &WorkspaceServices{closer: func() error { atomic.AddInt32(&closed, 1); return nil }}, nil
	})

	if _, err := mgr.Load(context.Background(), wsID1); err != nil {
		t.Fatalf("Load 1: %v", err)
	}
	if _, err := mgr.Load(context.Background(), wsID2); err != nil {
		t.Fatalf("Load 2: %v", err)
	}

	if err := mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if n := atomic.LoadInt32(&closed); n != 2 {
		t.Errorf("close count = %d, want 2", n)
	}
	// Close is idempotent.
	if err := mgr.Close(); err != nil {
		t.Errorf("Close second time: %v", err)
	}
}

// TestWorkspaceManagerFactoryError ensures a failing factory returns the
// error and does not cache a partial entry.
func TestWorkspaceManagerFactoryError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	wsPath, wsID := initTestWorkspace(t, tmpDir, "ws")

	reg := &workspace.Registry{}
	_ = reg.RegisterWithAlias(wsPath, "ws", "")

	sentinel := errors.New("factory boom")
	mgr := NewWorkspaceManager(reg, func(ctx context.Context, ws *workspace.Workspace) (*WorkspaceServices, error) {
		return nil, sentinel
	})
	defer mgr.Close() //nolint:errcheck

	_, err := mgr.Load(context.Background(), wsID)
	if err == nil || !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
	if got := mgr.Get(wsID); got != nil {
		t.Error("failed load should not cache entry")
	}
}

// TestWorkspaceManagerTouch verifies Touch updates lastAccess.
func TestWorkspaceServicesTouch(t *testing.T) {
	svc := &WorkspaceServices{}
	before := svc.LastAccess()
	time.Sleep(2 * time.Millisecond)
	svc.Touch()
	after := svc.LastAccess()
	if !after.After(before) {
		t.Error("Touch did not advance lastAccess")
	}
}
