//go:build postgres

// Postgres-backed tests for pkg/mcp. Opt-in via `-tags postgres`.
//
// These tests require a live Postgres instance. They honor DATABASE_URL,
// falling back to the default bcdb DSN (postgres://bc:bc@localhost:5432/bc).
// Each test truncates mcp_servers on entry + exit so concurrent runs stay
// sane — don't run this with -parallel against a shared DB.
package mcp

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	bcdb "github.com/rpuneet/bc/pkg/db"
)

// setupPostgresStore opens a real Postgres connection, creates a fresh
// mcp_servers table (truncating any leftover rows from previous runs),
// and returns a Store backed by PostgresStore.
func setupPostgresStore(t *testing.T) *Store {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = bcdb.DefaultPostgresDSN
	}

	pgDB, err := bcdb.OpenPostgres(dsn)
	if err != nil {
		t.Skipf("postgres not reachable at %s: %v", dsn, err)
	}

	pg := NewPostgresStore(pgDB)
	if err := pg.InitSchema(); err != nil {
		_ = pgDB.Close()
		t.Fatalf("init postgres schema: %v", err)
	}

	if _, err := pgDB.Exec(`TRUNCATE mcp_servers`); err != nil {
		_ = pgDB.Close()
		t.Fatalf("truncate mcp_servers: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pgDB.Exec(`TRUNCATE mcp_servers`)
		_ = pgDB.Close()
	})

	return &Store{pg: pg}
}

// countRow returns the number of mcp_servers rows with the given name.
// Used to assert a row was NOT deleted as a side effect of a failed update
// and was NOT silently created by an update against an unknown name.
func countRow(t *testing.T, pg *sql.DB, name string) int {
	t.Helper()
	var n int
	if err := pg.QueryRow(`SELECT COUNT(*) FROM mcp_servers WHERE name = $1`, name).Scan(&n); err != nil {
		t.Fatalf("count row: %v", err)
	}
	return n
}

func TestUpdateEnvPostgresAtomic(t *testing.T) {
	s := setupPostgresStore(t)

	const name = "pg-update-env"
	if err := s.Add(&ServerConfig{
		Name:      name,
		Transport: TransportStdio,
		Command:   "bin",
		Env: map[string]string{
			"A": "1",
			"B": "2",
			"C": "3",
		},
		Enabled: true,
	}); err != nil {
		t.Fatalf("seed Add: %v", err)
	}

	// Capture the original created_at so we can prove the row was not
	// deleted & re-inserted by the update path.
	var createdBefore time.Time
	if err := s.pg.db.QueryRow(`SELECT created_at FROM mcp_servers WHERE name = $1`, name).Scan(&createdBefore); err != nil {
		t.Fatalf("read created_at: %v", err)
	}

	// Replace env with 2 different keys + drop one entirely.
	// (UI pattern: empty value means "remove", cleanEnv drops it.)
	newEnv := map[string]string{
		"B":       "new-2", // changed
		"D":       "4",     // new
		"dropped": "",      // sentinel — cleanEnv will drop
	}
	if err := s.UpdateEnv(name, newEnv); err != nil {
		t.Fatalf("UpdateEnv: %v", err)
	}

	got, err := s.Get(name)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got == nil {
		t.Fatal("server row disappeared after UpdateEnv — data loss!")
	}
	if len(got.Env) != 2 {
		t.Errorf("env map size = %d, want 2; got=%v", len(got.Env), got.Env)
	}
	if got.Env["B"] != "new-2" {
		t.Errorf("Env[B] = %q, want %q", got.Env["B"], "new-2")
	}
	if got.Env["D"] != "4" {
		t.Errorf("Env[D] = %q, want %q", got.Env["D"], "4")
	}
	if _, stale := got.Env["A"]; stale {
		t.Error("stale key A should have been replaced")
	}
	if _, stale := got.Env["C"]; stale {
		t.Error("stale key C should have been replaced")
	}
	if _, stale := got.Env["dropped"]; stale {
		t.Error("empty-value key 'dropped' should have been removed by cleanEnv")
	}

	// Row must still exist — this is the whole point of the atomic
	// UPDATE replacing the old remove+add data-loss pattern.
	if got := countRow(t, s.pg.db, name); got != 1 {
		t.Errorf("row count after UpdateEnv = %d, want 1", got)
	}

	// created_at must be unchanged, proving it's the same row and not
	// a re-insert in disguise.
	var createdAfter time.Time
	if err := s.pg.db.QueryRow(`SELECT created_at FROM mcp_servers WHERE name = $1`, name).Scan(&createdAfter); err != nil {
		t.Fatalf("read created_at after: %v", err)
	}
	if !createdAfter.Equal(createdBefore) {
		t.Errorf("created_at changed (%s -> %s) — row was re-created, not updated",
			createdBefore, createdAfter)
	}

	// Other fields must be preserved — UPDATE only touches the env column.
	if got.Command != "bin" {
		t.Errorf("Command = %q, want %q (UPDATE should only touch env column)", got.Command, "bin")
	}
	if got.Transport != TransportStdio {
		t.Errorf("Transport = %q, want %q", got.Transport, TransportStdio)
	}
	if !got.Enabled {
		t.Error("Enabled flag should have been preserved")
	}
}

func TestUpdateEnvPostgresUnknownName(t *testing.T) {
	s := setupPostgresStore(t)

	const name = "does-not-exist"

	// Pre-condition: row must not exist.
	if got := countRow(t, s.pg.db, name); got != 0 {
		t.Fatalf("pre: unexpected row count %d for %q", got, name)
	}

	err := s.UpdateEnv(name, map[string]string{"X": "y"})
	if err == nil {
		t.Fatal("expected error for unknown name, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}

	// Post-condition: row still must not exist — no silent create.
	if got := countRow(t, s.pg.db, name); got != 0 {
		t.Errorf("UpdateEnv silently created row for %q (count=%d)", name, got)
	}
}
