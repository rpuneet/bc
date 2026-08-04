package server

import (
	"context"
	"sync"
	"testing"
	"time"
)

// countingSyncer records how often it was asked to reconcile and signals the
// first call, so a test can assert the startup pass without waiting for a tick.
type countingSyncer struct {
	first chan struct{}
	calls int
	mu    sync.Mutex
	once  sync.Once
}

func (c *countingSyncer) SyncSessions(context.Context) (int, int) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	c.once.Do(func() { close(c.first) })
	return 1, 1
}

func (c *countingSyncer) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// The startup pass is the one that matters most: records loaded from disk
// describe the machine as it was when the daemon last ran, so an agent listed as
// working may have had no session for days. Waiting a full interval to say so is
// a window in which the UI is knowingly wrong.
func TestReconcilerSyncsAtStartupWithoutWaitingForATick(t *testing.T) {
	c := &countingSyncer{first: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go runSessionReconciler(ctx, c)

	select {
	case <-c.first:
	case <-time.After(2 * time.Second):
		t.Fatal("no reconciliation at startup")
	}
}

// Canceling the context is how the daemon shuts the loop down; it must return
// rather than keep a goroutine alive past shutdown.
func TestReconcilerStopsWhenTheContextIsCanceled(t *testing.T) {
	c := &countingSyncer{first: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})
	go func() {
		runSessionReconciler(ctx, c)
		close(done)
	}()

	<-c.first
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reconciler did not return after cancel")
	}

	after := c.count()
	time.Sleep(50 * time.Millisecond)
	if c.count() != after {
		t.Error("reconciler kept working after the context was canceled")
	}
}

// A nil service is what a degraded daemon has; the loop must return rather than
// panic on the first tick.
func TestReconcilerWithNoAgentService(t *testing.T) {
	done := make(chan struct{})
	go func() {
		runSessionReconciler(t.Context(), nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reconciler did not return for a nil service")
	}
}
