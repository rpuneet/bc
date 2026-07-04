package cost

import (
	"context"
	"database/sql"
	"fmt"

	bcdb "github.com/rpuneet/mycel/pkg/db"
)

// OpenGlobalStore opens the user-global cost ledger at vaultPath (eg.
// ~/.mycel/costs.db). Records inserted through ScopedTo(repo) carry the
// repo path for cross-repo analytics.
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
	if err := initRepoSchema(d.DB); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("init repo column: %w", err)
	}
	return s, nil
}

// initRepoSchema makes sure the repo column exists on cost_records and
// that an index backs the "roll up by repo" query. The ALTER is
// idempotent-by-swallow-error: SQLite returns "duplicate column name"
// when it already exists. Rows written before the workspace→repo
// re-key keep their legacy workspace_id values and show up as
// unattributed until the one-time deploy consolidation rewrites them.
func initRepoSchema(db *sql.DB) error {
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `ALTER TABLE cost_records ADD COLUMN repo TEXT`)
	_, err := db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_cost_records_repo ON cost_records(repo)`,
	)
	return err
}

// ScopedStore wraps a *Store with a fixed repo path. Every record
// inserted via ScopedStore.Record populates repo so the global ledger
// can attribute spend to the right repo. Reads on a ScopedStore fall
// through unchanged (they use the underlying *Store methods, returning
// cross-repo totals).
type ScopedStore struct {
	store *Store
	repo  string
}

// ScopedTo returns a ScopedStore that tags every write with repo (the
// absolute cleaned repo path). Passing an empty repo is legal — it
// writes NULL, which the SumByRepo query reports as the empty-string
// key.
func (s *Store) ScopedTo(repo string) *ScopedStore {
	return &ScopedStore{store: s, repo: repo}
}

// Repo returns the repo path this ScopedStore tags writes with.
func (sc *ScopedStore) Repo() string { return sc.repo }

// Store returns the underlying *Store for direct access to non-repo
// operations (budgets, summaries, etc.).
func (sc *ScopedStore) Store() *Store { return sc.store }

// Record inserts a cost record tagged with the scoped repo path.
// Delegates to the underlying *Store after the write for parity with
// existing callers.
func (sc *ScopedStore) Record(ctx context.Context, agentID, teamID, model string, inputTokens, outputTokens int64, costUSD float64) (*Record, error) {
	if sc.store == nil {
		return nil, fmt.Errorf("scoped cost store has no backing store")
	}
	if sc.store.backend != nil {
		// The Postgres backend doesn't yet know about repo; fall back to
		// the unscoped Record path so we don't silently drop data.
		// Migrating the PG schema is deferred.
		return sc.store.Record(ctx, agentID, teamID, model, inputTokens, outputTokens, costUSD)
	}

	totalTokens := inputTokens + outputTokens
	var teamPtr *string
	if teamID != "" {
		teamPtr = &teamID
	}
	var repoPtr *string
	if sc.repo != "" {
		repoPtr = &sc.repo
	}
	result, err := sc.store.db.ExecContext(ctx,
		`INSERT INTO cost_records (agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, repo)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		agentID, teamPtr, model, inputTokens, outputTokens, totalTokens, costUSD, repoPtr,
	)
	if err != nil {
		return nil, fmt.Errorf("record scoped cost: %w", err)
	}
	id, _ := result.LastInsertId()
	return sc.store.GetByID(ctx, id)
}

// SumByRepo returns total cost USD aggregated by repo for records since
// the given time. The empty-string key collects rows that pre-date the
// repo re-key (repo NULL).
func (s *Store) SumByRepo(ctx context.Context, since interface{ Format(string) string }) (map[string]float64, error) {
	if s.db == nil {
		return map[string]float64{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(repo, '') AS repo, SUM(cost_usd)
		FROM cost_records
		WHERE timestamp >= ?
		GROUP BY COALESCE(repo, '')
	`, since.Format("2006-01-02T15:04:05Z"))
	if err != nil {
		return nil, fmt.Errorf("sum by repo: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]float64{}
	for rows.Next() {
		var repo string
		var total float64
		if err := rows.Scan(&repo, &total); err != nil {
			return nil, fmt.Errorf("scan sum: %w", err)
		}
		out[repo] = total
	}
	return out, rows.Err()
}

// RepoNameResolver converts a repo path to its human-readable name
// (typically via *workspace.Registry). SumByProject uses it to group
// by the repo's registered name; callers that don't need that grouping
// can pass a resolver that returns the path unchanged.
type RepoNameResolver func(repo string) string

// SumByProject returns total cost USD grouped by a project-level key
// since the given time. Each repo path is mapped through resolve() to
// produce the grouping key — typically the repo's registered name.
// Records without a repo are placed under the "unattributed" key.
func (s *Store) SumByProject(ctx context.Context, since interface{ Format(string) string }, resolve RepoNameResolver) (map[string]float64, error) {
	byRepo, err := s.SumByRepo(ctx, since)
	if err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for repo, total := range byRepo {
		key := "unattributed"
		if repo != "" {
			if resolve != nil {
				if name := resolve(repo); name != "" {
					key = name
				} else {
					key = repo
				}
			} else {
				key = repo
			}
		}
		out[key] += total
	}
	return out, nil
}
