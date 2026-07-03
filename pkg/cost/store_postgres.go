package cost

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PostgresStore provides Postgres-backed cost tracking.
// It implements CostBackend with the same API as the SQLite Store.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a PostgresStore from an existing *sql.DB connection.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// InitSchema creates the cost tables in Postgres if they don't exist.
func (s *PostgresStore) InitSchema() error {
	ctx := context.Background()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS cost_records (
			id                    BIGSERIAL PRIMARY KEY,
			agent_id              TEXT NOT NULL,
			team_id               TEXT,
			model                 TEXT NOT NULL,
			session_id            TEXT,
			input_tokens          BIGINT NOT NULL DEFAULT 0,
			output_tokens         BIGINT NOT NULL DEFAULT 0,
			total_tokens          BIGINT NOT NULL DEFAULT 0,
			cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
			cache_read_tokens     BIGINT NOT NULL DEFAULT 0,
			cost_usd              DOUBLE PRECISION NOT NULL DEFAULT 0,
			timestamp             TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_records_agent      ON cost_records(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_records_team       ON cost_records(team_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_records_model      ON cost_records(model)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_records_timestamp  ON cost_records(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_records_agent_time ON cost_records(agent_id, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_records_team_time  ON cost_records(team_id, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_records_session_time ON cost_records(session_id, timestamp)`,

		`CREATE TABLE IF NOT EXISTS cost_budgets (
			id         BIGSERIAL PRIMARY KEY,
			scope      TEXT NOT NULL UNIQUE,
			period     TEXT NOT NULL DEFAULT 'monthly' CHECK (period IN ('daily', 'weekly', 'monthly')),
			limit_usd  DOUBLE PRECISION NOT NULL DEFAULT 0,
			alert_at   DOUBLE PRECISION NOT NULL DEFAULT 0.8,
			hard_stop  BOOLEAN NOT NULL DEFAULT FALSE,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_budgets_scope ON cost_budgets(scope)`,

		`CREATE TABLE IF NOT EXISTS cost_imports (
			source_path  TEXT NOT NULL PRIMARY KEY,
			watermark    TEXT NOT NULL,
			record_count BIGINT NOT NULL DEFAULT 0,
			imported_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("postgres cost schema: %w\nSQL: %s", err, truncate(stmt, 60))
		}
	}
	return nil
}

// Close is a no-op — the shared DB is owned by the caller.
func (s *PostgresStore) Close() error {
	return nil
}

// DB returns the underlying database connection.
func (s *PostgresStore) DB() *sql.DB {
	return s.db
}

// --- Record operations ---

// Record adds a new cost record.
func (s *PostgresStore) Record(ctx context.Context, agentID, teamID, model string, inputTokens, outputTokens int64, costUSD float64) (*Record, error) {
	totalTokens := inputTokens + outputTokens

	var teamPtr *string
	if teamID != "" {
		teamPtr = &teamID
	}

	var r Record
	var ts time.Time
	var tid sql.NullString

	err := s.db.QueryRowContext(ctx,
		`INSERT INTO cost_records (agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, timestamp`,
		agentID, teamPtr, model, inputTokens, outputTokens, totalTokens, costUSD,
	).Scan(&r.ID, &r.AgentID, &tid, &r.Model, &r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.CostUSD, &ts)
	if err != nil {
		return nil, fmt.Errorf("failed to record cost: %w", err)
	}

	r.TeamID = tid.String
	r.Timestamp = ts
	return &r, nil
}

// GetByID returns a cost record by ID.
func (s *PostgresStore) GetByID(ctx context.Context, id int64) (*Record, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, timestamp
		 FROM cost_records WHERE id = $1`, id)
	return s.scanRecord(row)
}

func (s *PostgresStore) scanRecord(row *sql.Row) (*Record, error) {
	var r Record
	var ts time.Time
	var teamID sql.NullString

	err := row.Scan(&r.ID, &r.AgentID, &teamID, &r.Model, &r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.CostUSD, &ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan record: %w", err)
	}

	r.TeamID = teamID.String
	r.Timestamp = ts
	return &r, nil
}

// GetByAgent returns cost records for an agent.
func (s *PostgresStore) GetByAgent(ctx context.Context, agentID string, limit int) ([]*Record, error) {
	return s.GetByAgentWithOffset(ctx, agentID, limit, 0)
}

// GetByAgentWithOffset returns cost records for an agent with pagination.
func (s *PostgresStore) GetByAgentWithOffset(ctx context.Context, agentID string, limit, offset int) ([]*Record, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, timestamp
		 FROM cost_records WHERE agent_id = $1 ORDER BY timestamp DESC LIMIT $2 OFFSET $3`,
		agentID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get records by agent: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return s.scanRecords(rows)
}

// GetByTeam returns cost records for a team.
func (s *PostgresStore) GetByTeam(ctx context.Context, teamID string, limit int) ([]*Record, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, timestamp
		 FROM cost_records WHERE team_id = $1 ORDER BY timestamp DESC LIMIT $2`,
		teamID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get records by team: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return s.scanRecords(rows)
}

// GetAll returns all cost records.
func (s *PostgresStore) GetAll(ctx context.Context, limit int) ([]*Record, error) {
	return s.GetAllWithOffset(ctx, limit, 0)
}

// GetAllWithOffset returns cost records with pagination.
func (s *PostgresStore) GetAllWithOffset(ctx context.Context, limit, offset int) ([]*Record, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, team_id, model, input_tokens, output_tokens, total_tokens, cost_usd, timestamp
		 FROM cost_records ORDER BY timestamp DESC LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get all records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return s.scanRecords(rows)
}

func (s *PostgresStore) scanRecords(rows *sql.Rows) ([]*Record, error) {
	var records []*Record
	for rows.Next() {
		var r Record
		var ts time.Time
		var teamID sql.NullString

		if err := rows.Scan(&r.ID, &r.AgentID, &teamID, &r.Model, &r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.CostUSD, &ts); err != nil {
			return nil, fmt.Errorf("failed to scan record: %w", err)
		}

		r.TeamID = teamID.String
		r.Timestamp = ts
		records = append(records, &r)
	}
	return records, rows.Err()
}

// Clear removes all cost records.
func (s *PostgresStore) Clear(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM cost_records")
	if err != nil {
		return fmt.Errorf("failed to clear cost records: %w", err)
	}
	return nil
}

// --- Summary operations ---

// SummaryByAgent returns aggregated costs per agent.
func (s *PostgresStore) SummaryByAgent(ctx context.Context) ([]*Summary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_id, `+summaryAggregates+`
		 FROM cost_records GROUP BY agent_id ORDER BY SUM(cost_usd) DESC`)
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
func (s *PostgresStore) SummaryByTeam(ctx context.Context) ([]*Summary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT team_id, `+summaryAggregates+`
		 FROM cost_records WHERE team_id IS NOT NULL GROUP BY team_id ORDER BY SUM(cost_usd) DESC`)
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
func (s *PostgresStore) SummaryByModel(ctx context.Context) ([]*Summary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT model, `+summaryAggregates+`
		 FROM cost_records GROUP BY model ORDER BY SUM(cost_usd) DESC`)
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

// WorkspaceSummary returns the total cost summary for the workspace.
func (s *PostgresStore) WorkspaceSummary(ctx context.Context) (*Summary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+summaryAggregates+`
		 FROM cost_records`)

	var sum Summary
	if err := scanSummary(row, &sum); err != nil {
		return nil, fmt.Errorf("failed to scan workspace summary: %w", err)
	}
	return &sum, nil
}

// AgentSummary returns the cost summary for a specific agent.
func (s *PostgresStore) AgentSummary(ctx context.Context, agentID string) (*Summary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+summaryAggregates+`
		 FROM cost_records WHERE agent_id = $1`, agentID)

	var sum Summary
	if err := scanSummary(row, &sum); err != nil {
		return nil, fmt.Errorf("failed to scan agent summary: %w", err)
	}
	sum.AgentID = agentID
	return &sum, nil
}

// TeamSummary returns the cost summary for a specific team.
func (s *PostgresStore) TeamSummary(ctx context.Context, teamID string) (*Summary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+summaryAggregates+`
		 FROM cost_records WHERE team_id = $1`, teamID)

	var sum Summary
	if err := scanSummary(row, &sum); err != nil {
		return nil, fmt.Errorf("failed to scan team summary: %w", err)
	}
	sum.TeamID = teamID
	return &sum, nil
}

// GetSummarySince returns a summary of costs since the given time.
func (s *PostgresStore) GetSummarySince(ctx context.Context, since time.Time) (*Summary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+summaryAggregates+`
		 FROM cost_records
		 WHERE timestamp >= $1`, since)

	var sum Summary
	if err := scanSummary(row, &sum); err != nil {
		return nil, fmt.Errorf("failed to scan summary: %w", err)
	}
	return &sum, nil
}

// GetAgentSummarySince returns per-agent summaries since the given time.
func (s *PostgresStore) GetAgentSummarySince(ctx context.Context, since time.Time) ([]*Summary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_id, `+summaryAggregates+`
		 FROM cost_records
		 WHERE timestamp >= $1
		 GROUP BY agent_id
		 ORDER BY SUM(cost_usd) DESC`, since)
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

// --- Budget operations ---

// SetBudget creates or updates a budget for the given scope.
func (s *PostgresStore) SetBudget(ctx context.Context, scope string, period BudgetPeriod, limitUSD, alertAt float64, hardStop bool) (*Budget, error) {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cost_budgets (scope, period, limit_usd, alert_at, hard_stop, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT(scope) DO UPDATE SET
		   period = EXCLUDED.period,
		   limit_usd = EXCLUDED.limit_usd,
		   alert_at = EXCLUDED.alert_at,
		   hard_stop = EXCLUDED.hard_stop,
		   updated_at = EXCLUDED.updated_at`,
		scope, period, limitUSD, alertAt, hardStop)
	if err != nil {
		return nil, fmt.Errorf("failed to set budget: %w", err)
	}
	return s.GetBudget(ctx, scope)
}

// GetBudget returns the budget for a given scope.
func (s *PostgresStore) GetBudget(ctx context.Context, scope string) (*Budget, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, scope, period, limit_usd, alert_at, hard_stop, updated_at
		 FROM cost_budgets WHERE scope = $1`, scope)

	var b Budget
	var updatedAt time.Time

	err := row.Scan(&b.ID, &b.Scope, &b.Period, &b.LimitUSD, &b.AlertAt, &b.HardStop, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get budget: %w", err)
	}

	b.UpdatedAt = updatedAt
	return &b, nil
}

// GetAllBudgets returns all configured budgets.
func (s *PostgresStore) GetAllBudgets(ctx context.Context) ([]*Budget, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, scope, period, limit_usd, alert_at, hard_stop, updated_at
		 FROM cost_budgets ORDER BY scope`)
	if err != nil {
		return nil, fmt.Errorf("failed to get budgets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var budgets []*Budget
	for rows.Next() {
		var b Budget
		var updatedAt time.Time

		if err := rows.Scan(&b.ID, &b.Scope, &b.Period, &b.LimitUSD, &b.AlertAt, &b.HardStop, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan budget: %w", err)
		}
		b.UpdatedAt = updatedAt
		budgets = append(budgets, &b)
	}
	return budgets, rows.Err()
}

// DeleteBudget removes a budget for the given scope.
func (s *PostgresStore) DeleteBudget(ctx context.Context, scope string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM cost_budgets WHERE scope = $1", scope)
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
func (s *PostgresStore) CheckBudget(ctx context.Context, scope string) (*BudgetStatus, error) {
	budget, err := s.GetBudget(ctx, scope)
	if err != nil {
		return nil, err
	}
	if budget == nil {
		return nil, nil
	}

	now := time.Now().UTC()
	var periodStart time.Time
	switch budget.Period {
	case BudgetPeriodDaily:
		periodStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case BudgetPeriodWeekly:
		daysFromSunday := int(now.Weekday())
		periodStart = time.Date(now.Year(), now.Month(), now.Day()-daysFromSunday, 0, 0, 0, 0, time.UTC)
	case BudgetPeriodMonthly:
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	var currentSpend float64
	query := `SELECT COALESCE(SUM(cost_usd), 0) FROM cost_records WHERE timestamp >= $1`
	args := []any{periodStart}

	if scope != "workspace" {
		if len(scope) > 6 && scope[:6] == "agent:" {
			query += " AND agent_id = $2"
			args = append(args, scope[6:])
		} else if len(scope) > 5 && scope[:5] == "team:" {
			query += " AND team_id = $2"
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

// --- Daily cost operations ---

// GetDailyCosts returns daily cost totals since the given time.
func (s *PostgresStore) GetDailyCosts(ctx context.Context, since time.Time) ([]*DailyCost, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT
			DATE(timestamp) as day,
			SUM(cost_usd) as cost,
			SUM(input_tokens) + SUM(output_tokens) as tokens,
			COUNT(*) as records,
			SUM(input_tokens) as input,
			SUM(output_tokens) as output
		 FROM cost_records
		 WHERE timestamp >= $1
		 GROUP BY DATE(timestamp)
		 ORDER BY day ASC`, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily costs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var costs []*DailyCost
	for rows.Next() {
		var dc DailyCost
		var day time.Time
		if err := rows.Scan(&day, &dc.CostUSD, &dc.TotalTokens, &dc.RecordCount, &dc.InputTokens, &dc.OutputTokens); err != nil {
			return nil, fmt.Errorf("failed to scan daily cost: %w", err)
		}
		dc.Date = day.Format("2006-01-02")
		costs = append(costs, &dc)
	}
	return costs, rows.Err()
}

// GetAgentDailyCosts returns daily cost totals per agent since the given time.
func (s *PostgresStore) GetAgentDailyCosts(ctx context.Context, since time.Time) ([]*AgentDailyCost, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT
			agent_id,
			DATE(timestamp) as day,
			SUM(cost_usd) as cost,
			SUM(input_tokens) + SUM(output_tokens) as tokens,
			COUNT(*) as records,
			SUM(input_tokens) as input,
			SUM(output_tokens) as output
		 FROM cost_records
		 WHERE timestamp >= $1
		 GROUP BY agent_id, DATE(timestamp)
		 ORDER BY agent_id, day ASC`, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent daily costs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var costs []*AgentDailyCost
	for rows.Next() {
		var adc AgentDailyCost
		var day time.Time
		if err := rows.Scan(&adc.AgentID, &day, &adc.CostUSD, &adc.TotalTokens, &adc.RecordCount, &adc.InputTokens, &adc.OutputTokens); err != nil {
			return nil, fmt.Errorf("failed to scan agent daily cost: %w", err)
		}
		adc.Date = day.Format("2006-01-02")
		costs = append(costs, &adc)
	}
	return costs, rows.Err()
}

// --- Projection ---

// ProjectCost calculates a projected cost based on historical daily average.
func (s *PostgresStore) ProjectCost(ctx context.Context, lookbackDays int, projectDuration time.Duration) (*Projection, error) {
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

	for _, dc := range dailyCosts {
		proj.TotalHistorical += dc.CostUSD
	}
	proj.DailyAvgCost = proj.TotalHistorical / float64(len(dailyCosts))

	projectDays := projectDuration.Hours() / 24
	proj.ProjectedCost = proj.DailyAvgCost * projectDays

	return proj, nil
}

// truncate returns a string truncated to n characters.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
