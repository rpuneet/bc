// Package cost — global.go adds a cross-workspace cost aggregation store.
//
// The global store lives at ~/.bc/costs.db and holds cost records tagged with
// the workspace name/path that produced them. Individual workspaces continue
// to write their own records to their per-workspace database; records can be
// mirrored into the global store by callers that want cross-workspace reports.
package cost

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bcdb "github.com/rpuneet/bc/pkg/db"
)

// GlobalDBPath returns the filesystem path to the global cost database.
// Default is ~/.bc/costs.db; falls back to "./.bc/costs.db" if the user
// home directory cannot be determined.
func GlobalDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".bc", "costs.db")
	}
	return filepath.Join(home, ".bc", "costs.db")
}

// GlobalStore aggregates cost records across all bc workspaces.
//
// Unlike Store (which is per-workspace), GlobalStore keeps one row per
// cost entry tagged with the originating workspace name and project path.
// Callers typically mirror records into this store as they are recorded
// in the workspace store.
type GlobalStore struct {
	db   *bcdb.DB
	path string
}

// GlobalRecord is a cost entry in the global store.
type GlobalRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Workspace string    `json:"workspace"`
	Project   string    `json:"project"`
	AgentID   string    `json:"agent_id"`
	Model     string    `json:"model"`
	ID        int64     `json:"id"`
	CostUSD   float64   `json:"cost_usd"`
}

// GlobalSummary is a rollup row for SumByWorkspace / SumByProject.
type GlobalSummary struct {
	Key         string  `json:"key"`
	TotalUSD    float64 `json:"total_usd"`
	AgentCount  int64   `json:"agent_count"`
	RecordCount int64   `json:"record_count"`
}

// OpenGlobalStore opens (or creates) the global cost database at ~/.bc/costs.db.
func OpenGlobalStore() (*GlobalStore, error) {
	return OpenGlobalStoreAt(GlobalDBPath())
}

// OpenGlobalStoreAt opens the global cost database at the given path.
// Useful for tests.
func OpenGlobalStoreAt(path string) (*GlobalStore, error) {
	database, err := bcdb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open global cost db: %w", err)
	}
	gs := &GlobalStore{db: database, path: path}
	if err := gs.initSchema(); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("init global cost schema: %w", err)
	}
	return gs, nil
}

// Close releases the underlying database connection.
func (g *GlobalStore) Close() error {
	if g == nil || g.db == nil {
		return nil
	}
	return g.db.Close()
}

// Path returns the database file path.
func (g *GlobalStore) Path() string { return g.path }

func (g *GlobalStore) initSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	schema := `
		CREATE TABLE IF NOT EXISTS global_cost_records (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace  TEXT    NOT NULL,
			project    TEXT    NOT NULL DEFAULT '',
			agent_id   TEXT    NOT NULL,
			model      TEXT    NOT NULL DEFAULT '',
			cost_usd   REAL    NOT NULL DEFAULT 0,
			timestamp  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		);
		CREATE INDEX IF NOT EXISTS idx_global_cost_workspace ON global_cost_records(workspace);
		CREATE INDEX IF NOT EXISTS idx_global_cost_project   ON global_cost_records(project);
		CREATE INDEX IF NOT EXISTS idx_global_cost_timestamp ON global_cost_records(timestamp DESC);
	`
	_, err := g.db.ExecContext(ctx, schema)
	return err
}

// Record inserts a new cost record into the global store.
func (g *GlobalStore) Record(ctx context.Context, workspace, project, agentID, model string, costUSD float64) error {
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	_, err := g.db.ExecContext(ctx,
		`INSERT INTO global_cost_records (workspace, project, agent_id, model, cost_usd)
		 VALUES (?, ?, ?, ?, ?)`,
		workspace, project, agentID, model, costUSD,
	)
	if err != nil {
		return fmt.Errorf("record global cost: %w", err)
	}
	return nil
}

// SumByWorkspace returns total cost per workspace within the [start, end]
// window. A zero start/end disables that bound. Rows are ordered by
// total_usd DESC.
func (g *GlobalStore) SumByWorkspace(ctx context.Context, start, end time.Time) ([]*GlobalSummary, error) {
	return g.sumBy(ctx, "workspace", start, end)
}

// SumByProject returns total cost per project path within [start, end].
func (g *GlobalStore) SumByProject(ctx context.Context, start, end time.Time) ([]*GlobalSummary, error) {
	return g.sumBy(ctx, "project", start, end)
}

func (g *GlobalStore) sumBy(ctx context.Context, column string, start, end time.Time) ([]*GlobalSummary, error) {
	// Validate column name — we construct the SQL directly so this MUST be a
	// fixed identifier, never user input.
	switch column {
	case "workspace", "project":
	default:
		return nil, fmt.Errorf("invalid group column: %q", column)
	}

	where, args := buildTimeWhere(start, end)
	query := fmt.Sprintf(`
		SELECT %s AS key,
		       COALESCE(SUM(cost_usd), 0)          AS total_usd,
		       COUNT(DISTINCT agent_id)            AS agent_count,
		       COUNT(*)                            AS record_count
		FROM global_cost_records
		%s
		GROUP BY %s
		ORDER BY total_usd DESC`, column, where, column)

	rows, err := g.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sum by %s: %w", column, err)
	}
	defer func() { _ = rows.Close() }()

	var out []*GlobalSummary
	for rows.Next() {
		var s GlobalSummary
		if err := rows.Scan(&s.Key, &s.TotalUSD, &s.AgentCount, &s.RecordCount); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// buildTimeWhere returns a `WHERE ...` fragment and corresponding args for the
// provided [start, end] window. Zero times disable the corresponding bound.
func buildTimeWhere(start, end time.Time) (string, []any) {
	var clauses []string
	var args []any
	if !start.IsZero() {
		clauses = append(clauses, "timestamp >= ?")
		args = append(args, start.UTC().Format(time.RFC3339))
	}
	if !end.IsZero() {
		clauses = append(clauses, "timestamp <= ?")
		args = append(args, end.UTC().Format(time.RFC3339))
	}
	if len(clauses) == 0 {
		return "", nil
	}
	where := "WHERE "
	for i, c := range clauses {
		if i > 0 {
			where += " AND "
		}
		where += c
	}
	return where, args
}

// DB exposes the underlying *sql.DB for advanced use (e.g. tests).
func (g *GlobalStore) DB() *sql.DB {
	if g == nil || g.db == nil {
		return nil
	}
	return g.db.DB
}
