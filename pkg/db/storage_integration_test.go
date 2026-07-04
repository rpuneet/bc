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

// setupSharedDB opens a temporary workspace database through the
// per-workspace registry (the production path) and registers cleanup to
// close it after the test. Returns the workspace root and the handle.
func setupSharedDB(t *testing.T) (string, *db.DB) {
	t.Helper()
	dir := t.TempDir()
	d, driver, err := db.ForWorkspace(dir, nil)
	if err != nil {
		t.Fatalf("db.ForWorkspace: %v", err)
	}
	if driver != "sqlite" {
		t.Fatalf("driver = %q, want sqlite", driver)
	}
	t.Cleanup(func() { _ = db.CloseWorkspaceDB(dir) })
	return dir, d
}

// ---------------------------------------------------------------------------
// 1. Shared DB lifecycle
// ---------------------------------------------------------------------------

func TestStorageRegistryLifecycle(t *testing.T) {
	t.Run("same root twice returns the same handle", func(t *testing.T) {
		reg := db.NewRegistry()
		t.Cleanup(func() { _ = reg.Close() })

		dir := t.TempDir()
		d1, driver1, err := reg.Get(dir, nil)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if driver1 != "sqlite" {
			t.Errorf("driver = %q, want sqlite", driver1)
		}
		d2, _, err := reg.Get(dir, nil)
		if err != nil {
			t.Fatalf("Get (second): %v", err)
		}
		if d1 != d2 {
			t.Error("expected the cached handle on second Get")
		}
	})

	t.Run("different roots get isolated databases", func(t *testing.T) {
		reg := db.NewRegistry()
		t.Cleanup(func() { _ = reg.Close() })

		dirA, dirB := t.TempDir(), t.TempDir()
		dA, _, err := reg.Get(dirA, nil)
		if err != nil {
			t.Fatalf("Get A: %v", err)
		}
		dB, _, err := reg.Get(dirB, nil)
		if err != nil {
			t.Fatalf("Get B: %v", err)
		}
		if dA == dB {
			t.Fatal("expected distinct handles for distinct workspace roots")
		}

		ctx := context.Background()
		if _, execErr := dA.ExecContext(ctx, "CREATE TABLE reg_probe (id INTEGER)"); execErr != nil {
			t.Fatalf("create probe table in A: %v", execErr)
		}
		var n int
		err = dB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='reg_probe'").Scan(&n)
		if err != nil {
			t.Fatalf("query B: %v", err)
		}
		if n != 0 {
			t.Error("table created in workspace A leaked into workspace B's database")
		}
	})

	t.Run("CloseWorkspace evicts and Get reopens", func(t *testing.T) {
		reg := db.NewRegistry()
		t.Cleanup(func() { _ = reg.Close() })

		dir := t.TempDir()
		d1, _, err := reg.Get(dir, nil)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if closeErr := reg.CloseWorkspace(dir); closeErr != nil {
			t.Fatalf("CloseWorkspace: %v", closeErr)
		}
		if pingErr := d1.Ping(); pingErr == nil {
			t.Error("expected closed handle to fail Ping after CloseWorkspace")
		}
		// Closing again is a no-op.
		if closeErr := reg.CloseWorkspace(dir); closeErr != nil {
			t.Errorf("second CloseWorkspace: %v", closeErr)
		}

		d2, _, err := reg.Get(dir, nil)
		if err != nil {
			t.Fatalf("Get after CloseWorkspace: %v", err)
		}
		if pingErr := d2.Ping(); pingErr != nil {
			t.Errorf("reopened handle Ping: %v", pingErr)
		}
	})

	t.Run("Close closes everything", func(t *testing.T) {
		reg := db.NewRegistry()
		dirA, dirB := t.TempDir(), t.TempDir()
		dA, _, err := reg.Get(dirA, nil)
		if err != nil {
			t.Fatalf("Get A: %v", err)
		}
		dB, _, err := reg.Get(dirB, nil)
		if err != nil {
			t.Fatalf("Get B: %v", err)
		}
		if err := reg.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if dA.Ping() == nil || dB.Ping() == nil {
			t.Error("expected all handles closed after registry Close")
		}
	})
}

// TestStorageWorkspaceIsolation is the multi-workspace bleed regression
// test for issue #3238: stores built for two workspaces in the same
// process must land in each workspace's own database. Notify
// subscriptions were the observable symptom (workspace B's Slack
// subscriptions showed up in workspace A).
func TestStorageWorkspaceIsolation(t *testing.T) {
	ctx := context.Background()

	wsA, wsB := t.TempDir(), t.TempDir()
	for _, dir := range []string{wsA, wsB} {
		t.Cleanup(func() { _ = db.CloseWorkspaceDB(dir) })
	}

	openNotify := func(t *testing.T, root string) *notify.Store {
		t.Helper()
		d, driver, err := db.ForWorkspace(root, nil)
		if err != nil {
			t.Fatalf("ForWorkspace(%s): %v", root, err)
		}
		ns, err := notify.OpenStore(d, driver)
		if err != nil {
			t.Fatalf("notify.OpenStore(%s): %v", root, err)
		}
		return ns
	}

	nsA := openNotify(t, wsA)
	nsB := openNotify(t, wsB)

	tests := []struct {
		name    string
		store   *notify.Store
		other   *notify.Store
		channel string
		agent   string
	}{
		{"workspace A subscription stays in A", nsA, nsB, "#engineering", "agent-a"},
		{"workspace B subscription stays in B", nsB, nsA, "#ops", "agent-b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.store.Subscribe(ctx, tt.channel, tt.agent, false); err != nil {
				t.Fatalf("Subscribe: %v", err)
			}
			mine, err := tt.store.Subscribers(ctx, tt.channel)
			if err != nil {
				t.Fatalf("Subscribers (own): %v", err)
			}
			if len(mine) != 1 || mine[0].Agent != tt.agent {
				t.Fatalf("own workspace subscribers = %+v, want [%s]", mine, tt.agent)
			}
			theirs, err := tt.other.Subscribers(ctx, tt.channel)
			if err != nil {
				t.Fatalf("Subscribers (other): %v", err)
			}
			if len(theirs) != 0 {
				t.Errorf("subscription bled into the other workspace: %+v", theirs)
			}
		})
	}

	// The two stores must genuinely sit on different database files.
	if _, err := os.Stat(filepath.Join(wsA, ".bc", "bc.db")); err != nil {
		t.Errorf("workspace A bc.db missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wsB, ".bc", "bc.db")); err != nil {
		t.Errorf("workspace B bc.db missing: %v", err)
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
	dir, d := setupSharedDB(t)

	// Verify the workspace DB infrastructure is set up correctly for
	// channel use. Full channel CRUD tests live in pkg/notify/*_test.go.
	if _, err := os.Stat(filepath.Join(dir, ".bc", "bc.db")); err != nil {
		t.Fatalf("workspace bc.db not created: %v", err)
	}
	if err := d.Ping(); err != nil {
		t.Fatalf("workspace db Ping: %v", err)
	}
}
