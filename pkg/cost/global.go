package cost

import (
	"context"
	"database/sql"
	"fmt"

	bcdb "github.com/rpuneet/mycel/pkg/db"
)

// OpenGlobalStore opens the user-global cost ledger at vaultPath (eg.
// ~/.mycel/costs.db). Records inserted through ScopedTo(wsID) carry the
// workspace id for cross-workspace analytics.
func OpenGlobalStore(vaultPath string) (*Store, error) {
	d, err := bcdb.Open(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("open global costs: %w", err)
	}
	s := &Store{db: d, path: vaultPath}
	if err := s.initSchema(d.DB); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("init global costs schema: %w", err)
	}
	if err := initImporterSchema(d.DB); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("init importer schema: %w", err)
	}
	if err := initWorkspaceIDSchema(d.DB); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("init workspace_id column: %w", err)
	}
	return s, nil
}

// initWorkspaceIDSchema makes sure the workspace_id column exists on
// cost_records and that an index backs the "roll up by workspace"
// query. The ALTER is idempotent-by-swallow-error: SQLite returns
// "duplicate column name" when it already exists.
func initWorkspaceIDSchema(db *sql.DB) error {
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `ALTER TABLE cost_records ADD COLUMN workspace_id TEXT`)
	_, err := db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_cost_records_workspace ON cost_records(workspace_id)`,
	)
	return err
}

// ScopedStore wraps a *Store with a fixed workspace_id. Every record
// inserted via ScopedStore.Record populates workspace_id so the global
// ledger can attribute spend to the right workspace. Reads on a
// ScopedStore fall through unchanged (they use the underlying *Store
// methods, returning cross-workspace totals).
type ScopedStore struct {
	store       *Store
	workspaceID string
}

// ScopedTo returns a ScopedStore that tags every write with wsID.
// Passing an empty wsID is legal — it writes NULL, which the
// SumByWorkspace query reports as the empty-string key.
func (s *Store) ScopedTo(wsID string) *ScopedStore {
	return &ScopedStore{store: s, workspaceID: wsID}
}

// WorkspaceID returns the workspace id this ScopedStore tags writes with.
func (sc *ScopedStore) WorkspaceID() string { return sc.workspaceID }

// Store returns the underlying *Store for direct access to non-workspace
// operations (budgets, summaries, etc.).
func (sc *ScopedStore) Store() *Store { return sc.store }

// Record inserts a cost record tagged with the scoped workspace id.
// Delegates to the underlying *Store after the write for parity with
// existing callers.
func (sc *ScopedStore) Record(ctx context.Context, agentID, teamID, model string, inputTokens, outputTokens int64, costUSD float64) (*Record, error) {
	if sc.store == nil {
		return nil, fmt.Errorf("scoped cost store has no backing store")
	}
	if sc.store.backend != nil {
		// The Postgres backend doesn't yet know about workspace_id; fall
		// back to the unscoped Record path so we don't silently drop
		// data. Migrating the PG schema is deferred.
		return sc.store.Record(ctx, agentID, teamID, model, inputTokens, outputTokens, costUSD)
	}

	totalTokens := inputTokens + outputTokens
	var teamPtr *string
	if teamID != "" {
		teamPtr = &teamID
	}
	var wsPtr *string
	if sc.workspaceID != "" {
		wsPtr = &sc.workspaceID
	}
	result, err := sc.store.db.ExecContext(ctx,
		`INSERT INTO cost_records (agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, workspace_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		agentID, teamPtr, model, inputTokens, outputTokens, totalTokens, costUSD, wsPtr,
	)
	if err != nil {
		return nil, fmt.Errorf("record scoped cost: %w", err)
	}
	id, _ := result.LastInsertId()
	return sc.store.GetByID(ctx, id)
}

// SumByWorkspace returns total cost USD aggregated by workspace_id for
// records since the given time. The empty-string key collects rows that
// pre-date M8e (workspace_id NULL).
func (s *Store) SumByWorkspace(ctx context.Context, since interface{ Format(string) string }) (map[string]float64, error) {
	if s.db == nil {
		return map[string]float64{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(workspace_id, '') AS ws, SUM(cost_usd)
		FROM cost_records
		WHERE timestamp >= ?
		GROUP BY COALESCE(workspace_id, '')
	`, since.Format("2006-01-02T15:04:05Z"))
	if err != nil {
		return nil, fmt.Errorf("sum by workspace: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]float64{}
	for rows.Next() {
		var ws string
		var total float64
		if err := rows.Scan(&ws, &total); err != nil {
			return nil, fmt.Errorf("scan sum: %w", err)
		}
		out[ws] = total
	}
	return out, rows.Err()
}

// WorkspaceNameResolver converts a workspace id to its human-readable
// name (typically via *workspace.Registry). SumByProject uses it to
// group by the workspace's stored name; callers that don't need that
// grouping can pass a resolver that returns the id unchanged.
type WorkspaceNameResolver func(wsID string) string

// SumByProject returns total cost USD grouped by a project-level key
// since the given time. Each workspace's id is mapped through resolve()
// to produce the grouping key — typically the workspace name. Records
// without a workspace_id are placed under the "unattributed" key.
func (s *Store) SumByProject(ctx context.Context, since interface{ Format(string) string }, resolve WorkspaceNameResolver) (map[string]float64, error) {
	byWS, err := s.SumByWorkspace(ctx, since)
	if err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for wsID, total := range byWS {
		key := "unattributed"
		if wsID != "" {
			if resolve != nil {
				if name := resolve(wsID); name != "" {
					key = name
				} else {
					key = wsID
				}
			} else {
				key = wsID
			}
		}
		out[key] += total
	}
	return out, nil
}
