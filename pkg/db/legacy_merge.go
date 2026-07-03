package db

// legacy_merge.go — one-shot recovery of the nested <ws>/.bc/.bc/bc.db.
//
// Issue #3237: OpenWorkspaceDBWithConfig treated the default sqlite.path
// (".bc") as a base directory and resolved it relative to the process
// CWD, producing a second database at <ws>/.bc/.bc/bc.db when bcd ran
// from the workspace root. Live systems ended up with rows split across
// the two files (e.g. notify_* in the nested file, agents/cost tables in
// the canonical one). On open, mergeLegacyNestedDB folds the nested
// file's rows into the canonical database, then renames the nested file
// to bc.db.merged-<unix> so the merge never runs twice.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
)

// mergeLegacyNestedDB merges a legacy nested <ws>/.bc/.bc/bc.db into the
// already-open canonical database dst, then renames the legacy file so
// the merge is one-shot. Returns nil when there is nothing to merge.
func mergeLegacyNestedDB(dst *sql.DB, workspaceRoot string) error {
	legacy := filepath.Join(workspaceRoot, ".bc", ".bc", "bc.db")
	if _, err := os.Stat(legacy); err != nil {
		return nil // no nested legacy db — nothing to do
	}

	ctx := context.Background()
	counts, err := mergeSQLiteFile(ctx, dst, legacy)
	if err != nil {
		return err
	}

	// Rename (never delete) so the merge can't run twice and the original
	// data stays recoverable. WAL/SHM sidecars follow the main file.
	merged := fmt.Sprintf("%s.merged-%d", legacy, time.Now().Unix())
	if err := os.Rename(legacy, merged); err != nil {
		return fmt.Errorf("rename legacy db %s: %w", legacy, err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, statErr := os.Stat(legacy + suffix); statErr == nil {
			_ = os.Rename(legacy+suffix, merged+suffix)
		}
	}

	log.Info("merged legacy nested .bc/.bc/bc.db into canonical bc.db",
		"legacy", legacy, "renamed_to", merged, "rows_merged", counts)
	return nil
}

// mergeSQLiteFile attaches the SQLite database at legacyPath to dst and
// copies every user table's rows into dst. Tables missing from dst are
// created from the legacy schema and copied wholesale; tables present in
// both are merged over the intersection of their columns with INSERT OR
// IGNORE, except notify_subscriptions where the newest row per
// (channel, agent) wins. Returns per-table merged row counts.
func mergeSQLiteFile(ctx context.Context, dst *sql.DB, legacyPath string) (map[string]int64, error) {
	// ATTACH is connection-scoped — pin a single connection for the whole merge.
	conn, err := dst.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("pin connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, attachErr := conn.ExecContext(ctx, `ATTACH DATABASE ? AS legacy`, legacyPath); attachErr != nil {
		return nil, fmt.Errorf("attach %s: %w", legacyPath, attachErr)
	}
	defer func() {
		if _, detachErr := conn.ExecContext(ctx, `DETACH DATABASE legacy`); detachErr != nil {
			log.Warn("detach legacy db failed", "path", legacyPath, "error", detachErr)
		}
	}()

	rows, err := conn.QueryContext(ctx,
		`SELECT name, sql FROM legacy.sqlite_master
		 WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND sql IS NOT NULL
		 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list legacy tables: %w", err)
	}
	type legacyTable struct {
		name      string
		createSQL string
	}
	var tables []legacyTable
	for rows.Next() {
		var t legacyTable
		if scanErr := rows.Scan(&t.name, &t.createSQL); scanErr != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan legacy table: %w", scanErr)
		}
		tables = append(tables, t)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("close legacy table list: %w", closeErr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate legacy tables: %w", err)
	}

	counts := make(map[string]int64, len(tables))
	for _, t := range tables {
		n, mergeErr := mergeTable(ctx, conn, t.name, t.createSQL)
		if mergeErr != nil {
			return counts, fmt.Errorf("merge table %q: %w", t.name, mergeErr)
		}
		if n > 0 {
			counts[t.name] = n
		}
	}
	return counts, nil
}

// mergeTable copies one legacy table's rows into the main database on
// the pinned connection. Returns the number of rows inserted/updated.
func mergeTable(ctx context.Context, conn *sql.Conn, name, createSQL string) (int64, error) {
	var existsInMain int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM main.sqlite_master WHERE type = 'table' AND name = ?`, name,
	).Scan(&existsInMain); err != nil {
		return 0, fmt.Errorf("check main table: %w", err)
	}

	q := quoteIdent(name)

	if existsInMain == 0 {
		// The canonical db never saw this table (the live split-brain case:
		// notify_* only exists in the nested file). Recreate the schema and
		// copy every row verbatim. Indexes are re-created by the owning
		// store's CREATE INDEX IF NOT EXISTS on its next init.
		if _, err := conn.ExecContext(ctx, createSQL); err != nil {
			return 0, fmt.Errorf("create table in main: %w", err)
		}
		res, err := conn.ExecContext(ctx,
			fmt.Sprintf(`INSERT INTO main.%s SELECT * FROM legacy.%s`, q, q))
		if err != nil {
			return 0, fmt.Errorf("copy rows: %w", err)
		}
		return res.RowsAffected()
	}

	mainCols, err := tableColumns(ctx, conn, "main", name)
	if err != nil {
		return 0, err
	}
	legacyCols, err := tableColumns(ctx, conn, "legacy", name)
	if err != nil {
		return 0, err
	}
	common := intersect(legacyCols, mainCols)
	if len(common) == 0 {
		return 0, nil
	}

	if name == "notify_subscriptions" && hasAll(common, "channel", "agent", "mention_only", "created_at") {
		// Newest row per (channel, agent) wins. The id column is skipped so
		// AUTOINCREMENT assigns fresh ids instead of colliding with main's.
		res, upErr := conn.ExecContext(ctx, `
			INSERT INTO main.notify_subscriptions (channel, agent, mention_only, created_at)
			SELECT channel, agent, mention_only, created_at
			FROM legacy.notify_subscriptions
			WHERE true
			ON CONFLICT(channel, agent) DO UPDATE SET
				mention_only = excluded.mention_only,
				created_at   = excluded.created_at
			WHERE excluded.created_at > notify_subscriptions.created_at`)
		if upErr != nil {
			return 0, fmt.Errorf("upsert subscriptions: %w", upErr)
		}
		return res.RowsAffected()
	}

	quoted := make([]string, len(common))
	for i, c := range common {
		quoted[i] = quoteIdent(c)
	}
	colList := strings.Join(quoted, ", ")
	res, err := conn.ExecContext(ctx, fmt.Sprintf(
		`INSERT OR IGNORE INTO main.%s (%s) SELECT %s FROM legacy.%s`,
		q, colList, colList, q))
	if err != nil {
		return 0, fmt.Errorf("insert or ignore: %w", err)
	}
	return res.RowsAffected()
}

// tableColumns returns the ordered column names of schema.table.
func tableColumns(ctx context.Context, conn *sql.Conn, schema, table string) ([]string, error) {
	rows, err := conn.QueryContext(ctx,
		fmt.Sprintf(`PRAGMA %s.table_info(%s)`, schema, quoteIdent(table)))
	if err != nil {
		return nil, fmt.Errorf("table_info %s.%s: %w", schema, table, err)
	}
	defer func() { _ = rows.Close() }()

	var cols []string
	for rows.Next() {
		var (
			cid, notNull, pk int
			name, typ        string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("scan table_info: %w", err)
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

// intersect returns the elements of a that also appear in b, preserving
// a's order.
func intersect(a, b []string) []string {
	set := make(map[string]struct{}, len(b))
	for _, s := range b {
		set[s] = struct{}{}
	}
	out := make([]string, 0, len(a))
	for _, s := range a {
		if _, ok := set[s]; ok {
			out = append(out, s)
		}
	}
	return out
}

// hasAll reports whether cols contains every name in want.
func hasAll(cols []string, want ...string) bool {
	set := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		set[c] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; !ok {
			return false
		}
	}
	return true
}

// quoteIdent double-quotes a SQLite identifier, escaping embedded quotes.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
