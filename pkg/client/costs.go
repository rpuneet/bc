package client

import (
	"context"
	"fmt"
	"time"
)

// CostsClient provides cost operations via the daemon.
type CostsClient struct {
	client *Client
}

// CostSummary represents aggregated cost data.
type CostSummary struct {
	AgentID      string  `json:"agent_id,omitempty"`
	TeamID       string  `json:"team_id,omitempty"`
	Model        string  `json:"model,omitempty"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	RecordCount  int64   `json:"record_count"`
	// CostBasis is how TotalCostUSD was produced ("priced" today).
	CostBasis string `json:"cost_basis,omitempty"`
}

// CostBudget represents a cost budget configuration.
type CostBudget struct {
	UpdatedAt time.Time `json:"updated_at"`
	Period    string    `json:"period"`
	Scope     string    `json:"scope"`
	ID        int64     `json:"id"`
	LimitUSD  float64   `json:"limit_usd"`
	AlertAt   float64   `json:"alert_at"`
	HardStop  bool      `json:"hard_stop"`
}

// CostBudgetStatus represents the current status against a budget.
type CostBudgetStatus struct {
	Budget       *CostBudget `json:"budget"`
	CurrentSpend float64     `json:"current_spend"`
	Remaining    float64     `json:"remaining"`
	PercentUsed  float64     `json:"percent_used"`
	IsOverBudget bool        `json:"is_over_budget"`
	IsNearLimit  bool        `json:"is_near_limit"`
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

// CostProjection represents a cost projection based on historical data.
type CostProjection struct {
	Duration        time.Duration `json:"duration"`
	DailyAvgCost    float64       `json:"daily_avg_cost"`
	ProjectedCost   float64       `json:"projected_cost"`
	DaysAnalyzed    int           `json:"days_analyzed"`
	TotalHistorical float64       `json:"total_historical"`
}

// AgentCostDetail is the response from GET /api/costs/agent/{name}.
type AgentCostDetail struct {
	Summary *CostSummary      `json:"summary"`
	Daily   []*AgentDailyCost `json:"daily"`
}

// SetBudgetReq is the request body for POST /api/costs/budgets.
type SetBudgetReq struct {
	Scope    string  `json:"scope"`
	Period   string  `json:"period"`
	LimitUSD float64 `json:"limit_usd"`
	AlertAt  float64 `json:"alert_at"`
	HardStop bool    `json:"hard_stop"`
}

// TotalSummary returns the total cost summary for the workspace.
func (c *CostsClient) TotalSummary(ctx context.Context) (*CostSummary, error) {
	var s CostSummary
	if err := c.client.get(ctx, "/api/costs", &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// SummaryByAgent returns aggregated costs per agent.
func (c *CostsClient) SummaryByAgent(ctx context.Context) ([]*CostSummary, error) {
	var summaries []*CostSummary
	if err := c.client.get(ctx, "/api/costs/agents", &summaries); err != nil {
		return nil, err
	}
	return summaries, nil
}

// SummaryByTeam returns aggregated costs per team.
func (c *CostsClient) SummaryByTeam(ctx context.Context) ([]*CostSummary, error) {
	var summaries []*CostSummary
	if err := c.client.get(ctx, "/api/costs/teams", &summaries); err != nil {
		return nil, err
	}
	return summaries, nil
}

// SummaryByModel returns aggregated costs per model.
func (c *CostsClient) SummaryByModel(ctx context.Context) ([]*CostSummary, error) {
	var summaries []*CostSummary
	if err := c.client.get(ctx, "/api/costs/models", &summaries); err != nil {
		return nil, err
	}
	return summaries, nil
}

// Daily returns daily cost totals for the last N days.
func (c *CostsClient) Daily(ctx context.Context, days int) ([]*DailyCost, error) {
	var costs []*DailyCost
	path := fmt.Sprintf("/api/costs/daily?days=%d", days)
	if err := c.client.get(ctx, path, &costs); err != nil {
		return nil, err
	}
	return costs, nil
}

// ListBudgets returns all configured budgets.
func (c *CostsClient) ListBudgets(ctx context.Context) ([]*CostBudget, error) {
	var budgets []*CostBudget
	if err := c.client.get(ctx, "/api/costs/budgets", &budgets); err != nil {
		return nil, err
	}
	return budgets, nil
}

// CheckBudget returns budget status for the given scope.
func (c *CostsClient) CheckBudget(ctx context.Context, scope string) (*CostBudgetStatus, error) {
	var status CostBudgetStatus
	if err := c.client.get(ctx, "/api/costs/budgets/"+scope, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// SetBudget creates or updates a budget.
func (c *CostsClient) SetBudget(ctx context.Context, req *SetBudgetReq) (*CostBudget, error) {
	var budget CostBudget
	if err := c.client.post(ctx, "/api/costs/budgets", req, &budget); err != nil {
		return nil, err
	}
	return &budget, nil
}

// DeleteBudget removes a budget for the given scope.
func (c *CostsClient) DeleteBudget(ctx context.Context, scope string) error {
	return c.client.delete(ctx, "/api/costs/budgets/"+scope)
}

// ProjectCost returns a cost projection based on historical data.
func (c *CostsClient) ProjectCost(ctx context.Context, lookbackDays, projectDays int) (*CostProjection, error) {
	var proj CostProjection
	path := fmt.Sprintf("/api/costs/project?lookback_days=%d&project_days=%d", lookbackDays, projectDays)
	if err := c.client.get(ctx, path, &proj); err != nil {
		return nil, err
	}
	return &proj, nil
}

// AgentSummary returns the cost summary and daily breakdown for a specific agent.
func (c *CostsClient) AgentSummary(ctx context.Context, name string) (*AgentCostDetail, error) {
	var detail AgentCostDetail
	if err := c.client.get(ctx, "/api/costs/agent/"+name, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// Sync triggers a fresh cost import from JSONL files.
func (c *CostsClient) Sync(ctx context.Context) (int, error) {
	var result struct {
		Imported int `json:"imported"`
	}
	if err := c.client.post(ctx, "/api/costs/sync", nil, &result); err != nil {
		return 0, err
	}
	return result.Imported, nil
}
