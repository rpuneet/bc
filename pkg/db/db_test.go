package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	t.Run("creates directory and opens database", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "subdir", "test.db")

		db, err := Open(dbPath)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		// Verify directory was created
		if _, err := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(err) {
			t.Error("expected directory to be created")
		}

		// Verify database is accessible
		ctx := context.Background()
		if err := db.PingContext(ctx); err != nil {
			t.Errorf("PingContext() error = %v", err)
		}
	})

	t.Run("returns path", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "test.db")

		db, err := Open(dbPath)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		if got := db.Path(); got != dbPath {
			t.Errorf("Path() = %q, want %q", got, dbPath)
		}
	})
}

func TestOpenWithConfig(t *testing.T) {
	t.Run("applies custom config", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "test.db")

		cfg := Config{
			BusyTimeout: 10000,
			CacheSize:   4000,
			ReadOnly:    false,
		}

		db, err := OpenWithConfig(dbPath, cfg)
		if err != nil {
			t.Fatalf("OpenWithConfig() error = %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		// Verify database works
		ctx := context.Background()
		_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY)")
		if err != nil {
			t.Errorf("ExecContext() error = %v", err)
		}
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.BusyTimeout != DefaultBusyTimeout {
		t.Errorf("BusyTimeout = %d, want %d", cfg.BusyTimeout, DefaultBusyTimeout)
	}
	if cfg.CacheSize != DefaultCacheSize {
		t.Errorf("CacheSize = %d, want %d", cfg.CacheSize, DefaultCacheSize)
	}
	if cfg.ReadOnly {
		t.Error("ReadOnly should be false by default")
	}
}

func TestGlobalSingleton(t *testing.T) {
	t.Run("global open is a singleton", func(t *testing.T) {
		t.Setenv("MYCEL_HOME", t.TempDir())
		t.Cleanup(func() { _ = CloseGlobal() })

		d1, driver, err := Global(nil)
		if err != nil {
			t.Fatalf("Global() error = %v", err)
		}
		if driver != "sqlite" {
			t.Errorf("driver = %q, want sqlite", driver)
		}

		d2, _, err := Global(nil)
		if err != nil {
			t.Fatalf("Global() second call error = %v", err)
		}
		if d1 != d2 {
			t.Error("expected the same handle on every Global() call")
		}

		path, err := GlobalDBPath()
		if err != nil {
			t.Fatalf("GlobalDBPath() error = %v", err)
		}
		if d1.Path() != path {
			t.Errorf("handle path = %q, want %q", d1.Path(), path)
		}
		if _, statErr := os.Stat(path); statErr != nil {
			t.Errorf("mycel.db not created at %s: %v", path, statErr)
		}
	})

	t.Run("cfg only consulted on first open", func(t *testing.T) {
		t.Setenv("MYCEL_HOME", t.TempDir())
		t.Cleanup(func() { _ = CloseGlobal() })

		d1, _, err := Global(nil)
		if err != nil {
			t.Fatalf("Global() error = %v", err)
		}
		// Later calls with a different cfg still return the cached handle.
		d2, driver, err := Global(&StorageSettings{Default: "timescale"})
		if err != nil {
			t.Fatalf("Global(cfg) error = %v", err)
		}
		if d1 != d2 || driver != "sqlite" {
			t.Error("cfg must be ignored after first open")
		}
	})

	t.Run("CloseGlobal then Global reopens", func(t *testing.T) {
		t.Setenv("MYCEL_HOME", t.TempDir())
		t.Cleanup(func() { _ = CloseGlobal() })

		ctx := context.Background()
		d1, _, err := Global(nil)
		if err != nil {
			t.Fatalf("Global() error = %v", err)
		}
		if closeErr := CloseGlobal(); closeErr != nil {
			t.Fatalf("CloseGlobal() error = %v", closeErr)
		}
		if pingErr := d1.PingContext(ctx); pingErr == nil {
			t.Error("expected closed handle to fail Ping after CloseGlobal")
		}
		d2, _, err := Global(nil)
		if err != nil {
			t.Fatalf("Global() after CloseGlobal error = %v", err)
		}
		if pingErr := d2.PingContext(ctx); pingErr != nil {
			t.Errorf("reopened handle Ping: %v", pingErr)
		}
	})

	t.Run("GlobalDBPath honors MYCEL_HOME", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("MYCEL_HOME", home)
		path, err := GlobalDBPath()
		if err != nil {
			t.Fatalf("GlobalDBPath() error = %v", err)
		}
		if want := filepath.Join(home, GlobalDBFileName); path != want {
			t.Errorf("GlobalDBPath() = %q, want %q", path, want)
		}
	})
}

func TestPragmasApplied(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()

	// Check WAL mode
	var journalMode string
	err = db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("PRAGMA journal_mode error = %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	// Check foreign keys
	var foreignKeys int
	err = db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys)
	if err != nil {
		t.Fatalf("PRAGMA foreign_keys error = %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
}
