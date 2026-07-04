package cron

import (
	"context"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

func setupSharedDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(dir + "/bc.db")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestStore_AddGetList(t *testing.T) {
	store, err := Open(setupSharedDB(t), "sqlite")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close() //nolint:errcheck // test

	ctx := context.Background()

	// Add a job
	job := &Job{
		Name:      "test-job",
		Schedule:  "0 9 * * *",
		AgentName: "qa-01",
		Prompt:    "Run lint",
		Enabled:   true,
	}
	if err := store.AddJob(ctx, job); err != nil { //nolint:govet // shadow in test is acceptable
		t.Fatalf("AddJob: %v", err)
	}

	// Get it back
	got, err := store.GetJob(ctx, "test-job")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got == nil {
		t.Fatal("GetJob returned nil")
	}
	if got.Name != "test-job" {
		t.Errorf("Name = %q, want %q", got.Name, "test-job")
	}
	if got.Schedule != "0 9 * * *" {
		t.Errorf("Schedule = %q, want %q", got.Schedule, "0 9 * * *")
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
	if got.NextRun == nil {
		t.Error("NextRun should be set after AddJob")
	}

	// List
	jobs, err := store.ListJobs(ctx)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("ListJobs len = %d, want 1", len(jobs))
	}

	// Get nonexistent
	missing, err := store.GetJob(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetJob nonexistent: %v", err)
	}
	if missing != nil {
		t.Error("GetJob nonexistent should return nil")
	}
}

func TestStore_SetEnabled(t *testing.T) {
	store, err := Open(setupSharedDB(t), "sqlite")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close() //nolint:errcheck // test

	ctx := context.Background()

	if err := store.AddJob(ctx, &Job{
		Name:     "toggle-job",
		Schedule: "* * * * *",
		Enabled:  true,
		Command:  "echo hi",
	}); err != nil {
		t.Fatalf("AddJob: %v", err)
	}

	// Disable
	if err := store.SetEnabled(ctx, "toggle-job", false); err != nil {
		t.Fatalf("SetEnabled false: %v", err)
	}
	j, _ := store.GetJob(ctx, "toggle-job")
	if j.Enabled {
		t.Error("job should be disabled")
	}
	if j.NextRun != nil {
		t.Error("next_run should be nil when disabled")
	}

	// Enable
	if err := store.SetEnabled(ctx, "toggle-job", true); err != nil {
		t.Fatalf("SetEnabled true: %v", err)
	}
	j, _ = store.GetJob(ctx, "toggle-job")
	if !j.Enabled {
		t.Error("job should be enabled")
	}
	if j.NextRun == nil {
		t.Error("next_run should be set when enabled")
	}

	// Nonexistent
	if err := store.SetEnabled(ctx, "nope", true); err == nil {
		t.Error("SetEnabled on nonexistent should return error")
	}
}

func TestStore_Delete(t *testing.T) {
	store, err := Open(setupSharedDB(t), "sqlite")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close() //nolint:errcheck // test

	ctx := context.Background()

	if err := store.AddJob(ctx, &Job{
		Name:     "del-job",
		Schedule: "* * * * *",
		Enabled:  true,
		Command:  "echo bye",
	}); err != nil {
		t.Fatalf("AddJob: %v", err)
	}

	if err := store.DeleteJob(ctx, "del-job"); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}

	got, _ := store.GetJob(ctx, "del-job")
	if got != nil {
		t.Error("job should be deleted")
	}

	// Delete nonexistent
	if err := store.DeleteJob(ctx, "nope"); err == nil {
		t.Error("DeleteJob nonexistent should return error")
	}
}

func TestStore_RecordManualTrigger(t *testing.T) {
	store, err := Open(setupSharedDB(t), "sqlite")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close() //nolint:errcheck // test

	ctx := context.Background()

	if err := store.AddJob(ctx, &Job{
		Name:     "trig-job",
		Schedule: "0 9 * * *",
		Enabled:  true,
		Command:  "make test",
	}); err != nil {
		t.Fatalf("AddJob: %v", err)
	}

	before := time.Now()
	if err := store.RecordManualTrigger(ctx, "trig-job"); err != nil {
		t.Fatalf("RecordManualTrigger: %v", err)
	}

	j, _ := store.GetJob(ctx, "trig-job")
	if j.RunCount != 1 {
		t.Errorf("RunCount = %d, want 1", j.RunCount)
	}
	if j.LastRun == nil || j.LastRun.Before(before) {
		t.Error("LastRun should be set to ~now")
	}
}

func TestStore_GetLogs(t *testing.T) {
	store, err := Open(setupSharedDB(t), "sqlite")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close() //nolint:errcheck // test

	ctx := context.Background()

	if err := store.AddJob(ctx, &Job{ //nolint:govet // shadow in test is acceptable
		Name:     "log-job",
		Schedule: "* * * * *",
		Enabled:  true,
		Command:  "echo test",
	}); err != nil {
		t.Fatalf("AddJob: %v", err)
	}

	// Record some runs
	for i := 0; i < 3; i++ {
		if err := store.RecordRun(ctx, &LogEntry{ //nolint:govet // shadow in test is acceptable
			JobName:    "log-job",
			Status:     "success",
			DurationMS: int64(100 + i*10),
			RunAt:      time.Now(),
		}); err != nil {
			t.Fatalf("RecordRun %d: %v", i, err)
		}
	}

	// Get all logs
	logs, err := store.GetLogs(ctx, "log-job", 0)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("GetLogs len = %d, want 3", len(logs))
	}

	// Get last 2
	logs2, err := store.GetLogs(ctx, "log-job", 2)
	if err != nil {
		t.Fatalf("GetLogs last 2: %v", err)
	}
	if len(logs2) != 2 {
		t.Errorf("GetLogs last 2 len = %d, want 2", len(logs2))
	}
}
