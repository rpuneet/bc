package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// SQLiteStore provides SQLite-backed persistence for agent state.
// All agents live in the single global mycel.db; rows are keyed by the
// globally-unique agent name and carry the agent's repo path so
// per-repo managers can load only their own agents.
type SQLiteStore struct {
	db *db.DB
}

// NewSQLiteStore opens (or creates) the state database at dbPath and
// ensures the agents table exists.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	d, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open state db: %w", err)
	}

	if err := createAgentsTable(d); err != nil {
		_ = d.Close()
		return nil, err
	}

	return &SQLiteStore{db: d}, nil
}

func createAgentsTable(d *db.DB) error {
	ctx := context.Background()
	// Clean schema for the global mycel.db — the file starts empty and
	// the one-time deploy consolidation fills it, so no ALTER-based
	// migrations are carried here. The legacy `workspace` column is
	// kept only because the Agent struct still round-trips it; `repo`
	// (absolute cleaned repo path) is the attribution key.
	schema := `
		CREATE TABLE IF NOT EXISTS agents (
			name          TEXT PRIMARY KEY,
			role          TEXT NOT NULL,
			state         TEXT NOT NULL DEFAULT 'idle',
			tool          TEXT,
			parent_id     TEXT,
			team          TEXT,
			task          TEXT,
			session       TEXT,
			session_id    TEXT,
			workspace     TEXT NOT NULL DEFAULT '',
			repo          TEXT NOT NULL DEFAULT '',
			model         TEXT NOT NULL DEFAULT '',
			worktree_dir  TEXT,
			log_file      TEXT,
			env_file      TEXT,
			env_vars      TEXT NOT NULL DEFAULT '',
			hooked_work   TEXT,
			children      TEXT,
			is_root       INTEGER NOT NULL DEFAULT 0,
			crash_count   INTEGER NOT NULL DEFAULT 0,
			last_crash_time TEXT,
			recovered_from  TEXT,
			runtime_backend TEXT,
			cpus          REAL    NOT NULL DEFAULT 0,
			memory_mb     INTEGER NOT NULL DEFAULT 0,
			ttl           INTEGER NOT NULL DEFAULT 0,
			created_at    TEXT,
			stopped_at    TEXT,
			deleted_at    TEXT,
			started_at    TEXT NOT NULL,
			updated_at    TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_agents_state ON agents(state);
		CREATE INDEX IF NOT EXISTS idx_agents_role ON agents(role);
		CREATE INDEX IF NOT EXISTS idx_agents_parent ON agents(parent_id);
		CREATE INDEX IF NOT EXISTS idx_agents_repo ON agents(repo);
	`
	_, err := d.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("create agents table: %w", err)
	}

	// Pre-resource-limits DBs (created before cpus/memory_mb existed) keep
	// their old columns — CREATE TABLE IF NOT EXISTS is a no-op on an existing
	// table. Add any missing columns idempotently so agent reads don't break
	// on upgrade (otherwise every SELECT referencing cpus/memory_mb errors and
	// the whole fleet vanishes from the API).
	if err := ensureAgentColumns(ctx, d); err != nil {
		return err
	}

	// agent_stats: time-series Docker resource samples.
	statsSchema := `
		CREATE TABLE IF NOT EXISTS agent_stats (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_name    TEXT    NOT NULL,
			collected_at  TEXT    NOT NULL,
			cpu_pct       REAL    NOT NULL DEFAULT 0,
			mem_used_mb   REAL    NOT NULL DEFAULT 0,
			mem_limit_mb  REAL    NOT NULL DEFAULT 0,
			net_rx_mb     REAL    NOT NULL DEFAULT 0,
			net_tx_mb     REAL    NOT NULL DEFAULT 0,
			block_read_mb  REAL   NOT NULL DEFAULT 0,
			block_write_mb REAL   NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_agent_stats_agent ON agent_stats(agent_name);
		CREATE INDEX IF NOT EXISTS idx_agent_stats_time  ON agent_stats(collected_at);
	`
	if _, err := d.ExecContext(ctx, statsSchema); err != nil {
		return fmt.Errorf("create agent_stats table: %w", err)
	}

	return nil
}

// ensureAgentColumns adds columns introduced after the original schema to a
// pre-existing agents table. SQLite has no ADD COLUMN IF NOT EXISTS, so we
// inspect the current columns via PRAGMA and ALTER only the missing ones.
func ensureAgentColumns(ctx context.Context, d *db.DB) error {
	rows, err := d.QueryContext(ctx, `PRAGMA table_info(agents)`)
	if err != nil {
		return fmt.Errorf("inspect agents columns: %w", err)
	}
	have := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan agents column: %w", err)
		}
		have[name] = true
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate agents columns: %w", err)
	}
	for _, add := range []struct{ col, ddl string }{
		{"cpus", "ALTER TABLE agents ADD COLUMN cpus REAL NOT NULL DEFAULT 0"},
		{"memory_mb", "ALTER TABLE agents ADD COLUMN memory_mb INTEGER NOT NULL DEFAULT 0"},
	} {
		if have[add.col] {
			continue
		}
		if _, err := d.ExecContext(ctx, add.ddl); err != nil {
			return fmt.Errorf("add agents.%s column: %w", add.col, err)
		}
	}
	return nil
}

// Save persists a single agent (INSERT OR REPLACE).
func (s *SQLiteStore) Save(ctx context.Context, a *Agent) error {
	children, err := json.Marshal(a.Children)
	if err != nil {
		return fmt.Errorf("marshal children: %w", err)
	}
	envVars, err := marshalEnv(a.Env)
	if err != nil {
		return fmt.Errorf("marshal env: %w", err)
	}

	now := time.Now()
	createdAt := a.CreatedAt
	if createdAt.IsZero() {
		createdAt = a.StartedAt // backward compat: use started_at if created_at not set
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO agents
		(name, role, state, tool, parent_id, team, task, session, workspace,
		 worktree_dir, log_file, env_file, env_vars, hooked_work, children,
		 is_root, crash_count, last_crash_time, recovered_from,
		 runtime_backend, session_id, created_at, stopped_at, deleted_at,
		 started_at, updated_at, repo, model, cpus, memory_mb)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Name, string(a.Role), string(a.State),
		nullStr(a.Tool), nullStr(a.ParentID), nullStr(a.Team), nullStr(a.Task),
		nullStr(a.Session), a.Workspace,
		nullStr(a.WorktreeDir), nullStr(a.LogFile), nullStr(a.EnvFile), envVars,
		nullStr(a.HookedWork), string(children),
		boolToInt(a.IsRoot), a.CrashCount,
		nullTime(a.LastCrashTime), nullStr(a.RecoveredFrom),
		nullStr(a.RuntimeBackend), nullStr(a.SessionID),
		formatTime(createdAt), nullTime(a.StoppedAt), nullTime(a.DeletedAt),
		formatTime(a.StartedAt), formatTime(now), a.Repo, a.Model, a.CPUs, a.MemoryMB,
	)
	return err
}

// Load reads a single agent by name. Returns nil, nil if not found.
func (s *SQLiteStore) Load(ctx context.Context, name string) (*Agent, error) {
	row := s.db.QueryRowContext(ctx, agentSelectCols+` FROM agents WHERE name = ?`, name)

	a, err := scanAgentRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

// LoadRoot reads the root agent (is_root=1). Returns nil, nil if not found.
func (s *SQLiteStore) LoadRoot(ctx context.Context) (*Agent, error) {
	row := s.db.QueryRowContext(ctx, agentSelectCols+` FROM agents WHERE is_root = 1 LIMIT 1`)

	a, err := scanAgentRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

// SoftDelete marks an agent as deleted by setting deleted_at.
// The agent row remains in the database but is excluded from LoadAll.
func (s *SQLiteStore) SoftDelete(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE agents SET deleted_at = ?, updated_at = ? WHERE name = ?",
		formatTime(time.Now()), formatTime(time.Now()), name,
	)
	return err
}

// Delete removes a single agent by name.
func (s *SQLiteStore) Delete(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM agents WHERE name = ?", name)
	return err
}

// LoadAll reads every non-deleted agent into a map keyed by name.
// Agents with a non-null deleted_at are skipped to prevent resurrection after restart.
func (s *SQLiteStore) LoadAll(ctx context.Context) (map[string]*Agent, error) {
	rows, err := s.db.QueryContext(ctx, agentSelectCols+` FROM agents WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	agents := make(map[string]*Agent)
	for rows.Next() {
		a, err := scanAgentRow(rows)
		if err != nil {
			return nil, err
		}
		agents[a.Name] = a
	}
	return agents, rows.Err()
}

// RepoCounts returns the number of non-deleted agents per distinct
// non-empty repo path, across all repos. Backs GET /api/repos.
func (s *SQLiteStore) RepoCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT repo, COUNT(*) FROM agents WHERE deleted_at IS NULL AND repo != '' GROUP BY repo`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[string]int)
	for rows.Next() {
		var repo string
		var n int
		if scanErr := rows.Scan(&repo, &n); scanErr != nil {
			return nil, scanErr
		}
		counts[repo] = n
	}
	return counts, rows.Err()
}

// LoadNames returns the names of every non-deleted agent in the global
// table, across all repos. Used for global-uniqueness checks and
// collision-free name generation.
func (s *SQLiteStore) LoadNames(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM agents WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	names := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names[name] = true
	}
	return names, rows.Err()
}

// SaveAll persists every agent in the map inside a single transaction.
func (s *SQLiteStore) SaveAll(ctx context.Context, agents map[string]*Agent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // rollback after commit is no-op

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO agents
		(name, role, state, tool, parent_id, team, task, session, workspace,
		 worktree_dir, log_file, env_file, env_vars, hooked_work, children,
		 is_root, crash_count, last_crash_time, recovered_from,
		 runtime_backend, session_id, created_at, stopped_at, deleted_at,
		 started_at, updated_at, repo, model, cpus, memory_mb)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now()
	for _, a := range agents {
		children, err := json.Marshal(a.Children)
		if err != nil {
			return fmt.Errorf("marshal children for %s: %w", a.Name, err)
		}
		envVars, err := marshalEnv(a.Env)
		if err != nil {
			return fmt.Errorf("marshal env for %s: %w", a.Name, err)
		}
		createdAt := a.CreatedAt
		if createdAt.IsZero() {
			createdAt = a.StartedAt
		}
		_, err = stmt.ExecContext(ctx,
			a.Name, string(a.Role), string(a.State),
			nullStr(a.Tool), nullStr(a.ParentID), nullStr(a.Team), nullStr(a.Task),
			nullStr(a.Session), a.Workspace,
			nullStr(a.WorktreeDir), nullStr(a.LogFile), nullStr(a.EnvFile), envVars,
			nullStr(a.HookedWork), string(children),
			boolToInt(a.IsRoot), a.CrashCount,
			nullTime(a.LastCrashTime), nullStr(a.RecoveredFrom),
			nullStr(a.RuntimeBackend), nullStr(a.SessionID),
			formatTime(createdAt), nullTime(a.StoppedAt), nullTime(a.DeletedAt),
			formatTime(a.StartedAt), formatTime(now), a.Repo, a.Model, a.CPUs, a.MemoryMB,
		)
		if err != nil {
			return fmt.Errorf("save agent %s: %w", a.Name, err)
		}
	}
	return tx.Commit()
}

// UpdateState updates only the state column for a given agent.
func (s *SQLiteStore) UpdateState(ctx context.Context, name string, state State) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE agents SET state = ?, updated_at = ? WHERE name = ?",
		string(state), formatTime(time.Now()), name,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent %s not found", name)
	}
	return nil
}

// UpdateField updates a single text column for a given agent.
func (s *SQLiteStore) UpdateField(ctx context.Context, name, field, value string) error {
	// Allowlist of updatable columns to prevent SQL injection.
	allowed := map[string]bool{
		"tool": true, "parent_id": true, "team": true, "task": true,
		"session": true, "session_id": true, "worktree_dir": true,
		"log_file": true, "hooked_work": true, "children": true,
		"recovered_from": true, "runtime_backend": true,
	}
	if !allowed[field] {
		return fmt.Errorf("field %q is not updatable", field)
	}

	query := fmt.Sprintf("UPDATE agents SET %s = ?, updated_at = ? WHERE name = ?", field) //nolint:gosec // field validated above
	res, err := s.db.ExecContext(ctx, query, value, formatTime(time.Now()), name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent %s not found", name)
	}
	return nil
}

// Close closes the database.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// --- scan helpers ---

// agentSelectCols is the SELECT column list used by all Load* methods.
const agentSelectCols = `SELECT name, role, state, tool, parent_id, team, task, session, workspace,
	       worktree_dir, log_file, env_file, env_vars, hooked_work, children,
	       is_root, crash_count, last_crash_time, recovered_from,
	       runtime_backend, session_id, created_at, stopped_at, deleted_at,
	       started_at, updated_at, repo, model, cpus, memory_mb`

func scanAgentRow(s interface{ Scan(...any) error }) (*Agent, error) {
	var a Agent
	var role, state string
	var tool, parentID, team, task, session, worktreeDir, logFile, envFile, hookedWork, childrenJSON *string
	var lastCrashTime, recoveredFrom, runtimeBackend, sessionID *string
	var repo, model, envVars string
	var createdAt, stoppedAt, deletedAt *string
	var startedAt, updatedAt string
	var isRoot, crashCount int
	var cpus float64
	var memoryMB int64

	err := s.Scan(
		&a.Name, &role, &state,
		&tool, &parentID, &team, &task, &session, &a.Workspace,
		&worktreeDir, &logFile, &envFile, &envVars, &hookedWork, &childrenJSON,
		&isRoot, &crashCount, &lastCrashTime, &recoveredFrom,
		&runtimeBackend, &sessionID, &createdAt, &stoppedAt, &deletedAt,
		&startedAt, &updatedAt, &repo, &model, &cpus, &memoryMB,
	)
	if err != nil {
		return nil, err
	}
	if envVars != "" {
		_ = json.Unmarshal([]byte(envVars), &a.Env) //nolint:errcheck // best-effort
	}

	a.ID = a.Name
	a.Role = Role(role)
	a.State = State(state)
	a.Tool = deref(tool)
	a.ParentID = deref(parentID)
	a.Team = deref(team)
	a.Task = deref(task)
	a.Session = deref(session)
	a.SessionID = deref(sessionID)
	a.WorktreeDir = deref(worktreeDir)
	a.LogFile = deref(logFile)
	a.EnvFile = deref(envFile)
	a.HookedWork = deref(hookedWork)
	a.IsRoot = isRoot != 0
	a.CrashCount = crashCount
	a.RecoveredFrom = deref(recoveredFrom)
	a.RuntimeBackend = deref(runtimeBackend)
	a.Repo = repo
	a.Model = model
	a.CPUs = cpus
	a.MemoryMB = memoryMB

	if childrenJSON != nil && *childrenJSON != "" {
		_ = json.Unmarshal([]byte(*childrenJSON), &a.Children) //nolint:errcheck // best-effort
	}
	if lastCrashTime != nil && *lastCrashTime != "" {
		if t, err := time.Parse(time.RFC3339, *lastCrashTime); err == nil {
			a.LastCrashTime = &t
		}
	}
	if createdAt != nil && *createdAt != "" {
		if t, err := time.Parse(time.RFC3339, *createdAt); err == nil {
			a.CreatedAt = t
		}
	}
	if stoppedAt != nil && *stoppedAt != "" {
		if t, err := time.Parse(time.RFC3339, *stoppedAt); err == nil {
			a.StoppedAt = &t
		}
	}
	if deletedAt != nil && *deletedAt != "" {
		if t, err := time.Parse(time.RFC3339, *deletedAt); err == nil {
			a.DeletedAt = &t
		}
	}
	a.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	a.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	// Backward compat: if created_at not set, use started_at
	if a.CreatedAt.IsZero() {
		a.CreatedAt = a.StartedAt
	}

	return &a, nil
}

// --- value helpers ---

// marshalEnv JSON-encodes an agent env map for the env_vars column.
// Empty/nil maps store as the empty string (column is NOT NULL DEFAULT ”).
func marshalEnv(env map[string]string) (string, error) {
	if len(env) == 0 {
		return "", nil
	}
	b, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// SaveStats inserts a single AgentStatsRecord into the agent_stats table.
func (s *SQLiteStore) SaveStats(ctx context.Context, rec *AgentStatsRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_stats
		(agent_name, collected_at, cpu_pct, mem_used_mb, mem_limit_mb,
		 net_rx_mb, net_tx_mb, block_read_mb, block_write_mb)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.AgentName, rec.CollectedAt.Format(time.RFC3339),
		rec.CPUPct, rec.MemUsedMB, rec.MemLimitMB,
		rec.NetRxMB, rec.NetTxMB, rec.BlockReadMB, rec.BlockWriteMB,
	)
	return err
}

// QueryStats returns the most recent limit stats rows for an agent, newest first.
func (s *SQLiteStore) QueryStats(ctx context.Context, agentName string, limit int) ([]*AgentStatsRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT agent_name, collected_at, cpu_pct, mem_used_mb, mem_limit_mb,
		       net_rx_mb, net_tx_mb, block_read_mb, block_write_mb
		FROM agent_stats
		WHERE agent_name = ?
		ORDER BY collected_at DESC
		LIMIT ?`, agentName, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var records []*AgentStatsRecord
	for rows.Next() {
		var rec AgentStatsRecord
		var collectedAt string
		if err := rows.Scan(
			&rec.AgentName, &collectedAt, &rec.CPUPct, &rec.MemUsedMB, &rec.MemLimitMB,
			&rec.NetRxMB, &rec.NetTxMB, &rec.BlockReadMB, &rec.BlockWriteMB,
		); err != nil {
			return nil, err
		}
		rec.CollectedAt, _ = time.Parse(time.RFC3339, collectedAt)
		records = append(records, &rec)
	}
	return records, rows.Err()
}
