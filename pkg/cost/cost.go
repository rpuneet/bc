// Package cost provides cost tracking and reporting for bc agents.
//
// The package supports both SQLite and PostgreSQL backends for persistent
// storage of cost records and budgets. Use OpenStore to automatically select
// the backend (Postgres via DATABASE_URL, falling back to SQLite).
//
// # Basic Usage
//
// Create and open a cost store:
//
//	store := cost.NewStore("/path/to/workspace")
//	if err := store.Open(); err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
//
// Record a cost entry:
//
//	record, err := store.Record("agent-1", "team-alpha", "claude-3-opus",
//	    1000,  // input tokens
//	    500,   // output tokens
//	    0.05,  // cost in USD
//	)
//
// Get cost summaries:
//
//	// By agent
//	summaries, _ := store.SummaryByAgent()
//
//	// By model
//	summaries, _ := store.SummaryByModel()
//
//	// Total workspace cost
//	total, _ := store.WorkspaceSummary()
//
// # Budgets
//
// Set and check budgets:
//
//	// Set monthly budget for workspace
//	store.SetBudget("workspace", cost.BudgetPeriodMonthly, 100.0, 0.8, false)
//
//	// Check budget status
//	status, _ := store.CheckBudget("workspace")
//	if status.IsNearLimit {
//	    log.Warn("approaching budget limit")
//	}
package cost

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	bcdb "github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// BudgetPeriod represents the time period for a budget.
type BudgetPeriod string

const (
	BudgetPeriodDaily   BudgetPeriod = "daily"
	BudgetPeriodWeekly  BudgetPeriod = "weekly"
	BudgetPeriodMonthly BudgetPeriod = "monthly"
)

// Budget represents a cost budget configuration.
type Budget struct {
	UpdatedAt time.Time    `json:"updated_at"`
	Period    BudgetPeriod `json:"period"`
	Scope     string       `json:"scope"` // "workspace", "agent:<id>", "team:<id>"
	ID        int64        `json:"id"`
	LimitUSD  float64      `json:"limit_usd"`
	AlertAt   float64      `json:"alert_at"`  // Percentage (0.0-1.0) at which to alert
	HardStop  bool         `json:"hard_stop"` // If true, stop when limit reached
}

// BudgetStatus represents the current status against a budget.
type BudgetStatus struct {
	Budget       *Budget `json:"budget"`
	CurrentSpend float64 `json:"current_spend"`
	Remaining    float64 `json:"remaining"`
	PercentUsed  float64 `json:"percent_used"`
	IsOverBudget bool    `json:"is_over_budget"`
	IsNearLimit  bool    `json:"is_near_limit"` // True if >= AlertAt percentage
}

// Record represents a cost entry for an API call.
type Record struct {
	Timestamp    time.Time `json:"timestamp"`
	AgentID      string    `json:"agent_id"`
	Model        string    `json:"model"`
	TeamID       string    `json:"team_id,omitempty"`
	ID           int64     `json:"id"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	TotalTokens  int64     `json:"total_tokens"`
	CostUSD      float64   `json:"cost_usd"`
}

// Summary represents aggregated cost data.
//
// TotalTokens is input + output only. Cache tokens are reported separately
// via CacheReadTokens / CacheWriteTokens — they are priced at a fraction of
// input tokens and lumping them into the total makes it meaningless (cache
// reads dominate by 1000x in agentic workloads).
type Summary struct {
	AgentID          string  `json:"agent_id,omitempty"`
	TeamID           string  `json:"team_id,omitempty"`
	Model            string  `json:"model,omitempty"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
	RecordCount      int64   `json:"record_count"`
}

// summaryAggregates is the shared SELECT column list for Summary queries.
// total_tokens is computed as input+output so that legacy rows (whose stored
// total_tokens column included cache tokens) don't inflate the totals.
// Works on both SQLite and Postgres.
const summaryAggregates = `SUM(input_tokens), SUM(output_tokens),
	 SUM(cache_read_tokens), SUM(cache_creation_tokens),
	 SUM(input_tokens) + SUM(output_tokens), SUM(cost_usd), COUNT(*)`

// summaryScanner is satisfied by both *sql.Row and *sql.Rows.
type summaryScanner interface {
	Scan(dest ...any) error
}

// scanSummary scans the seven summaryAggregates columns into sum. lead holds
// destinations for any leading GROUP BY columns (agent_id, team_id, model).
func scanSummary(row summaryScanner, sum *Summary, lead ...any) error {
	var in, out, cacheRead, cacheWrite, total, count sql.NullInt64
	var cost sql.NullFloat64
	dest := make([]any, 0, len(lead)+7)
	dest = append(dest, lead...)
	dest = append(dest, &in, &out, &cacheRead, &cacheWrite, &total, &cost, &count)
	if err := row.Scan(dest...); err != nil {
		return err
	}
	sum.InputTokens = in.Int64
	sum.OutputTokens = out.Int64
	sum.CacheReadTokens = cacheRead.Int64
	sum.CacheWriteTokens = cacheWrite.Int64
	sum.TotalTokens = total.Int64
	sum.TotalCostUSD = cost.Float64
	sum.RecordCount = count.Int64
	return nil
}

// Store provides cost tracking backed by SQLite or Postgres.
// When created via OpenStore, the backend field is set and all operations
// are delegated to it. When created via NewStore/Open (legacy), the store
// uses its embedded SQLite connection directly.
type Store struct {
	db      *bcdb.DB
	backend CostBackend // non-nil when using Postgres via OpenStore
	path    string
}

// NewStore creates a new cost store backed by the ledger at
// ~/.mycel/costs.db.
func NewStore(_ string) *Store {
	home, err := workspace.MycelHome()
	if err != nil {
		// Home dir unresolvable — Open() will fail with a clear error.
		return &Store{}
	}
	return &Store{
		path: filepath.Join(home, "costs.db"),
	}
}

// Open initializes the SQLite database.
func (s *Store) Open() error {
	if s.path == "" {
		return fmt.Errorf("cost store: no database path (cannot resolve mycel home)")
	}
	database, err := bcdb.Open(s.path)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if err := s.initSchema(database.DB); err != nil {
		_ = database.Close()
		return fmt.Errorf("failed to initialize schema: %w", err)
	}
	if err := initImporterSchema(database.DB); err != nil {
		_ = database.Close()
		return fmt.Errorf("failed to initialize importer schema: %w", err)
	}

	s.db = database
	return nil
}

// initSchema creates the database tables.
func (s *Store) initSchema(db *sql.DB) error {

	ctx := context.Background()
	schema := `
		CREATE TABLE IF NOT EXISTS cost_records (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id      TEXT NOT NULL,
			team_id       TEXT,
			model         TEXT NOT NULL,
			input_tokens  INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens  INTEGER NOT NULL DEFAULT 0,
			cost_usd      REAL NOT NULL DEFAULT 0,
			timestamp     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		);
		CREATE INDEX IF NOT EXISTS idx_cost_records_agent ON cost_records(agent_id);
		CREATE INDEX IF NOT EXISTS idx_cost_records_team ON cost_records(team_id);
		CREATE INDEX IF NOT EXISTS idx_cost_records_model ON cost_records(model);
		CREATE INDEX IF NOT EXISTS idx_cost_records_timestamp ON cost_records(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_cost_records_agent_time ON cost_records(agent_id, timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_cost_records_team_time ON cost_records(team_id, timestamp DESC);

		CREATE TABLE IF NOT EXISTS cost_budgets (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			scope      TEXT NOT NULL UNIQUE,
			period     TEXT NOT NULL DEFAULT 'monthly' CHECK (period IN ('daily', 'weekly', 'monthly')),
			limit_usd  REAL NOT NULL DEFAULT 0,
			alert_at   REAL NOT NULL DEFAULT 0.8,
			hard_stop  INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		);
		CREATE INDEX IF NOT EXISTS idx_cost_budgets_scope ON cost_budgets(scope);
	`

	if _, err := db.ExecContext(ctx, schema); err != nil {
		return err
	}
	return nil
}

// OpenStore opens the cost store on the given workspace database. The
// handle is borrowed: callers (typically the per-workspace db registry)
// own its lifecycle. When d is nil (workspace DB unavailable), the store
// falls back to a dedicated costs.db under workspacePath.
func OpenStore(d *bcdb.DB, driver string, workspacePath string) (*Store, error) {
	if d != nil && driver == "timescale" {
		pg := NewPostgresStore(d.DB)
		if schemaErr := pg.InitSchema(); schemaErr != nil {
			return nil, fmt.Errorf("cost store: init timescale schema: %w", schemaErr)
		}
		log.Debug("cost store: using TimescaleDB backend")
		return &Store{backend: pg}, nil
	}

	// SQLite via workspace DB — fall back to dedicated costs.db if unavailable.
	if d == nil {
		log.Info("cost store: workspace DB unavailable, falling back to dedicated costs.db")
		s := NewStore(workspacePath)
		if err := s.Open(); err != nil {
			return nil, fmt.Errorf("cost store: open dedicated costs.db: %w", err)
		}
		return s, nil
	}
	s := &Store{db: d}
	if err := s.initSchema(d.DB); err != nil {
		return nil, fmt.Errorf("cost store: init sqlite schema: %w", err)
	}
	if err := initImporterSchema(d.DB); err != nil {
		return nil, fmt.Errorf("cost store: init importer schema: %w", err)
	}
	return s, nil
}

// Close is a no-op — the workspace DB is owned by the caller.
// The Postgres backend also uses the workspace connection.
func (s *Store) Close() error {
	return nil
}

// DB returns the underlying database connection.
func (s *Store) DB() *sql.DB {
	if s.backend != nil {
		return s.backend.DB()
	}
	return s.db.DB
}

// Record adds a new cost record.
func (s *Store) Record(ctx context.Context, agentID, teamID, model string, inputTokens, outputTokens int64, costUSD float64) (*Record, error) {
	if s.backend != nil {
		return s.backend.Record(ctx, agentID, teamID, model, inputTokens, outputTokens, costUSD)
	}

	totalTokens := inputTokens + outputTokens

	var teamPtr *string
	if teamID != "" {
		teamPtr = &teamID
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO cost_records (agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		agentID, teamPtr, model, inputTokens, outputTokens, totalTokens, costUSD,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to record cost: %w", err)
	}

	id, _ := result.LastInsertId()
	return s.GetByID(ctx, id)
}

// GetByID returns a cost record by ID.
func (s *Store) GetByID(ctx context.Context, id int64) (*Record, error) {
	if s.backend != nil {
		return s.backend.GetByID(ctx, id)
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT id, agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, timestamp
		 FROM cost_records WHERE id = ?`,
		id,
	)
	return s.scanRecord(row)
}

func (s *Store) scanRecord(row *sql.Row) (*Record, error) {
	var r Record
	var timestamp string
	var teamID sql.NullString

	err := row.Scan(&r.ID, &r.AgentID, &teamID, &r.Model, &r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.CostUSD, &timestamp)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan record: %w", err)
	}

	r.TeamID = teamID.String
	var parseErr error
	r.Timestamp, parseErr = time.Parse(time.RFC3339, timestamp)
	if parseErr != nil {
		log.Warn("invalid timestamp in cost record", "id", r.ID, "raw", timestamp, "error", parseErr)
	}
	return &r, nil
}

// GetByAgent returns all cost records for an agent.
func (s *Store) GetByAgent(ctx context.Context, agentID string, limit int) ([]*Record, error) {
	if s.backend != nil {
		return s.backend.GetByAgent(ctx, agentID, limit)
	}
	return s.GetByAgentWithOffset(ctx, agentID, limit, 0)
}

// GetByAgentWithOffset returns cost records for an agent with pagination support.
func (s *Store) GetByAgentWithOffset(ctx context.Context, agentID string, limit, offset int) ([]*Record, error) {
	if s.backend != nil {
		return s.backend.GetByAgentWithOffset(ctx, agentID, limit, offset)
	}

	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, timestamp
		 FROM cost_records WHERE agent_id = ? ORDER BY timestamp DESC LIMIT ? OFFSET ?`,
		agentID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get records by agent: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return s.scanRecords(rows)
}

// GetByTeam returns all cost records for a team.
func (s *Store) GetByTeam(ctx context.Context, teamID string, limit int) ([]*Record, error) {
	if s.backend != nil {
		return s.backend.GetByTeam(ctx, teamID, limit)
	}

	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, timestamp
		 FROM cost_records WHERE team_id = ? ORDER BY timestamp DESC LIMIT ?`,
		teamID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get records by team: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return s.scanRecords(rows)
}

// GetAll returns all cost records.
func (s *Store) GetAll(ctx context.Context, limit int) ([]*Record, error) {
	if s.backend != nil {
		return s.backend.GetAll(ctx, limit)
	}
	return s.GetAllWithOffset(ctx, limit, 0)
}

// GetAllWithOffset returns cost records with pagination support.
func (s *Store) GetAllWithOffset(ctx context.Context, limit, offset int) ([]*Record, error) {
	if s.backend != nil {
		return s.backend.GetAllWithOffset(ctx, limit, offset)
	}

	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, timestamp
		 FROM cost_records ORDER BY timestamp DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get all records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return s.scanRecords(rows)
}

func (s *Store) scanRecords(rows *sql.Rows) ([]*Record, error) {
	var records []*Record
	for rows.Next() {
		var r Record
		var timestamp string
		var teamID sql.NullString

		if err := rows.Scan(&r.ID, &r.AgentID, &teamID, &r.Model, &r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.CostUSD, &timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan record: %w", err)
		}

		r.TeamID = teamID.String
		var parseErr error
		r.Timestamp, parseErr = time.Parse(time.RFC3339, timestamp)
		if parseErr != nil {
			log.Warn("invalid timestamp in cost record", "id", r.ID, "raw", timestamp, "error", parseErr)
		}
		records = append(records, &r)
	}
	return records, rows.Err()
}

// SummaryByAgent returns aggregated costs per agent.
func (s *Store) SummaryByAgent(ctx context.Context) ([]*Summary, error) {
	if s.backend != nil {
		return s.backend.SummaryByAgent(ctx)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_id, `+summaryAggregates+`
		 FROM cost_records GROUP BY agent_id ORDER BY SUM(cost_usd) DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get summary by agent: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []*Summary
	for rows.Next() {
		var sum Summary
		if err := scanSummary(rows, &sum, &sum.AgentID); err != nil {
			return nil, fmt.Errorf("failed to scan summary: %w", err)
		}
		summaries = append(summaries, &sum)
	}
	return summaries, rows.Err()
}

// SummaryByTeam returns aggregated costs per team.
func (s *Store) SummaryByTeam(ctx context.Context) ([]*Summary, error) {
	if s.backend != nil {
		return s.backend.SummaryByTeam(ctx)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT team_id, `+summaryAggregates+`
		 FROM cost_records WHERE team_id IS NOT NULL GROUP BY team_id ORDER BY SUM(cost_usd) DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get summary by team: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []*Summary
	for rows.Next() {
		var sum Summary
		var teamID sql.NullString
		if err := scanSummary(rows, &sum, &teamID); err != nil {
			return nil, fmt.Errorf("failed to scan summary: %w", err)
		}
		sum.TeamID = teamID.String
		summaries = append(summaries, &sum)
	}
	return summaries, rows.Err()
}

// SummaryByModel returns aggregated costs per model.
func (s *Store) SummaryByModel(ctx context.Context) ([]*Summary, error) {
	if s.backend != nil {
		return s.backend.SummaryByModel(ctx)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT model, `+summaryAggregates+`
		 FROM cost_records GROUP BY model ORDER BY SUM(cost_usd) DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get summary by model: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []*Summary
	for rows.Next() {
		var sum Summary
		if err := scanSummary(rows, &sum, &sum.Model); err != nil {
			return nil, fmt.Errorf("failed to scan summary: %w", err)
		}
		summaries = append(summaries, &sum)
	}
	return summaries, rows.Err()
}

// WorkspaceSummary returns the total cost summary for the entire workspace.
func (s *Store) WorkspaceSummary(ctx context.Context) (*Summary, error) {
	if s.backend != nil {
		return s.backend.WorkspaceSummary(ctx)
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT `+summaryAggregates+`
		 FROM cost_records`,
	)

	var sum Summary
	if err := scanSummary(row, &sum); err != nil {
		return nil, fmt.Errorf("failed to scan workspace summary: %w", err)
	}
	return &sum, nil
}

// AgentSummary returns the cost summary for a specific agent.
func (s *Store) AgentSummary(ctx context.Context, agentID string) (*Summary, error) {
	if s.backend != nil {
		return s.backend.AgentSummary(ctx, agentID)
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT `+summaryAggregates+`
		 FROM cost_records WHERE agent_id = ?`,
		agentID,
	)

	var sum Summary
	if err := scanSummary(row, &sum); err != nil {
		return nil, fmt.Errorf("failed to scan agent summary: %w", err)
	}
	sum.AgentID = agentID
	return &sum, nil
}

// TeamSummary returns the cost summary for a specific team.
func (s *Store) TeamSummary(ctx context.Context, teamID string) (*Summary, error) {
	if s.backend != nil {
		return s.backend.TeamSummary(ctx, teamID)
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT `+summaryAggregates+`
		 FROM cost_records WHERE team_id = ?`,
		teamID,
	)

	var sum Summary
	if err := scanSummary(row, &sum); err != nil {
		return nil, fmt.Errorf("failed to scan team summary: %w", err)
	}
	sum.TeamID = teamID
	return &sum, nil
}

// SetBudget creates or updates a budget for the given scope.
func (s *Store) SetBudget(ctx context.Context, scope string, period BudgetPeriod, limitUSD, alertAt float64, hardStop bool) (*Budget, error) {
	if s.backend != nil {
		return s.backend.SetBudget(ctx, scope, period, limitUSD, alertAt, hardStop)
	}

	hardStopInt := 0
	if hardStop {
		hardStopInt = 1
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cost_budgets (scope, period, limit_usd, alert_at, hard_stop, updated_at)
		 VALUES (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		 ON CONFLICT(scope) DO UPDATE SET
		   period = excluded.period,
		   limit_usd = excluded.limit_usd,
		   alert_at = excluded.alert_at,
		   hard_stop = excluded.hard_stop,
		   updated_at = excluded.updated_at`,
		scope, period, limitUSD, alertAt, hardStopInt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set budget: %w", err)
	}

	return s.GetBudget(ctx, scope)
}

// GetBudget returns the budget for a given scope.
func (s *Store) GetBudget(ctx context.Context, scope string) (*Budget, error) {
	if s.backend != nil {
		return s.backend.GetBudget(ctx, scope)
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT id, scope, period, limit_usd, alert_at, hard_stop, updated_at
		 FROM cost_budgets WHERE scope = ?`,
		scope,
	)

	var b Budget
	var hardStop int
	var updatedAt string

	err := row.Scan(&b.ID, &b.Scope, &b.Period, &b.LimitUSD, &b.AlertAt, &hardStop, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get budget: %w", err)
	}

	b.HardStop = hardStop == 1
	var parseErr error
	b.UpdatedAt, parseErr = time.Parse(time.RFC3339, updatedAt)
	if parseErr != nil {
		log.Warn("invalid timestamp in budget", "scope", b.Scope, "raw", updatedAt, "error", parseErr)
	}
	return &b, nil
}

// GetAllBudgets returns all configured budgets.
func (s *Store) GetAllBudgets(ctx context.Context) ([]*Budget, error) {
	if s.backend != nil {
		return s.backend.GetAllBudgets(ctx)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, scope, period, limit_usd, alert_at, hard_stop, updated_at
		 FROM cost_budgets ORDER BY scope`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get budgets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var budgets []*Budget
	for rows.Next() {
		var b Budget
		var hardStop int
		var updatedAt string

		if err := rows.Scan(&b.ID, &b.Scope, &b.Period, &b.LimitUSD, &b.AlertAt, &hardStop, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan budget: %w", err)
		}

		b.HardStop = hardStop == 1
		var parseErr error
		b.UpdatedAt, parseErr = time.Parse(time.RFC3339, updatedAt)
		if parseErr != nil {
			log.Warn("invalid timestamp in budget", "scope", b.Scope, "raw", updatedAt, "error", parseErr)
		}
		budgets = append(budgets, &b)
	}
	return budgets, rows.Err()
}

// DeleteBudget removes a budget for the given scope.
func (s *Store) DeleteBudget(ctx context.Context, scope string) error {
	if s.backend != nil {
		return s.backend.DeleteBudget(ctx, scope)
	}

	result, err := s.db.ExecContext(ctx, "DELETE FROM cost_budgets WHERE scope = ?", scope)
	if err != nil {
		return fmt.Errorf("failed to delete budget: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("budget not found for scope %q", scope)
	}
	return nil
}

// CheckBudget returns the current status against a budget.
func (s *Store) CheckBudget(ctx context.Context, scope string) (*BudgetStatus, error) {
	if s.backend != nil {
		return s.backend.CheckBudget(ctx, scope)
	}

	budget, err := s.GetBudget(ctx, scope)
	if err != nil {
		return nil, err
	}
	if budget == nil {
		return nil, nil
	}

	// Calculate period start time
	now := time.Now().UTC()
	var periodStart time.Time
	switch budget.Period {
	case BudgetPeriodDaily:
		periodStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case BudgetPeriodWeekly:
		// Start of week (Sunday)
		daysFromSunday := int(now.Weekday())
		periodStart = time.Date(now.Year(), now.Month(), now.Day()-daysFromSunday, 0, 0, 0, 0, time.UTC)
	case BudgetPeriodMonthly:
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	// Get spend for the period
	var currentSpend float64

	query := `SELECT COALESCE(SUM(cost_usd), 0) FROM cost_records WHERE timestamp >= ?`
	args := []any{periodStart.Format(time.RFC3339)}

	// Add scope filter
	if scope != "workspace" {
		if len(scope) > 6 && scope[:6] == "agent:" {
			query += " AND agent_id = ?"
			args = append(args, scope[6:])
		} else if len(scope) > 5 && scope[:5] == "team:" {
			query += " AND team_id = ?"
			args = append(args, scope[5:])
		}
	}

	row := s.db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(&currentSpend); err != nil {
		return nil, fmt.Errorf("failed to calculate current spend: %w", err)
	}

	status := &BudgetStatus{
		Budget:       budget,
		CurrentSpend: currentSpend,
		Remaining:    budget.LimitUSD - currentSpend,
	}

	if budget.LimitUSD > 0 {
		status.PercentUsed = currentSpend / budget.LimitUSD
		status.IsOverBudget = currentSpend >= budget.LimitUSD
		status.IsNearLimit = status.PercentUsed >= budget.AlertAt
	}

	if status.Remaining < 0 {
		status.Remaining = 0
	}

	return status, nil
}

// Clear removes all cost records.
func (s *Store) Clear(ctx context.Context) error {
	if s.backend != nil {
		return s.backend.Clear(ctx)
	}

	_, err := s.db.ExecContext(ctx, "DELETE FROM cost_records")
	if err != nil {
		return fmt.Errorf("failed to clear cost records: %w", err)
	}
	return nil
}

// DailyCost represents aggregated cost data for a single day.
type DailyCost struct {
	Date         string  `json:"date"`
	CostUSD      float64 `json:"cost_usd"`
	TotalTokens  int64   `json:"total_tokens"`
	RecordCount  int64   `json:"record_count"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}

// AgentDailyCost represents daily cost data for a specific agent.
type AgentDailyCost struct {
	AgentID      string  `json:"agent_id"`
	Date         string  `json:"date"`
	CostUSD      float64 `json:"cost_usd"`
	TotalTokens  int64   `json:"total_tokens"`
	RecordCount  int64   `json:"record_count"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}

// Projection represents a cost projection based on historical data.
type Projection struct {
	Duration        time.Duration `json:"duration"`
	DailyAvgCost    float64       `json:"daily_avg_cost"`
	ProjectedCost   float64       `json:"projected_cost"`
	DaysAnalyzed    int           `json:"days_analyzed"`
	TotalHistorical float64       `json:"total_historical"`
}

// GetDailyCosts returns daily cost totals since the given time.
func (s *Store) GetDailyCosts(ctx context.Context, since time.Time) ([]*DailyCost, error) {
	if s.backend != nil {
		return s.backend.GetDailyCosts(ctx, since)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT
			date(timestamp) as day,
			SUM(cost_usd) as cost,
			SUM(input_tokens) + SUM(output_tokens) as tokens,
			COUNT(*) as records,
			SUM(input_tokens) as input,
			SUM(output_tokens) as output
		 FROM cost_records
		 WHERE timestamp >= ?
		 GROUP BY date(timestamp)
		 ORDER BY day ASC`,
		since.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily costs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var costs []*DailyCost
	for rows.Next() {
		var dc DailyCost
		if err := rows.Scan(&dc.Date, &dc.CostUSD, &dc.TotalTokens, &dc.RecordCount, &dc.InputTokens, &dc.OutputTokens); err != nil {
			return nil, fmt.Errorf("failed to scan daily cost: %w", err)
		}
		costs = append(costs, &dc)
	}
	return costs, rows.Err()
}

// GetAgentDailyCosts returns daily cost totals per agent since the given time.
func (s *Store) GetAgentDailyCosts(ctx context.Context, since time.Time) ([]*AgentDailyCost, error) {
	if s.backend != nil {
		return s.backend.GetAgentDailyCosts(ctx, since)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT
			agent_id,
			date(timestamp) as day,
			SUM(cost_usd) as cost,
			SUM(input_tokens) + SUM(output_tokens) as tokens,
			COUNT(*) as records,
			SUM(input_tokens) as input,
			SUM(output_tokens) as output
		 FROM cost_records
		 WHERE timestamp >= ?
		 GROUP BY agent_id, date(timestamp)
		 ORDER BY agent_id, day ASC`,
		since.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent daily costs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var costs []*AgentDailyCost
	for rows.Next() {
		var adc AgentDailyCost
		if err := rows.Scan(&adc.AgentID, &adc.Date, &adc.CostUSD, &adc.TotalTokens, &adc.RecordCount, &adc.InputTokens, &adc.OutputTokens); err != nil {
			return nil, fmt.Errorf("failed to scan agent daily cost: %w", err)
		}
		costs = append(costs, &adc)
	}
	return costs, rows.Err()
}

// GetSummarySince returns a summary of costs since the given time.
func (s *Store) GetSummarySince(ctx context.Context, since time.Time) (*Summary, error) {
	if s.backend != nil {
		return s.backend.GetSummarySince(ctx, since)
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT `+summaryAggregates+`
		 FROM cost_records
		 WHERE timestamp >= ?`,
		since.Format(time.RFC3339),
	)

	var sum Summary
	if err := scanSummary(row, &sum); err != nil {
		return nil, fmt.Errorf("failed to scan summary: %w", err)
	}
	return &sum, nil
}

// GetAgentSummarySince returns per-agent summaries since the given time.
func (s *Store) GetAgentSummarySince(ctx context.Context, since time.Time) ([]*Summary, error) {
	if s.backend != nil {
		return s.backend.GetAgentSummarySince(ctx, since)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_id, `+summaryAggregates+`
		 FROM cost_records
		 WHERE timestamp >= ?
		 GROUP BY agent_id
		 ORDER BY SUM(cost_usd) DESC`,
		since.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent summary since: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []*Summary
	for rows.Next() {
		var sum Summary
		if err := scanSummary(rows, &sum, &sum.AgentID); err != nil {
			return nil, fmt.Errorf("failed to scan summary: %w", err)
		}
		summaries = append(summaries, &sum)
	}
	return summaries, rows.Err()
}

// ProjectCost calculates a projected cost based on historical daily average.
func (s *Store) ProjectCost(ctx context.Context, lookbackDays int, projectDuration time.Duration) (*Projection, error) {
	if s.backend != nil {
		return s.backend.ProjectCost(ctx, lookbackDays, projectDuration)
	}

	since := time.Now().AddDate(0, 0, -lookbackDays)
	dailyCosts, err := s.GetDailyCosts(ctx, since)
	if err != nil {
		return nil, err
	}

	proj := &Projection{
		Duration:     projectDuration,
		DaysAnalyzed: len(dailyCosts),
	}

	if len(dailyCosts) == 0 {
		return proj, nil
	}

	// Calculate total and daily average
	for _, dc := range dailyCosts {
		proj.TotalHistorical += dc.CostUSD
	}
	proj.DailyAvgCost = proj.TotalHistorical / float64(len(dailyCosts))

	// Project forward
	projectDays := projectDuration.Hours() / 24
	proj.ProjectedCost = proj.DailyAvgCost * projectDays

	return proj, nil
}
