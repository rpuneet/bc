package db_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/cron"
	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/mcp"
	"github.com/rpuneet/mycel/pkg/tool"
)

// setupSharedDB opens a temporary SQLite database and sets it as the shared DB.
// It registers cleanup to reset the shared state after the test.
func setupSharedDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bc.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetShared(d.DB, "sqlite")
	t.Cleanup(func() {
		db.SetShared(nil, "")
		_ = d.Close()
	})
	return dir
}

// ---------------------------------------------------------------------------
// 1. Shared DB lifecycle
// ---------------------------------------------------------------------------

func TestStorageSharedLifecycle(t *testing.T) {
	t.Run("SetShared and Shared return expected values", func(t *testing.T) {
		dir := t.TempDir()
		d, err := db.Open(filepath.Join(dir, "lifecycle.db"))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = d.Close() }()

		// Before setting, should be nil.
		db.SetShared(nil, "")
		if got := db.Shared(); got != nil {
			t.Error("expected Shared() to be nil before SetShared")
		}

		db.SetShared(d.DB, "sqlite")
		defer db.SetShared(nil, "")

		if got := db.Shared(); got == nil {
			t.Fatal("expected Shared() to be non-nil after SetShared")
		}
		if got := db.SharedDriver(); got != "sqlite" {
			t.Errorf("SharedDriver() = %q, want %q", got, "sqlite")
		}
	})

	t.Run("SharedWrapped returns nil when no shared DB", func(t *testing.T) {
		db.SetShared(nil, "")
		if got := db.SharedWrapped(); got != nil {
			t.Error("expected SharedWrapped() to return nil when no shared DB")
		}
	})

	t.Run("SharedWrapped returns wrapper when shared DB set", func(t *testing.T) {
		dir := t.TempDir()
		d, err := db.Open(filepath.Join(dir, "wrapped.db"))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = d.Close() }()

		db.SetShared(d.DB, "sqlite")
		defer db.SetShared(nil, "")

		wrapped := db.SharedWrapped()
		if wrapped == nil {
			t.Fatal("expected SharedWrapped() to return non-nil")
		}
		if wrapped.DB != d.DB {
			t.Error("wrapped.DB should point to the same *sql.DB")
		}
	})

	t.Run("CloseShared cleans up", func(t *testing.T) {
		dir := t.TempDir()
		d, err := db.Open(filepath.Join(dir, "closeshared.db"))
		if err != nil {
			t.Fatalf("open: %v", err)
		}

		db.SetShared(d.DB, "sqlite")

		if err := db.CloseShared(); err != nil {
			t.Fatalf("CloseShared() error: %v", err)
		}

		if got := db.Shared(); got != nil {
			t.Error("expected Shared() to be nil after CloseShared")
		}

		// Calling CloseShared again should be a no-op.
		if err := db.CloseShared(); err != nil {
			t.Errorf("second CloseShared() error: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// 2. Cross-store integration
// ---------------------------------------------------------------------------

func TestStorageCrossStoreIntegration(t *testing.T) {
	dir := setupSharedDB(t)
	ctx := context.Background()

	// Initialize all stores against the shared DB.
	cronStore, err := cron.Open(dir)
	if err != nil {
		t.Fatalf("cron.Open: %v", err)
	}
	t.Cleanup(func() { _ = cronStore.Close() })

	mcpStore, err := mcp.NewStore(dir)
	if err != nil {
		t.Fatalf("mcp.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = mcpStore.Close() })

	toolStore := tool.NewStore(dir)
	if openErr := toolStore.Open(); openErr != nil {
		t.Fatalf("tool.Open: %v", openErr)
	}
	t.Cleanup(func() { _ = toolStore.Close() })

	eventsStore, err := events.NewSQLiteLog(filepath.Join(dir, "events.db"))
	if err != nil {
		t.Fatalf("events.NewSQLiteLog: %v", err)
	}
	t.Cleanup(func() { _ = eventsStore.Close() })

	// Verify each store can CRUD independently.

	// Cron: add and retrieve a job.
	err = cronStore.AddJob(ctx, &cron.Job{
		Name:     "cross-test-job",
		Schedule: "*/5 * * * *",
		Prompt:   "hello",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("cron AddJob: %v", err)
	}
	job, err := cronStore.GetJob(ctx, "cross-test-job")
	if err != nil {
		t.Fatalf("cron GetJob: %v", err)
	}
	if job == nil || job.Name != "cross-test-job" {
		t.Fatalf("expected cron job 'cross-test-job', got %v", job)
	}

	// MCP: add and retrieve a server config.
	err = mcpStore.Add(&mcp.ServerConfig{
		Name:      "cross-test-mcp",
		Transport: mcp.TransportStdio,
		Command:   "echo",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("mcp Add: %v", err)
	}
	mcpCfg, err := mcpStore.Get("cross-test-mcp")
	if err != nil {
		t.Fatalf("mcp Get: %v", err)
	}
	if mcpCfg == nil || mcpCfg.Name != "cross-test-mcp" {
		t.Fatalf("expected mcp config 'cross-test-mcp', got %v", mcpCfg)
	}

	// Tool: add and retrieve a custom tool (builtins are seeded by Open).
	err = toolStore.Add(ctx, &tool.Tool{
		Name:    "cross-test-tool",
		Command: "cross-cmd",
		Type:    tool.ToolTypeCLI,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("tool Add: %v", err)
	}
	got, err := toolStore.Get(ctx, "cross-test-tool")
	if err != nil {
		t.Fatalf("tool Get: %v", err)
	}
	if got == nil || got.Name != "cross-test-tool" {
		t.Fatalf("expected tool 'cross-test-tool', got %v", got)
	}

	// Events: append and read.
	err = eventsStore.Append(events.Event{
		Type:    events.AgentSpawned,
		Agent:   "cross-agent",
		Message: "spawned for cross test",
	})
	if err != nil {
		t.Fatalf("events Append: %v", err)
	}
	evts, err := eventsStore.Read()
	if err != nil {
		t.Fatalf("events Read: %v", err)
	}
	if len(evts) == 0 {
		t.Fatal("expected at least one event")
	}

	// Verify stores don't close the shared connection (Close is no-op for shared stores).
	_ = cronStore.Close()
	_ = mcpStore.Close()
	_ = toolStore.Close()
	_ = eventsStore.Close()

	// The shared DB should still be usable after store Close calls.
	shared := db.Shared()
	if shared == nil {
		t.Fatal("shared DB should still be set after store Close calls")
	}
	if err := shared.Ping(); err != nil {
		t.Fatalf("shared DB should still be pingable after store Close calls: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 3. Store isolation -- concurrent reads/writes across stores
// ---------------------------------------------------------------------------

func TestStorageIsolationConcurrent(t *testing.T) {
	dir := setupSharedDB(t)
	ctx := context.Background()

	cronStore, err := cron.Open(dir)
	if err != nil {
		t.Fatalf("cron.Open: %v", err)
	}
	t.Cleanup(func() { _ = cronStore.Close() })

	mcpStore, err := mcp.NewStore(dir)
	if err != nil {
		t.Fatalf("mcp.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = mcpStore.Close() })

	eventsStore, err := events.NewSQLiteLog(filepath.Join(dir, "events.db"))
	if err != nil {
		t.Fatalf("events.NewSQLiteLog: %v", err)
	}
	t.Cleanup(func() { _ = eventsStore.Close() })

	const iterations = 20
	var wg sync.WaitGroup
	errs := make(chan error, iterations*3)

	// Concurrent cron writes.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range iterations {
			name := fmt.Sprintf("concurrent-cron-%d", i)
			if addErr := cronStore.AddJob(ctx, &cron.Job{
				Name:     name,
				Schedule: "0 * * * *",
				Prompt:   "test",
				Enabled:  true,
			}); addErr != nil {
				errs <- fmt.Errorf("cron AddJob %s: %w", name, addErr)
				return
			}
		}
	}()

	// Concurrent MCP writes.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range iterations {
			name := fmt.Sprintf("concurrent-mcp-%d", i)
			if addErr := mcpStore.Add(&mcp.ServerConfig{
				Name:      name,
				Transport: mcp.TransportStdio,
				Command:   "echo",
				Enabled:   true,
			}); addErr != nil {
				errs <- fmt.Errorf("mcp Add %s: %w", name, addErr)
				return
			}
		}
	}()

	// Concurrent event writes.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range iterations {
			if appendErr := eventsStore.Append(events.Event{
				Type:    events.AgentSpawned,
				Agent:   fmt.Sprintf("agent-%d", i),
				Message: "concurrent test",
			}); appendErr != nil {
				errs <- fmt.Errorf("events Append %d: %w", i, appendErr)
				return
			}
		}
	}()

	wg.Wait()
	close(errs)

	for e := range errs {
		t.Errorf("concurrent error: %v", e)
	}

	// Verify all rows landed.
	jobs, err := cronStore.ListJobs(ctx)
	if err != nil {
		t.Fatalf("cron ListJobs: %v", err)
	}
	if len(jobs) < iterations {
		t.Errorf("expected at least %d cron jobs, got %d", iterations, len(jobs))
	}

	mcpList, err := mcpStore.List()
	if err != nil {
		t.Fatalf("mcp List: %v", err)
	}
	if len(mcpList) < iterations {
		t.Errorf("expected at least %d mcp configs, got %d", iterations, len(mcpList))
	}

	evtList, err := eventsStore.Read()
	if err != nil {
		t.Fatalf("events Read: %v", err)
	}
	if len(evtList) < iterations {
		t.Errorf("expected at least %d events, got %d", iterations, len(evtList))
	}
}

// ---------------------------------------------------------------------------
// 4. Config validation
// ---------------------------------------------------------------------------

func TestStorageConfigValidation(t *testing.T) {
	t.Run("OpenWorkspaceDBWithConfig sqlite default", func(t *testing.T) {
		dir := t.TempDir()
		sqlDB, driver, err := db.OpenWorkspaceDBWithConfig(dir, &db.StorageSettings{
			Default: "sqlite",
		})
		if err != nil {
			t.Fatalf("OpenWorkspaceDBWithConfig: %v", err)
		}
		defer func() { _ = sqlDB.Close() }()

		if driver != "sqlite" {
			t.Errorf("driver = %q, want %q", driver, "sqlite")
		}
		if err := sqlDB.Ping(); err != nil {
			t.Errorf("Ping: %v", err)
		}
	})

	t.Run("OpenWorkspaceDBWithConfig nil config defaults to sqlite", func(t *testing.T) {
		dir := t.TempDir()
		sqlDB, driver, err := db.OpenWorkspaceDBWithConfig(dir, nil)
		if err != nil {
			t.Fatalf("OpenWorkspaceDBWithConfig: %v", err)
		}
		defer func() { _ = sqlDB.Close() }()

		if driver != "sqlite" {
			t.Errorf("driver = %q, want %q", driver, "sqlite")
		}
	})

	t.Run("OpenWorkspaceDBWithConfig timescale without postgres falls back to sqlite", func(t *testing.T) {
		// Ensure DATABASE_URL is not set so it hits the config path.
		t.Setenv("DATABASE_URL", "")

		dir := t.TempDir()
		sqlDB, driver, err := db.OpenWorkspaceDBWithConfig(dir, &db.StorageSettings{
			Default: "timescale",
			Timescale: db.TimescaleSettings{
				Host: "127.0.0.1",
				Port: 59999, // non-existent port
			},
		})
		if err != nil {
			t.Fatalf("expected sqlite fallback when postgres is unreachable, got error: %v", err)
		}
		defer func() { _ = sqlDB.Close() }()
		if driver != "sqlite" {
			t.Errorf("driver = %q, want %q (fallback)", driver, "sqlite")
		}
	})

	t.Run("OpenWorkspaceDBWithConfig legacy sql treated as timescale", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")

		dir := t.TempDir()
		sqlDB, driver, err := db.OpenWorkspaceDBWithConfig(dir, &db.StorageSettings{
			Default: "sql",
			Timescale: db.TimescaleSettings{
				Host: "127.0.0.1",
				Port: 59999,
			},
		})
		// Should attempt timescale connection, fail, and fall back to SQLite.
		if err != nil {
			t.Fatalf("expected sqlite fallback for legacy 'sql' default with unreachable postgres, got error: %v", err)
		}
		defer func() { _ = sqlDB.Close() }()
		if driver != "sqlite" {
			t.Errorf("driver = %q, want %q (fallback)", driver, "sqlite")
		}
	})

	t.Run("TimescaleSettings DSN builds correct string", func(t *testing.T) {
		tests := []struct { //nolint:govet // field order matches test readability
			name     string
			want     string
			settings db.TimescaleSettings
		}{
			{
				name:     "all defaults",
				settings: db.TimescaleSettings{},
				want:     "postgres://bc:bc@localhost:5432/bc",
			},
			{
				name: "custom values",
				settings: db.TimescaleSettings{
					Host:     "db.example.com",
					Port:     5433,
					User:     "admin",
					Password: "secret",
					Database: "mydb",
				},
				want: "postgres://admin:secret@db.example.com:5433/mydb",
			},
			{
				name: "partial overrides",
				settings: db.TimescaleSettings{
					Host: "custom-host",
					Port: 5434,
				},
				want: "postgres://bc:bc@custom-host:5434/bc",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := tt.settings.DSN()
				if got != tt.want {
					t.Errorf("DSN() = %q, want %q", got, tt.want)
				}
			})
		}
	})
}

// ---------------------------------------------------------------------------
// 5. Store-specific smoke tests
// ---------------------------------------------------------------------------

func TestStorageCronSmoke(t *testing.T) {
	dir := setupSharedDB(t)
	ctx := context.Background()

	store, err := cron.Open(dir)
	if err != nil {
		t.Fatalf("cron.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// AddJob
	err = store.AddJob(ctx, &cron.Job{
		Name:      "smoke-job",
		Schedule:  "0 12 * * *",
		AgentName: "agent-1",
		Prompt:    "do lint",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("AddJob: %v", err)
	}

	// GetJob
	job, err := store.GetJob(ctx, "smoke-job")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if job == nil {
		t.Fatal("expected job, got nil")
	}
	if job.Schedule != "0 12 * * *" {
		t.Errorf("Schedule = %q, want %q", job.Schedule, "0 12 * * *")
	}
	if job.AgentName != "agent-1" {
		t.Errorf("AgentName = %q, want %q", job.AgentName, "agent-1")
	}

	// ListJobs
	jobs, err := store.ListJobs(ctx)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("ListJobs returned %d jobs, want 1", len(jobs))
	}

	// SetEnabled (disable)
	if setErr := store.SetEnabled(ctx, "smoke-job", false); setErr != nil {
		t.Fatalf("SetEnabled(false): %v", setErr)
	}
	job, _ = store.GetJob(ctx, "smoke-job")
	if job.Enabled {
		t.Error("expected job to be disabled")
	}

	// SetEnabled (re-enable)
	if setErr := store.SetEnabled(ctx, "smoke-job", true); setErr != nil {
		t.Fatalf("SetEnabled(true): %v", setErr)
	}
	job, _ = store.GetJob(ctx, "smoke-job")
	if !job.Enabled {
		t.Error("expected job to be enabled")
	}

	// RecordRun
	err = store.RecordRun(ctx, &cron.LogEntry{
		JobName:    "smoke-job",
		Status:     "success",
		DurationMS: 150,
		CostUSD:    0.01,
		Output:     "all good",
		RunAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	// GetLogs
	logs, err := store.GetLogs(ctx, "smoke-job", 10)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("GetLogs returned %d entries, want 1", len(logs))
	}
	if logs[0].Status != "success" {
		t.Errorf("log status = %q, want %q", logs[0].Status, "success")
	}

	// Verify run count was incremented.
	job, _ = store.GetJob(ctx, "smoke-job")
	if job.RunCount != 1 {
		t.Errorf("RunCount = %d, want 1", job.RunCount)
	}

	// DeleteJob
	if err := store.DeleteJob(ctx, "smoke-job"); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	job, _ = store.GetJob(ctx, "smoke-job")
	if job != nil {
		t.Error("expected job to be nil after delete")
	}
}

func TestStorageMCPSmoke(t *testing.T) {
	dir := setupSharedDB(t)

	store, err := mcp.NewStore(dir)
	if err != nil {
		t.Fatalf("mcp.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Add
	err = store.Add(&mcp.ServerConfig{
		Name:      "smoke-mcp",
		Transport: mcp.TransportStdio,
		Command:   "/usr/bin/echo",
		Args:      []string{"--flag"},
		Env:       map[string]string{"KEY": "val"},
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Get
	cfg, err := store.Get("smoke-mcp")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.Command != "/usr/bin/echo" {
		t.Errorf("Command = %q, want %q", cfg.Command, "/usr/bin/echo")
	}
	if len(cfg.Args) != 1 || cfg.Args[0] != "--flag" {
		t.Errorf("Args = %v, want [--flag]", cfg.Args)
	}
	if cfg.Env["KEY"] != "val" {
		t.Errorf("Env[KEY] = %q, want %q", cfg.Env["KEY"], "val")
	}

	// List
	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List returned %d, want 1", len(list))
	}

	// SetEnabled
	if err := store.SetEnabled("smoke-mcp", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	cfg, _ = store.Get("smoke-mcp")
	if cfg.Enabled {
		t.Error("expected disabled")
	}

	// Remove
	if err := store.Remove("smoke-mcp"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	cfg, _ = store.Get("smoke-mcp")
	if cfg != nil {
		t.Error("expected nil after Remove")
	}
}

func TestStorageToolSmoke(t *testing.T) {
	dir := setupSharedDB(t)
	ctx := context.Background()

	store := tool.NewStore(dir)
	if err := store.Open(); err != nil {
		t.Fatalf("tool.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Builtins should be seeded.
	builtins, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List builtins: %v", err)
	}
	if len(builtins) == 0 {
		t.Fatal("expected seeded builtin tools, got 0")
	}

	// Add a custom tool.
	err = store.Add(ctx, &tool.Tool{
		Name:    "smoke-tool",
		Command: "smoke-cmd",
		Type:    tool.ToolTypeCLI,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Get
	got, err := store.Get(ctx, "smoke-tool")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.Name != "smoke-tool" {
		t.Fatalf("expected 'smoke-tool', got %v", got)
	}
	// The internal add() method does not persist Type, so it defaults to "provider".
	if got.Type != tool.ToolTypeProvider {
		t.Errorf("Type = %q, want %q", got.Type, tool.ToolTypeProvider)
	}

	// Update
	got.Command = "updated-cmd"
	if err := store.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = store.Get(ctx, "smoke-tool")
	if got.Command != "updated-cmd" {
		t.Errorf("Command after update = %q, want %q", got.Command, "updated-cmd")
	}

	// SetEnabled
	if err := store.SetEnabled(ctx, "smoke-tool", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	got, _ = store.Get(ctx, "smoke-tool")
	if got.Enabled {
		t.Error("expected disabled")
	}

	// Delete
	if err := store.Delete(ctx, "smoke-tool"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = store.Get(ctx, "smoke-tool")
	if got != nil {
		t.Error("expected nil after Delete")
	}
}

func TestStorageEventsSmoke(t *testing.T) {
	dir := setupSharedDB(t)

	store, err := events.NewSQLiteLog(filepath.Join(dir, "events.db"))
	if err != nil {
		t.Fatalf("events.NewSQLiteLog: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Append multiple events.
	for _, ev := range []events.Event{
		{Type: events.AgentSpawned, Agent: "a1", Message: "spawned"},
		{Type: events.WorkAssigned, Agent: "a1", Message: "assigned work"},
		{Type: events.AgentSpawned, Agent: "a2", Message: "spawned second"},
		{Type: events.WorkCompleted, Agent: "a1", Message: "done"},
	} {
		if appendErr := store.Append(ev); appendErr != nil {
			t.Fatalf("Append: %v", appendErr)
		}
	}

	// Read all.
	all, err := store.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("Read returned %d events, want 4", len(all))
	}

	// ReadLast(2) should return last 2 in chronological order.
	last2, err := store.ReadLast(2)
	if err != nil {
		t.Fatalf("ReadLast: %v", err)
	}
	if len(last2) != 2 {
		t.Errorf("ReadLast(2) returned %d events, want 2", len(last2))
	}
	// The first of the two should be the third event (agent a2 spawn).
	if last2[0].Agent != "a2" {
		t.Errorf("ReadLast[0].Agent = %q, want %q", last2[0].Agent, "a2")
	}

	// ReadByAgent.
	a1Events, err := store.ReadByAgent("a1")
	if err != nil {
		t.Fatalf("ReadByAgent: %v", err)
	}
	if len(a1Events) != 3 {
		t.Errorf("ReadByAgent(a1) returned %d, want 3", len(a1Events))
	}

	a2Events, err := store.ReadByAgent("a2")
	if err != nil {
		t.Fatalf("ReadByAgent: %v", err)
	}
	if len(a2Events) != 1 {
		t.Errorf("ReadByAgent(a2) returned %d, want 1", len(a2Events))
	}
}

func TestStorageChannelSharedDBReady(t *testing.T) {
	dir := setupSharedDB(t)

	// Channel SQLiteStore opens its own connection to .bc/bc.db.
	// Verify the shared DB infrastructure is set up correctly for channel use.
	// Full channel CRUD tests live in pkg/notify/*_test.go.
	bcDir := filepath.Join(dir, ".bc")
	if mkErr := os.MkdirAll(bcDir, 0750); mkErr != nil {
		t.Fatalf("mkdir .bc: %v", mkErr)
	}

	shared := db.Shared()
	if shared == nil {
		t.Fatal("Shared() should be non-nil for channel store")
	}
	if db.SharedDriver() != "sqlite" {
		t.Errorf("SharedDriver() = %q, want %q", db.SharedDriver(), "sqlite")
	}
	if err := shared.Ping(); err != nil {
		t.Fatalf("shared.Ping: %v", err)
	}
}
