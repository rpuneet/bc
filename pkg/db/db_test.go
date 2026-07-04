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

func TestRegistry(t *testing.T) {
	t.Run("get opens lazily and caches", func(t *testing.T) {
		dir := t.TempDir()
		registry := NewRegistry()
		t.Cleanup(func() { _ = registry.Close() })

		d, driver, err := registry.Get(dir, nil)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if driver != "sqlite" {
			t.Errorf("driver = %q, want sqlite", driver)
		}

		// Verify same instance is returned
		d2, _, err := registry.Get(dir, nil)
		if err != nil {
			t.Fatalf("Get() second call error = %v", err)
		}
		if d != d2 {
			t.Error("expected same database instance")
		}
	})

	t.Run("distinct roots open distinct connections", func(t *testing.T) {
		registry := NewRegistry()
		t.Cleanup(func() { _ = registry.Close() })

		dA, _, err := registry.Get(t.TempDir(), nil)
		if err != nil {
			t.Fatalf("Get(A) error = %v", err)
		}
		dB, _, err := registry.Get(t.TempDir(), nil)
		if err != nil {
			t.Fatalf("Get(B) error = %v", err)
		}
		if dA == dB {
			t.Error("expected distinct connections for distinct roots")
		}
	})

	t.Run("close all", func(t *testing.T) {
		registry := NewRegistry()

		if _, _, err := registry.Get(t.TempDir(), nil); err != nil {
			t.Fatalf("Get(db1) error = %v", err)
		}
		if _, _, err := registry.Get(t.TempDir(), nil); err != nil {
			t.Fatalf("Get(db2) error = %v", err)
		}

		// Close all
		if err := registry.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	t.Run("close workspace then reopen", func(t *testing.T) {
		dir := t.TempDir()
		registry := NewRegistry()
		t.Cleanup(func() { _ = registry.Close() })

		if _, _, err := registry.Get(dir, nil); err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if closeErr := registry.CloseWorkspace(dir); closeErr != nil {
			t.Errorf("CloseWorkspace() error = %v", closeErr)
		}

		// Getting again should reopen
		if _, _, err := registry.Get(dir, nil); err != nil {
			t.Errorf("Get() after CloseWorkspace error = %v", err)
		}
	})

	t.Run("close workspace not open is no-op", func(t *testing.T) {
		registry := NewRegistry()
		t.Cleanup(func() { _ = registry.Close() })

		// Close without opening - should not error
		if err := registry.CloseWorkspace(t.TempDir()); err != nil {
			t.Errorf("CloseWorkspace() error = %v", err)
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
