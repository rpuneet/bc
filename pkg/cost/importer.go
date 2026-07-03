package cost

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// Importer scans Claude Code JSONL session files and imports token usage into
// the cost.db store. It tracks which session files have been imported to avoid
// double-counting.
type Importer struct {
	store        *Store
	workspaceDir string
	// workspaceID, when non-empty, is written into the workspace_id
	// column of each cost_records row inserted by this Importer. Used
	// with the user-global cost ledger (M8e) so imports get attributed
	// to the right workspace.
	workspaceID string
}

// NewImporter creates an Importer for the given workspace.
func NewImporter(store *Store, workspaceDir string) *Importer {
	return &Importer{store: store, workspaceDir: workspaceDir}
}

// SetWorkspaceID sets the workspace id tagged onto every record
// imported by this Importer. Pass the empty string to revert to the
// legacy behavior (untagged rows).
func (imp *Importer) SetWorkspaceID(wsID string) {
	imp.workspaceID = wsID
}

// ImportAll scans all known Claude projects directories and imports new sessions.
// It is safe to call repeatedly — already-imported sessions are skipped.
func (imp *Importer) ImportAll(ctx context.Context) (int, error) {
	dirs := imp.claudeProjectsDirs()
	log.Info("cost importer: scanning directories", "count", len(dirs), "workspace", imp.workspaceDir)
	total := 0
	totalFiles := 0
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err != nil {
			continue // not present — skip
		}
		files, err := FindSessionFiles(dir)
		if err != nil {
			log.Warn("cost importer: failed to scan dir", "dir", dir, "error", err)
			continue
		}
		totalFiles += len(files)
		for _, f := range files {
			if err := ctx.Err(); err != nil {
				return total, err
			}
			n, err := imp.importFile(ctx, f)
			if err != nil {
				log.Warn("cost importer: failed to import file", "file", f, "error", err)
				continue
			}
			total += n
		}
	}
	if totalFiles > 0 || total > 0 {
		log.Info("cost importer: scan complete", "files_found", totalFiles, "records_imported", total)
	}
	return total, nil
}

// claudeProjectsDirs returns all directories to scan for JSONL session files.
// Host agents use ~/.claude/projects/, Docker agents use
// .bc/agents/<name>/auth/.claude/projects/.
func (imp *Importer) claudeProjectsDirs() []string {
	var dirs []string

	// Host Claude Code projects directory
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".claude", "projects"))
	}

	// Per-agent Docker auth directories. M11 moved runtime state to
	// ~/.bc/workspaces/<id>/agents/<name>/; scan both the global dir
	// and the legacy sidecar so freshly-migrated and not-yet-migrated
	// workspaces both work.
	var agentsDirs []string
	if globalDir, gErr := workspace.GlobalStateDir(imp.workspaceDir); gErr == nil {
		agentsDirs = append(agentsDirs, filepath.Join(globalDir, "agents"))
	}
	agentsDirs = append(agentsDirs, filepath.Join(imp.workspaceDir, ".bc", "agents"))

	for _, agentsDir := range agentsDirs {
		entries, err := os.ReadDir(agentsDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			// Check both paths: current layout and legacy
			for _, subpath := range []string{
				filepath.Join("claude", "projects"),          // current
				filepath.Join("auth", ".claude", "projects"), // legacy Docker layout
			} {
				p := filepath.Join(agentsDir, e.Name(), subpath)
				if _, err := os.Stat(p); err == nil {
					dirs = append(dirs, p)
					break
				}
			}
		}
	}

	return dirs
}

// importFile imports all new entries from a single JSONL file into the store.
// Returns the number of records inserted. All inserts + watermark update are
// wrapped in a transaction to prevent duplicates on crash recovery.
func (imp *Importer) importFile(ctx context.Context, path string) (int, error) {
	// Determine which entries in this file have already been imported.
	lastImport, err := imp.lastImportedTimestamp(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("query import state: %w", err)
	}

	entries, err := ParseSessionFile(path)
	if err != nil {
		return 0, fmt.Errorf("parse session file %s: %w", path, err)
	}

	// Filter to new entries only
	type importEntry struct {
		agentID string
		entry   SessionEntry
		costUSD float64
	}
	var toInsert []importEntry
	var latest time.Time
	for _, e := range entries {
		if !lastImport.IsZero() && !e.Timestamp.After(lastImport) {
			continue
		}
		if e.Timestamp.After(latest) {
			latest = e.Timestamp
		}
		agentID := imp.resolveAgent(e.CWD, path)
		costUSD := CalcCost(e.Model, e.InputTokens, e.OutputTokens, e.CacheCreationTokens, e.CacheReadTokens)
		toInsert = append(toInsert, importEntry{entry: e, agentID: agentID, costUSD: costUSD})
	}

	if len(toInsert) == 0 {
		return 0, nil
	}

	// Wrap all inserts + watermark update in a transaction
	db := imp.store.DB()
	usePostgres := imp.store.backend != nil

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after commit

	var inserted int
	for _, ie := range toInsert {
		e := ie.entry
		// total_tokens = input + output. Cache tokens are stored in their
		// own columns and reported separately — lumping them in here is
		// what made /api/costs report tens of billions of "tokens".
		total := e.InputTokens + e.OutputTokens

		var res sql.Result
		var insertErr error
		// workspace_id lives as a nullable TEXT column (added by the
		// importer schema migrations below). When no id is set, pass
		// nil so pre-M8e rows stay NULL.
		var wsPtr *string
		if imp.workspaceID != "" {
			wsPtr = &imp.workspaceID
		}
		ts := e.Timestamp.UTC().Format(time.RFC3339Nano)
		// The WHERE NOT EXISTS guard dedups identical usage entries that
		// appear in more than one JSONL file. Claude Code writes compaction
		// sidechain files (<session>/subagents/agent-acompact-*.jsonl) that
		// replicate the parent session's assistant messages verbatim — same
		// sessionId, timestamps and token counts — and the per-file
		// watermark alone can't catch that.
		if usePostgres {
			// Postgres backend has not yet grown the workspace_id column
			// — deferred to a follow-up. Keep the legacy insert so the
			// backend continues to work without schema changes.
			res, insertErr = tx.ExecContext(ctx,
				`INSERT INTO cost_records
				 (agent_id, model, session_id, input_tokens, output_tokens, total_tokens,
				  cache_creation_tokens, cache_read_tokens, cost_usd, timestamp)
				 SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
				 WHERE NOT EXISTS (
				   SELECT 1 FROM cost_records
				   WHERE session_id = $11 AND timestamp = $12 AND model = $13
				     AND input_tokens = $14 AND output_tokens = $15
				     AND cache_creation_tokens = $16 AND cache_read_tokens = $17
				 )`,
				ie.agentID, e.Model, e.SessionID,
				e.InputTokens, e.OutputTokens, total,
				e.CacheCreationTokens, e.CacheReadTokens,
				ie.costUSD, ts,
				e.SessionID, ts, e.Model,
				e.InputTokens, e.OutputTokens,
				e.CacheCreationTokens, e.CacheReadTokens,
			)
		} else {
			res, insertErr = tx.ExecContext(ctx,
				`INSERT INTO cost_records
				 (agent_id, model, session_id, input_tokens, output_tokens, total_tokens,
				  cache_creation_tokens, cache_read_tokens, cost_usd, timestamp, workspace_id)
				 SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
				 WHERE NOT EXISTS (
				   SELECT 1 FROM cost_records
				   WHERE session_id = ? AND timestamp = ? AND model = ?
				     AND input_tokens = ? AND output_tokens = ?
				     AND cache_creation_tokens = ? AND cache_read_tokens = ?
				 )`,
				ie.agentID, e.Model, e.SessionID,
				e.InputTokens, e.OutputTokens, total,
				e.CacheCreationTokens, e.CacheReadTokens,
				ie.costUSD, ts, wsPtr,
				e.SessionID, ts, e.Model,
				e.InputTokens, e.OutputTokens,
				e.CacheCreationTokens, e.CacheReadTokens,
			)
		}
		if insertErr != nil {
			log.Warn("cost importer: failed to insert record", "session", e.SessionID, "error", insertErr)
			continue
		}
		if n, raErr := res.RowsAffected(); raErr == nil && n == 0 {
			continue // duplicate of an already-imported entry — skipped
		}
		inserted++
	}

	// Advance the watermark whenever new entries were parsed — even if they
	// all turned out to be duplicates — so pure-duplicate files (compaction
	// sidechains) aren't re-parsed on every scan.
	if len(toInsert) > 0 {
		var wmErr error
		if usePostgres {
			_, wmErr = tx.ExecContext(ctx,
				`INSERT INTO cost_imports (source_path, watermark, record_count, imported_at)
				 VALUES ($1, $2, $3, NOW())
				 ON CONFLICT(source_path) DO UPDATE SET
				   watermark    = excluded.watermark,
				   record_count = cost_imports.record_count + excluded.record_count,
				   imported_at  = excluded.imported_at`,
				path, latest.UTC().Format(time.RFC3339Nano), inserted,
			)
		} else {
			_, wmErr = tx.ExecContext(ctx,
				`INSERT INTO cost_imports (source_path, watermark, record_count, imported_at)
				 VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
				 ON CONFLICT(source_path) DO UPDATE SET
				   watermark    = excluded.watermark,
				   record_count = cost_imports.record_count + excluded.record_count,
				   imported_at  = excluded.imported_at`,
				path, latest.UTC().Format(time.RFC3339Nano), inserted,
			)
		}
		if wmErr != nil {
			return 0, fmt.Errorf("record import state: %w", wmErr)
		}
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return 0, fmt.Errorf("commit transaction: %w", commitErr)
	}
	return inserted, nil
}

// resolveAgent maps a session's CWD (or the JSONL file path) to a bc agent name.
// Docker agent JSONL files live under .bc/agents/<name>/auth/..., so we can
// extract the name from the path. For host sessions we fall back to the
// workspace name derived from CWD.
func (imp *Importer) resolveAgent(cwd, path string) string {
	// Docker path: .bc/agents/<name>/auth/.claude/projects/...
	agentsDir := filepath.Join(imp.workspaceDir, ".bc", "agents") + string(filepath.Separator)
	if strings.HasPrefix(path, agentsDir) {
		rest := strings.TrimPrefix(path, agentsDir)
		parts := strings.SplitN(rest, string(filepath.Separator), 2)
		if len(parts) > 0 && parts[0] != "" {
			return parts[0]
		}
	}

	// Host session: use the last component of the CWD as a loose agent ID.
	// This won't always match a bc agent name, but provides grouping.
	if cwd != "" {
		return filepath.Base(cwd)
	}
	return "unknown"
}

// initImporterSchema adds the cost_imports and session_id/cache columns if missing.
// Called once from Store.Open via migrate().
func initImporterSchema(db *sql.DB) error {
	ctx := context.Background()

	// Use CURRENT_TIMESTAMP — works on both SQLite and Postgres.
	// (strftime is SQLite-only and breaks on Postgres)
	schema := `
		CREATE TABLE IF NOT EXISTS cost_imports (
			source_path  TEXT NOT NULL,
			watermark    TEXT NOT NULL,
			record_count INTEGER NOT NULL DEFAULT 0,
			imported_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (source_path)
		);
	`
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("cost_imports schema: %w", err)
	}

	// Add optional columns to cost_records (migrations — fail silently if already present).
	migrations := []string{
		`ALTER TABLE cost_records ADD COLUMN session_id TEXT`,
		`ALTER TABLE cost_records ADD COLUMN cache_creation_tokens INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE cost_records ADD COLUMN cache_read_tokens INTEGER NOT NULL DEFAULT 0`,
		// M8e: workspace_id tags each row with the owning workspace so
		// the user-global ledger can roll up cross-workspace spend.
		`ALTER TABLE cost_records ADD COLUMN workspace_id TEXT`,
	}
	for _, m := range migrations {
		_, _ = db.ExecContext(ctx, m) // ignore "duplicate column" errors
	}

	// Backs the WHERE NOT EXISTS dedup guard in importFile.
	// Must run after the session_id migration above.
	_, _ = db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_cost_records_session_time ON cost_records(session_id, timestamp)`)
	return nil
}

func (imp *Importer) lastImportedTimestamp(ctx context.Context, path string) (time.Time, error) {
	db := imp.store.DB()
	placeholder := "?"
	if imp.store.backend != nil {
		placeholder = "$1"
	}
	row := db.QueryRowContext(ctx,
		`SELECT watermark FROM cost_imports WHERE source_path = `+placeholder, path) //nolint:gosec // G202: placeholder is either "?" or "$1", not user input
	var watermark string
	if err := row.Scan(&watermark); err == sql.ErrNoRows {
		return time.Time{}, nil
	} else if err != nil {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, watermark)
	return t, err
}
