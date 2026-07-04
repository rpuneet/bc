package cron

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// openTestStore creates a temporary cron store for testing.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(dir + "/bc.db")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	store, err := Open(d, "sqlite")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() }) //nolint:errcheck // test
	return store
}

func TestScheduler_EnabledJobGetExecuted(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	// Add an enabled job with a command.
	job := &Job{
		Name:     "echo-job",
		Schedule: "* * * * *", // every minute
		Command:  "echo hello",
		Enabled:  true,
	}
	if err := store.AddJob(ctx, job); err != nil {
		t.Fatalf("AddJob: %v", err)
	}

	// Force next_run to be in the past so the scheduler picks it up.
	past := time.Now().Add(-2 * time.Minute)
	_, err := store.db.ExecContext(ctx, `UPDATE cron_jobs SET next_run = ? WHERE name = ?`, past, "echo-job")
	if err != nil {
		t.Fatalf("update next_run: %v", err)
	}

	var mu sync.Mutex
	var execCalls []string

	sched := NewScheduler(store, t.TempDir())
	sched.execFn = func(_ context.Context, command string, _ io.Writer) error {
		mu.Lock()
		execCalls = append(execCalls, command)
		mu.Unlock()
		return nil
	}

	// Run a single tick.
	sched.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(execCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(execCalls))
	}
	if execCalls[0] != "echo hello" {
		t.Errorf("expected command %q, got %q", "echo hello", execCalls[0])
	}

	// Verify a log entry was recorded.
	logs, err := store.GetLogs(ctx, "echo-job", 10)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
	if logs[0].Status != "success" {
		t.Errorf("expected status %q, got %q", "success", logs[0].Status)
	}
}

func TestScheduler_DisabledJobsSkipped(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	job := &Job{
		Name:     "disabled-job",
		Schedule: "* * * * *",
		Command:  "echo should-not-run",
		Enabled:  false,
	}
	if err := store.AddJob(ctx, job); err != nil {
		t.Fatalf("AddJob: %v", err)
	}

	// Force next_run to the past.
	past := time.Now().Add(-2 * time.Minute)
	_, err := store.db.ExecContext(ctx, `UPDATE cron_jobs SET next_run = ? WHERE name = ?`, past, "disabled-job")
	if err != nil {
		t.Fatalf("update next_run: %v", err)
	}

	var mu sync.Mutex
	var execCalls []string

	sched := NewScheduler(store, t.TempDir())
	sched.execFn = func(_ context.Context, command string, _ io.Writer) error {
		mu.Lock()
		execCalls = append(execCalls, command)
		mu.Unlock()
		return nil
	}

	sched.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(execCalls) != 0 {
		t.Fatalf("expected 0 exec calls for disabled job, got %d", len(execCalls))
	}
}

func TestScheduler_FutureNextRunSkipped(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	job := &Job{
		Name:     "future-job",
		Schedule: "* * * * *",
		Command:  "echo not-yet",
		Enabled:  true,
	}
	if err := store.AddJob(ctx, job); err != nil {
		t.Fatalf("AddJob: %v", err)
	}

	// next_run is already in the future from AddJob, so no override needed.

	var mu sync.Mutex
	var execCalls []string

	sched := NewScheduler(store, t.TempDir())
	sched.execFn = func(_ context.Context, command string, _ io.Writer) error {
		mu.Lock()
		execCalls = append(execCalls, command)
		mu.Unlock()
		return nil
	}

	sched.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(execCalls) != 0 {
		t.Fatalf("expected 0 exec calls for future job, got %d", len(execCalls))
	}
}

func TestScheduler_ContextCancellation(t *testing.T) {
	store := openTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())

	sched := NewScheduler(store, t.TempDir())
	sched.interval = 10 * time.Millisecond

	done := make(chan struct{})
	go func() {
		sched.Run(ctx)
		close(done)
	}()

	// Cancel immediately and verify Run returns.
	cancel()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not stop after context cancellation")
	}
}

func TestIsDue(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)

	tests := []struct {
		job  *Job
		name string
		want bool
	}{
		{
			name: "nil next_run",
			job:  &Job{NextRun: nil},
			want: false,
		},
		{
			name: "past next_run",
			job:  &Job{NextRun: &past},
			want: true,
		},
		{
			name: "future next_run",
			job:  &Job{NextRun: &future},
			want: false,
		},
		{
			name: "exactly now",
			job:  &Job{NextRun: &now},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDue(tt.job, now)
			if got != tt.want {
				t.Errorf("isDue() = %v, want %v", got, tt.want)
			}
		})
	}
}
