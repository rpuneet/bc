// Package cost computes cost/usage analytics directly from provider
// session files (source-direct — there is no ledger).
//
// Providers that implement provider.CostReader expose their local
// usage records; Service fans out to every reader, caches the merged
// entries briefly, and aggregates them into the summary shapes the
// API and CLI serve. Budgets are configuration (thresholds stored in
// ~/.mycel/prefs.json) evaluated against the computed totals.
package cost

import "time"

// BudgetPeriod represents the time period for a budget.
type BudgetPeriod string

// Budget periods.
const (
	BudgetPeriodDaily   BudgetPeriod = "daily"
	BudgetPeriodWeekly  BudgetPeriod = "weekly"
	BudgetPeriodMonthly BudgetPeriod = "monthly"
)

// ValidBudgetPeriod reports whether p is a known period.
func ValidBudgetPeriod(p BudgetPeriod) bool {
	switch p {
	case BudgetPeriodDaily, BudgetPeriodWeekly, BudgetPeriodMonthly:
		return true
	}
	return false
}

// BudgetConfig is the stored shape of a budget threshold: pure
// configuration, keyed by scope in the global prefs.
type BudgetConfig struct {
	UpdatedAt time.Time    `json:"updated_at"`
	Period    BudgetPeriod `json:"period"`
	LimitUSD  float64      `json:"limit_usd"`
	AlertAt   float64      `json:"alert_at"`  // Percentage (0.0-1.0) at which to alert
	HardStop  bool         `json:"hard_stop"` // If true, stop when limit reached
}

// Budget is the API shape of a budget: a BudgetConfig plus its scope.
// The ID field is kept for response compatibility with the old ledger
// (budgets are config now, so it is always 0).
type Budget struct {
	UpdatedAt time.Time    `json:"updated_at"`
	Period    BudgetPeriod `json:"period"`
	Scope     string       `json:"scope"` // "workspace", "agent:<id>", "team:<id>"
	ID        int64        `json:"id"`
	LimitUSD  float64      `json:"limit_usd"`
	AlertAt   float64      `json:"alert_at"`
	HardStop  bool         `json:"hard_stop"`
}

// BudgetStatus represents the current computed spend against a budget.
type BudgetStatus struct {
	Budget       *Budget `json:"budget"`
	CurrentSpend float64 `json:"current_spend"`
	Remaining    float64 `json:"remaining"`
	PercentUsed  float64 `json:"percent_used"`
	IsOverBudget bool    `json:"is_over_budget"`
	IsNearLimit  bool    `json:"is_near_limit"` // True if >= AlertAt percentage
}

// BudgetStore persists budget thresholds. The daemon backs it with the
// budgets section of ~/.mycel/prefs.json.
type BudgetStore interface {
	// All returns every stored budget keyed by scope.
	All() (map[string]BudgetConfig, error)
	// Set stores (creates or replaces) the budget for scope.
	Set(scope string, b BudgetConfig) error
	// Delete removes the budget for scope. Deleting a missing scope
	// returns an error.
	Delete(scope string) error
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
