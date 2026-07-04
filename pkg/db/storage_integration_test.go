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
	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/tool"
)

// setupSharedDB opens the single global database (pinned to a per-test
// MYCEL_HOME) through the production path and registers cleanup to
// close it after the test. Returns the mycel home dir and the handle.
func setupSharedDB(t *testing.T) (string, *db.DB) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("MYCEL_HOME", home)
	d, driver, err := db.Global(nil)
	if err != nil {
		t.Fatalf("db.Global: %v", err)
	}
	if driver != "sqlite" {
		t.Fatalf("driver = %q, want sqlite", driver)
	}
	t.Cleanup(func() { _ = db.CloseGlobal() })
	return home, d
}

// ---------------------------------------------------------------------------
// 1. Shared DB lifecycle
// ---------------------------------------------------------------------------

func TestStorageGlobalLifecycle(t *testing.T) {
	t.Run("same process shares one handle", func(t *testing.T) {
		home, d1 := setupSharedDB(t)
		d2, driver, err := db.Global(nil)
		if err != nil {
			t.Fatalf("Global (second): %v", err)
		}
		if driver != "sqlite" {
			t.Errorf("driver = %q, want sqlite", driver)
		}
		if d1 != d2 {
			t.Error("expected the cached global handle on second Global")
		}
		if _, statErr := os.Stat(filepath.Join(home, db.GlobalDBFileName)); statErr != nil {
			t.Errorf("mycel.db missing: %v", statErr)
		}
	})

	t.Run("CloseGlobal evicts and Global reopens", func(t *testing.T) {
		_, d1 := setupSharedDB(t)
		if closeErr := db.CloseGlobal(); closeErr != nil {
			t.Fatalf("CloseGlobal: %v", closeErr)
		}
		if pingErr := d1.Ping(); pingErr == nil {
			t.Error("expected closed handle to fail Ping after CloseGlobal")
		}
		// Closing again is a no-op.
		if closeErr := db.CloseGlobal(); closeErr != nil {
			t.Errorf("second CloseGlobal: %v", closeErr)
		}

		d2, _, err := db.Global(nil)
		if err != nil {
			t.Fatalf("Global after CloseGlobal: %v", err)
		}
		if pingErr := d2.Ping(); pingErr != nil {
			t.Errorf("reopened handle Ping: %v", pingErr)
		}
	})
}

// TestStorageSharedDBKeyIsolation replaces the old per-workspace file
// isolation test (#3238/#3275): with the single global mycel.db, two
// "workspaces" now SHARE one database. Isolation is provided by data
// keys — channel names for subscriptions, globally-unique agent names,
// and the repo column on agents — not by separate files.
func TestStorageSharedDBKeyIsolation(t *testing.T) {
	ctx := context.Background()
	_, d := setupSharedDB(t)

	ns, err := notify.OpenStore(d, "sqlite")
	if err != nil {
		t.Fatalf("notify.OpenStore: %v", err)
	}

	// Subscriptions written by "workspace A" and "workspace B" land in
	// the same table; they are distinguished by channel and agent keys.
	if subErr := ns.Subscribe(ctx, "#engineering", "agent-a", false); subErr != nil {
		t.Fatalf("Subscribe A: %v", subErr)
	}
	if subErr := ns.Subscribe(ctx, "#ops", "agent-b", false); subErr != nil {
		t.Fatalf("Subscribe B: %v", subErr)
	}

	eng, err := ns.Subscribers(ctx, "#engineering")
	if err != nil {
		t.Fatalf("Subscribers #engineering: %v", err)
	}
	if len(eng) != 1 || eng[0].Agent != "agent-a" {
		t.Errorf("#engineering subscribers = %+v, want [agent-a]", eng)
	}
	ops, err := ns.Subscribers(ctx, "#ops")
	if err != nil {
		t.Fatalf("Subscribers #ops: %v", err)
	}
	if len(ops) != 1 || ops[0].Agent != "agent-b" {
		t.Errorf("#ops subscribers = %+v, want [agent-b]", ops)
	}

	// A second Global call from "another workspace" returns the SAME
	// handle and sees the same data — the single-DB contract.
	d2, _, err := db.Global(nil)
	if err != nil {
		t.Fatalf("Global (workspace B view): %v", err)
	}
	if d2 != d {
		t.Fatal("expected both workspaces to share the one global handle")
	}
	ns2, err := notify.OpenStore(d2, "sqlite")
	if err != nil {
		t.Fatalf("notify.OpenStore (B): %v", err)
	}
	shared, err := ns2.Subscribers(ctx, "#engineering")
	if err != nil {
		t.Fatalf("Subscribers via B: %v", err)
	}
	if len(shared) != 1 || shared[0].Agent != "agent-a" {
		t.Errorf("workspace B must see the shared subscription, got %+v", shared)
	}
}

// ---------------------------------------------------------------------------
// 2. Cross-store integration
// ---------------------------------------------------------------------------

func TestStorageCrossStoreIntegration(t *testing.T) {
	dir, d := setupSharedDB(t)
	_ = dir
	ctx := context.Background()

	// Initialize all stores against the workspace DB.
	cronStore, err := cron.Open(d, "sqlite")
	if err != nil {
		t.Fatalf("cron.Open: %v", err)
	}
	t.Cleanup(func() { _ = cronStore.Close() })

	mcpStore, err := mcp.NewStore(d, "sqlite")
	if err != nil {
		t.Fatalf("mcp.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = mcpStore.Close() })

	toolStore := tool.NewStore(d, "sqlite")
	if openErr := toolStore.Open(); openErr != nil {
		t.Fatalf("tool.Open: %v", openErr)
	}
	t.Cleanup(func() { _ = toolStore.Close() })

	eventsStore, err := events.NewSQLiteLog(d)
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

	// The workspace DB should still be usable after store Close calls.
	if err := d.Ping(); err != nil {
		t.Fatalf("workspace DB should still be pingable after store Close calls: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 3. Store isolation -- concurrent reads/writes across stores
// ---------------------------------------------------------------------------

func TestStorageIsolationConcurrent(t *testing.T) {
	dir, d := setupSharedDB(t)
	_ = dir
	ctx := context.Background()

	cronStore, err := cron.Open(d, "sqlite")
	if err != nil {
		t.Fatalf("cron.Open: %v", err)
	}
	t.Cleanup(func() { _ = cronStore.Close() })

	mcpStore, err := mcp.NewStore(d, "sqlite")
	if err != nil {
		t.Fatalf("mcp.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = mcpStore.Close() })

	eventsStore, err := events.NewSQLiteLog(d)
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
	t.Run("OpenGlobalDBWithConfig sqlite default", func(t *testing.T) {
		dir := t.TempDir()
		sqlDB, driver, err := db.OpenGlobalDBWithConfig(filepath.Join(dir, db.GlobalDBFileName), &db.StorageSettings{
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

	t.Run("OpenGlobalDBWithConfig nil config defaults to sqlite", func(t *testing.T) {
		dir := t.TempDir()
		sqlDB, driver, err := db.OpenGlobalDBWithConfig(filepath.Join(dir, db.GlobalDBFileName), nil)
		if err != nil {
			t.Fatalf("OpenWorkspaceDBWithConfig: %v", err)
		}
		defer func() { _ = sqlDB.Close() }()

		if driver != "sqlite" {
			t.Errorf("driver = %q, want %q", driver, "sqlite")
		}
	})

	t.Run("OpenGlobalDBWithConfig timescale without postgres falls back to sqlite", func(t *testing.T) {
		// Ensure DATABASE_URL is not set so it hits the config path.
		t.Setenv("DATABASE_URL", "")

		dir := t.TempDir()
		sqlDB, driver, err := db.OpenGlobalDBWithConfig(filepath.Join(dir, db.GlobalDBFileName), &db.StorageSettings{
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

	t.Run("OpenGlobalDBWithConfig legacy sql treated as timescale", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")

		dir := t.TempDir()
		sqlDB, driver, err := db.OpenGlobalDBWithConfig(filepath.Join(dir, db.GlobalDBFileName), &db.StorageSettings{
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
	_, d := setupSharedDB(t)
	ctx := context.Background()

	store, err := cron.Open(d, "sqlite")
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
	_, d := setupSharedDB(t)

	store, err := mcp.NewStore(d, "sqlite")
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
	_, d := setupSharedDB(t)
	ctx := context.Background()

	store := tool.NewStore(d, "sqlite")
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
	_, d := setupSharedDB(t)

	store, err := events.NewSQLiteLog(d)
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
	home, d := setupSharedDB(t)

	// Verify the global DB infrastructure is set up correctly for
	// channel use. Full channel CRUD tests live in pkg/notify/*_test.go.
	if _, err := os.Stat(filepath.Join(home, db.GlobalDBFileName)); err != nil {
		t.Fatalf("global mycel.db not created: %v", err)
	}
	if err := d.Ping(); err != nil {
		t.Fatalf("global db Ping: %v", err)
	}
}
