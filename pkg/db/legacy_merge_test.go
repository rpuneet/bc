package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// openRaw opens a SQLite db at path via the package Open helper and
// fails the test on error.
func openRaw(t *testing.T, path string) *DB {
	t.Helper()
	d, err := Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	return d
}

func exec(t *testing.T, d *DB, query string, args ...any) {
	t.Helper()
	if _, err := d.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func countRows(t *testing.T, d *DB, table string) int {
	t.Helper()
	var n int
	if err := d.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestOpenWorkspaceDB_DefaultPathIsWorkspaceScoped is the #3237
// regression test: the default sqlite.path (".bc") must resolve to
// <workspaceRoot>/.bc/bc.db regardless of the process CWD — never to a
// CWD-relative ".bc/.bc/bc.db".
func TestOpenWorkspaceDB_DefaultPathIsWorkspaceScoped(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()
	t.Chdir(elsewhere) // reproduce the bug: CWD != workspace root

	cfg := &StorageSettings{Default: "sqlite", SQLite: SQLiteSettings{Path: ".bc"}}
	sqlDB, driver, err := OpenWorkspaceDBWithConfig(root, cfg)
	if err != nil {
		t.Fatalf("OpenWorkspaceDBWithConfig: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	if driver != "sqlite" {
		t.Fatalf("driver = %q, want sqlite", driver)
	}
	if _, statErr := os.Stat(BCDBPath(root)); statErr != nil {
		t.Fatalf("canonical db %s not created: %v", BCDBPath(root), statErr)
	}
	if _, statErr := os.Stat(filepath.Join(elsewhere, ".bc")); statErr == nil {
		t.Fatalf("db leaked into CWD at %s", filepath.Join(elsewhere, ".bc"))
	}
	// The resolved main database file must be exactly BCDBPath(root).
	var name, file string
	if err := sqlDB.QueryRowContext(context.Background(),
		`SELECT name, file FROM pragma_database_list WHERE name = 'main'`).Scan(&name, &file); err != nil {
		t.Fatalf("pragma database_list: %v", err)
	}
	want, _ := filepath.EvalSymlinks(BCDBPath(root))
	got, _ := filepath.EvalSymlinks(file)
	if got != want {
		t.Fatalf("main db file = %s, want %s", got, want)
	}
}

// TestOpenWorkspaceDB_CustomRelativePathResolvesUnderRoot asserts a
// relative custom sqlite.path resolves against the workspace root, not
// the CWD.
func TestOpenWorkspaceDB_CustomRelativePathResolvesUnderRoot(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()
	t.Chdir(elsewhere)

	cfg := &StorageSettings{Default: "sqlite", SQLite: SQLiteSettings{Path: "data"}}
	sqlDB, _, err := OpenWorkspaceDBWithConfig(root, cfg)
	if err != nil {
		t.Fatalf("OpenWorkspaceDBWithConfig: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	want := filepath.Join(root, "data", "bc.db")
	if _, statErr := os.Stat(want); statErr != nil {
		t.Fatalf("custom-path db %s not created: %v", want, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(elsewhere, "data")); statErr == nil {
		t.Fatalf("db leaked into CWD at %s", filepath.Join(elsewhere, "data"))
	}
}

// TestMergeLegacyNestedDB covers the live split-brain: the canonical db
// holds agent/cost tables, the nested .bc/.bc/bc.db holds notify_*
// tables, and each side has a few overlapping rows. Opening the
// workspace db must fold the nested rows into the canonical file,
// apply newest-wins on notify_subscriptions conflicts, and rename the
// nested file exactly once.
func TestMergeLegacyNestedDB(t *testing.T) {
	root := t.TempDir()

	// Seed the canonical db: an agents-ish table plus one subscription
	// (older) that must lose to the legacy row, and one (newer) that must win.
	canon := openRaw(t, BCDBPath(root))
	exec(t, canon, `CREATE TABLE agents (name TEXT PRIMARY KEY, role TEXT)`)
	exec(t, canon, `INSERT INTO agents VALUES ('zen-zebra', 'engineer')`)
	exec(t, canon, `CREATE TABLE notify_subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		channel TEXT NOT NULL, agent TEXT NOT NULL,
		mention_only INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		UNIQUE(channel, agent))`)
	exec(t, canon, `INSERT INTO notify_subscriptions (channel, agent, mention_only, created_at)
		VALUES ('#ops', 'zen-zebra', 0, '2026-01-01T00:00:00Z'),
		       ('#merge', 'zen-zebra', 1, '2026-06-01T00:00:00Z')`)
	if err := canon.Close(); err != nil {
		t.Fatalf("close canonical: %v", err)
	}

	// Seed the nested legacy db: notify tables the canonical file lacks,
	// plus conflicting + disjoint subscriptions.
	legacyPath := filepath.Join(root, ".bc", ".bc", "bc.db")
	legacy := openRaw(t, legacyPath)
	exec(t, legacy, `CREATE TABLE notify_subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		channel TEXT NOT NULL, agent TEXT NOT NULL,
		mention_only INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		UNIQUE(channel, agent))`)
	exec(t, legacy, `INSERT INTO notify_subscriptions (channel, agent, mention_only, created_at)
		VALUES ('#ops', 'zen-zebra', 1, '2026-05-01T00:00:00Z'),
		       ('#merge', 'zen-zebra', 0, '2026-02-01T00:00:00Z'),
		       ('#engineering', 'lucid-meerkat', 0, '2026-03-01T00:00:00Z')`)
	exec(t, legacy, `CREATE TABLE notify_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		channel TEXT NOT NULL, sender TEXT NOT NULL, content TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')))`)
	exec(t, legacy, `INSERT INTO notify_messages (channel, sender, content)
		VALUES ('#ops', 'bcd', 'hello'), ('#merge', 'zen-zebra', 'PR ready')`)
	exec(t, legacy, `CREATE TABLE events (
		id INTEGER PRIMARY KEY AUTOINCREMENT, kind TEXT NOT NULL, payload TEXT)`)
	exec(t, legacy, `INSERT INTO events (kind, payload) VALUES ('agent.start', '{}')`)
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}

	// Open through the public path — this triggers the merge.
	cfg := &StorageSettings{Default: "sqlite", SQLite: SQLiteSettings{Path: ".bc"}}
	sqlDB, _, err := OpenWorkspaceDBWithConfig(root, cfg)
	if err != nil {
		t.Fatalf("OpenWorkspaceDBWithConfig: %v", err)
	}
	merged := &DB{DB: sqlDB}

	// Disjoint canonical data survives.
	if n := countRows(t, merged, "agents"); n != 1 {
		t.Errorf("agents rows = %d, want 1", n)
	}
	// Legacy-only tables copied wholesale.
	if n := countRows(t, merged, "notify_messages"); n != 2 {
		t.Errorf("notify_messages rows = %d, want 2", n)
	}
	if n := countRows(t, merged, "events"); n != 1 {
		t.Errorf("events rows = %d, want 1", n)
	}
	// Subscriptions: 2 canonical + 1 legacy-only = 3 distinct (channel, agent).
	if n := countRows(t, merged, "notify_subscriptions"); n != 3 {
		t.Errorf("notify_subscriptions rows = %d, want 3", n)
	}
	// Newest wins: legacy #ops row (2026-05-01, mention_only=1) beats the
	// canonical 2026-01-01 row; canonical #merge row (2026-06-01,
	// mention_only=1) beats the legacy 2026-02-01 row.
	var mentionOnly int
	var createdAt string
	row := sqlDB.QueryRowContext(context.Background(),
		`SELECT mention_only, created_at FROM notify_subscriptions WHERE channel = '#ops'`)
	if scanErr := row.Scan(&mentionOnly, &createdAt); scanErr != nil {
		t.Fatalf("scan #ops: %v", scanErr)
	}
	if mentionOnly != 1 || createdAt != "2026-05-01T00:00:00Z" {
		t.Errorf("#ops = (mention_only=%d, created_at=%s), want newest legacy row", mentionOnly, createdAt)
	}
	row = sqlDB.QueryRowContext(context.Background(),
		`SELECT mention_only, created_at FROM notify_subscriptions WHERE channel = '#merge'`)
	if scanErr := row.Scan(&mentionOnly, &createdAt); scanErr != nil {
		t.Fatalf("scan #merge: %v", scanErr)
	}
	if mentionOnly != 1 || createdAt != "2026-06-01T00:00:00Z" {
		t.Errorf("#merge = (mention_only=%d, created_at=%s), want newer canonical row", mentionOnly, createdAt)
	}

	// The legacy file was renamed to bc.db.merged-<unix>, not deleted.
	if _, statErr := os.Stat(legacyPath); !os.IsNotExist(statErr) {
		t.Errorf("legacy db still present at %s (stat err: %v)", legacyPath, statErr)
	}
	matches, globErr := filepath.Glob(legacyPath + ".merged-*")
	if globErr != nil || len(matches) != 1 {
		t.Errorf("expected exactly one renamed legacy file, got %v (err %v)", matches, globErr)
	}
	if closeErr := sqlDB.Close(); closeErr != nil {
		t.Fatalf("close merged: %v", closeErr)
	}

	// Idempotency: reopening must not re-merge or duplicate anything.
	sqlDB2, _, err := OpenWorkspaceDBWithConfig(root, cfg)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = sqlDB2.Close() }()
	merged2 := &DB{DB: sqlDB2}
	if n := countRows(t, merged2, "notify_subscriptions"); n != 3 {
		t.Errorf("after reopen notify_subscriptions rows = %d, want 3", n)
	}
	if n := countRows(t, merged2, "notify_messages"); n != 2 {
		t.Errorf("after reopen notify_messages rows = %d, want 2", n)
	}
	matches, _ = filepath.Glob(legacyPath + ".merged-*")
	if len(matches) != 1 {
		t.Errorf("after reopen expected 1 renamed legacy file, got %v", matches)
	}
}
