package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/ui"
)

// nowDate returns today's date as YYYY-MM-DD.
func nowDate() string {
	return time.Now().UTC().Format("2006-01-02")
}

// weekStartDate returns the start of the current week (Sunday) as YYYY-MM-DD.
func weekStartDate(today string) string {
	t, err := time.Parse("2006-01-02", today)
	if err != nil {
		return today
	}
	weekday := int(t.Weekday())
	start := t.AddDate(0, 0, -weekday)
	return start.Format("2006-01-02")
}

// monthStartDate returns the first day of the current month as YYYY-MM-DD.
func monthStartDate(today string) string {
	t, err := time.Parse("2006-01-02", today)
	if err != nil {
		return today
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

var costSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show repo cost overview",
	Long: `Show cost summary with today, this week, this month, and all-time totals.

Examples:
  mycel cost summary
  mycel cost summary --json`,
	RunE: runCostSummary,
}

var costAgentCmd = &cobra.Command{
	Use:   "agent [name]",
	Short: "Show per-agent cost breakdown",
	Long: `Show cost breakdown by agent. If a name is given, shows detail for that agent.

Examples:
  mycel cost agent                    # All agents
  mycel cost agent swift-falcon       # Specific agent`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCostAgent,
}

var costModelCmd = &cobra.Command{
	Use:   "model [name]",
	Short: "Show per-model cost breakdown",
	Long: `Show cost breakdown by model.

Examples:
  mycel cost model
  mycel cost model claude-sonnet-4-6`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCostModel,
}

var costDailyCmd = &cobra.Command{
	Use:   "daily",
	Short: "Show daily cost totals",
	Long: `Show daily cost totals for the last N days.

Examples:
  mycel cost daily              # Last 30 days (default)
  mycel cost daily --days 7     # Last 7 days
  mycel cost daily --json`,
	RunE: runCostDaily,
}

var costDashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Show rich cost dashboard",
	Long: `Show a rich formatted cost dashboard with summary, per-agent breakdown,
per-model breakdown, and budget status.

Examples:
  mycel cost dashboard
  mycel cost dashboard --json`,
	RunE: runCostDashboard,
}

var costDailyDaysFlag int

func initCostAnalyticsFlags() {
	costDailyCmd.Flags().IntVar(&costDailyDaysFlag, "days", 30, "Number of days to show")

	costCmd.AddCommand(costSummaryCmd)
	costCmd.AddCommand(costAgentCmd)
	costCmd.AddCommand(costModelCmd)
	costCmd.AddCommand(costDailyCmd)
	costCmd.AddCommand(costDashboardCmd)
}

func runCostSummary(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	// Use daily costs to derive today/week/month summaries
	dailyCosts, err := c.Costs.Daily(cmd.Context(), 365)
	if err != nil {
		return err
	}
	allTime, err := c.Costs.WorkspaceSummary(cmd.Context())
	if err != nil {
		return err
	}

	// Compute today/week/month from daily data
	todayCost, weekCost, monthCost := aggregateDailyCosts(dailyCosts)

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		response := struct {
			TodayCost    float64 `json:"today_cost"`
			WeekCost     float64 `json:"week_cost"`
			MonthCost    float64 `json:"month_cost"`
			AllTimeCost  float64 `json:"all_time_cost"`
			TotalRecords int64   `json:"total_records"`
			TotalTokens  int64   `json:"total_tokens"`
		}{
			TodayCost:    todayCost,
			WeekCost:     weekCost,
			MonthCost:    monthCost,
			AllTimeCost:  allTime.TotalCostUSD,
			TotalRecords: allTime.RecordCount,
			TotalTokens:  allTime.TotalTokens,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	}

	fmt.Println("Cost Summary")
	fmt.Println("============")
	ui.SimpleTable(
		"Today", fmt.Sprintf("$%.4f", todayCost),
		"This Week", fmt.Sprintf("$%.4f", weekCost),
		"This Month", fmt.Sprintf("$%.4f", monthCost),
		"All Time", fmt.Sprintf("$%.4f", allTime.TotalCostUSD),
		"Total Records", fmt.Sprintf("%d", allTime.RecordCount),
		"Total Tokens", fmt.Sprintf("%d", allTime.TotalTokens),
	)
	return nil
}

// aggregateDailyCosts computes today, this week, and this month totals from daily cost data.
func aggregateDailyCosts(dailyCosts []*client.DailyCost) (today, week, month float64) {
	if len(dailyCosts) == 0 {
		return 0, 0, 0
	}

	now := nowDate()
	weekStart := weekStartDate(now)
	monthStart := monthStartDate(now)

	for _, dc := range dailyCosts {
		if dc.Date == now {
			today += dc.CostUSD
		}
		if dc.Date >= weekStart {
			week += dc.CostUSD
		}
		if dc.Date >= monthStart {
			month += dc.CostUSD
		}
	}
	return today, week, month
}

func runCostAgent(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")

	if len(args) > 0 {
		// Show specific agent
		agentID := args[0]
		detail, agentErr := c.Costs.AgentSummary(cmd.Context(), agentID)
		if agentErr != nil {
			return agentErr
		}

		if jsonOutput {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(detail.Summary)
		}

		if detail.Summary.RecordCount == 0 {
			fmt.Printf("No cost records for agent %q\n", agentID)
			return nil
		}

		fmt.Printf("Cost: %s\n", agentID)
		fmt.Println("============")
		ui.SimpleTable(
			"Total Cost", fmt.Sprintf("$%.4f", detail.Summary.TotalCostUSD),
			"Input Tokens", fmt.Sprintf("%d", detail.Summary.InputTokens),
			"Output Tokens", fmt.Sprintf("%d", detail.Summary.OutputTokens),
			"Records", fmt.Sprintf("%d", detail.Summary.RecordCount),
		)
		return nil
	}

	// Show all agents
	summaries, err := c.Costs.SummaryByAgent(cmd.Context())
	if err != nil {
		return err
	}

	if jsonOutput {
		response := struct {
			Agents []*costAgentSummary `json:"agents"`
		}{Agents: toClientAgentSummaries(summaries)}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	}

	if len(summaries) == 0 {
		fmt.Println("No cost records found")
		return nil
	}

	table := ui.NewTable("AGENT", "COST", "INPUT", "OUTPUT", "RECORDS")
	for _, s := range summaries {
		table.AddRow(
			s.AgentID,
			fmt.Sprintf("$%.4f", s.TotalCostUSD),
			fmt.Sprintf("%d", s.InputTokens),
			fmt.Sprintf("%d", s.OutputTokens),
			fmt.Sprintf("%d", s.RecordCount),
		)
	}
	table.Print()
	return nil
}

type costAgentSummary struct {
	AgentID      string  `json:"agent_id"`
	TotalCost    float64 `json:"total_cost"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	RecordCount  int64   `json:"record_count"`
}

func toClientAgentSummaries(summaries []*client.CostSummary) []*costAgentSummary {
	result := make([]*costAgentSummary, len(summaries))
	for i, s := range summaries {
		result[i] = &costAgentSummary{
			AgentID:      s.AgentID,
			TotalCost:    s.TotalCostUSD,
			InputTokens:  s.InputTokens,
			OutputTokens: s.OutputTokens,
			RecordCount:  s.RecordCount,
		}
	}
	return result
}

func runCostModel(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	summaries, err := c.Costs.SummaryByModel(cmd.Context())
	if err != nil {
		return err
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")

	// Filter to specific model if given
	if len(args) > 0 {
		modelName := args[0]
		var filtered []*client.CostSummary
		for _, s := range summaries {
			if s.Model == modelName {
				filtered = append(filtered, s)
			}
		}
		summaries = filtered
	}

	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Models []*client.CostSummary `json:"models"`
		}{Models: summaries})
	}

	if len(summaries) == 0 {
		fmt.Println("No cost records found")
		return nil
	}

	table := ui.NewTable("MODEL", "COST", "INPUT", "OUTPUT", "RECORDS")
	for _, s := range summaries {
		table.AddRow(
			s.Model,
			fmt.Sprintf("$%.4f", s.TotalCostUSD),
			fmt.Sprintf("%d", s.InputTokens),
			fmt.Sprintf("%d", s.OutputTokens),
			fmt.Sprintf("%d", s.RecordCount),
		)
	}
	table.Print()
	return nil
}

func runCostDaily(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	dailyCosts, err := c.Costs.Daily(cmd.Context(), costDailyDaysFlag)
	if err != nil {
		return err
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Daily any `json:"daily"`
			Days  int `json:"days"`
		}{Daily: dailyCosts, Days: costDailyDaysFlag})
	}

	if len(dailyCosts) == 0 {
		fmt.Printf("No cost records in the last %d days\n", costDailyDaysFlag)
		return nil
	}

	table := ui.NewTable("DATE", "COST", "TOKENS", "RECORDS")
	for _, dc := range dailyCosts {
		table.AddRow(
			dc.Date,
			fmt.Sprintf("$%.4f", dc.CostUSD),
			fmt.Sprintf("%d", dc.TotalTokens),
			fmt.Sprintf("%d", dc.RecordCount),
		)
	}
	table.Print()
	return nil
}

func runCostDashboard(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	dailyCosts, err := c.Costs.Daily(cmd.Context(), 365)
	if err != nil {
		return err
	}
	allTime, err := c.Costs.WorkspaceSummary(cmd.Context())
	if err != nil {
		return err
	}
	agentSummaries, err := c.Costs.SummaryByAgent(cmd.Context())
	if err != nil {
		return err
	}
	modelSummaries, err := c.Costs.SummaryByModel(cmd.Context())
	if err != nil {
		return err
	}
	budgets, err := c.Costs.ListBudgets(cmd.Context())
	if err != nil {
		return err
	}

	todayCost, _, monthCost := aggregateDailyCosts(dailyCosts)

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		response := struct {
			ByAgent     []*costAgentSummary   `json:"by_agent"`
			ByModel     []*client.CostSummary `json:"by_model"`
			TodayCost   float64               `json:"today_cost"`
			MonthCost   float64               `json:"month_cost"`
			AllTime     float64               `json:"all_time_cost"`
			BudgetCount int                   `json:"budget_count"`
		}{
			TodayCost:   todayCost,
			MonthCost:   monthCost,
			AllTime:     allTime.TotalCostUSD,
			ByAgent:     toClientAgentSummaries(agentSummaries),
			ByModel:     modelSummaries,
			BudgetCount: len(budgets),
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	}

	// Summary
	fmt.Println("Cost Dashboard")
	fmt.Println("==============")
	ui.SimpleTable(
		"Today", fmt.Sprintf("$%.4f", todayCost),
		"This Month", fmt.Sprintf("$%.4f", monthCost),
		"All Time", fmt.Sprintf("$%.4f", allTime.TotalCostUSD),
	)

	// Per-agent
	if len(agentSummaries) > 0 {
		fmt.Println("\nBy Agent")
		fmt.Println("--------")
		table := ui.NewTable("AGENT", "COST", "TOKENS")
		for _, s := range agentSummaries {
			table.AddRow(s.AgentID, fmt.Sprintf("$%.4f", s.TotalCostUSD), fmt.Sprintf("%d", s.TotalTokens))
		}
		table.Print()
	}

	// Per-model
	if len(modelSummaries) > 0 {
		fmt.Println("\nBy Model")
		fmt.Println("--------")
		table := ui.NewTable("MODEL", "COST", "TOKENS")
		for _, s := range modelSummaries {
			table.AddRow(s.Model, fmt.Sprintf("$%.4f", s.TotalCostUSD), fmt.Sprintf("%d", s.TotalTokens))
		}
		table.Print()
	}

	// Budget status
	if len(budgets) > 0 {
		fmt.Println("\nBudgets")
		fmt.Println("-------")
		for _, b := range budgets {
			status, _ := c.Costs.CheckBudget(cmd.Context(), b.Scope)
			if status != nil {
				printBudgetStatus(b.Scope, status)
			}
		}
	}

	return nil
}
