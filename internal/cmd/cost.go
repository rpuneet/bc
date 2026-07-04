package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
)

var costCmd = &cobra.Command{
	Use:     "cost",
	Aliases: []string{"co"},
	Short:   "Show cost information",
	Long: `Commands for viewing API cost information.

Shows Claude Code token usage, costs, and budget management.

Examples:
  mycel cost                              # Show cost records (default)
  mycel cost show eng-01                  # Show costs for specific agent
  mycel cost usage                        # Claude Code usage via ccusage
  mycel cost usage --monthly              # Monthly summary
  mycel cost budget show                  # Show budget status

See Also:
  mycel home           TUI dashboard with cost overview
  mycel status         Agent status (includes cost info)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCostShow,
}

var costShowCmd = &cobra.Command{
	Use:   "show [agent]",
	Short: "Show cost records",
	Long: `Show cost records, optionally filtered by agent.

You can specify the agent either as a positional argument or using --agent flag.

Examples:
  mycel cost show
  mycel cost show engineer-01
  mycel cost show --agent engineer-01`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCostShow,
}

var (
	costShowAgentFlag string
	costLimitFlag     int
	costOffsetFlag    int
)

func init() {
	costShowCmd.Flags().IntVarP(&costLimitFlag, "limit", "n", 20, "Number of records to show")
	costShowCmd.Flags().IntVar(&costOffsetFlag, "offset", 0, "Number of records to skip (for pagination)")
	costShowCmd.Flags().StringVar(&costShowAgentFlag, "agent", "", "Filter by agent (alternative to positional argument)")

	// Budget flags (in cost_budget.go)
	initCostBudgetFlags()

	// Usage flags (in cost_usage.go)
	initCostUsageFlags()

	// Analytics subcommands (in cost_analytics.go)
	initCostAnalyticsFlags()

	costCmd.AddCommand(costShowCmd)
	costCmd.AddCommand(costBudgetCmd)
	costCmd.AddCommand(costUsageCmd)
	rootCmd.AddCommand(costCmd)
}

// ccusageRunner runs ccusage and returns raw JSON output. Overridable for testing.
var ccusageRunner = defaultCCUsageRunner

func defaultCCUsageRunner(ctx context.Context) ([]byte, error) {
	npxPath, err := exec.LookPath("npx")
	if err != nil {
		return nil, err
	}
	ccCmd := exec.CommandContext(ctx, npxPath, "ccusage@latest", "--json") //nolint:gosec // args are static
	ccCmd.Stderr = os.Stderr
	return ccCmd.Output()
}

// fetchCCUsageDailyReport calls ccusage and returns the daily report, or nil if unavailable.
func fetchCCUsageDailyReport(ctx context.Context) *ccusageDailyReport {
	output, err := ccusageRunner(ctx)
	if err != nil {
		return nil
	}
	var report ccusageDailyReport
	if unmarshalErr := json.Unmarshal(output, &report); unmarshalErr != nil {
		return nil
	}
	return &report
}

// costShowResponse is the enriched JSON response for 'mycel cost show --json'.
type costShowResponse struct {
	ByAgent            map[string]float64 `json:"by_agent"`
	ByTeam             map[string]float64 `json:"by_team"`
	ByModel            map[string]float64 `json:"by_model"`
	CacheHitRate       *float64           `json:"cache_hit_rate,omitempty"`
	BurnRate           *float64           `json:"burn_rate,omitempty"`
	ProjectedTotal     *float64           `json:"projected_total,omitempty"`
	BillingWindowSpent *float64           `json:"billing_window_spent,omitempty"`
	TotalInputTokens   int64              `json:"total_input_tokens"`
	TotalOutputTokens  int64              `json:"total_output_tokens"`
	TotalCost          float64            `json:"total_cost"`
}

// enrichWithCCUsage merges ccusage daily report data into the cost show response.
func enrichWithCCUsage(resp *costShowResponse, report *ccusageDailyReport) {
	if report == nil {
		return
	}

	totals := report.Totals

	// Override totals from ccusage if internal DB had no data
	if resp.TotalCost == 0 && totals.TotalCost > 0 {
		resp.TotalCost = totals.TotalCost
		resp.TotalInputTokens = totals.InputTokens
		resp.TotalOutputTokens = totals.OutputTokens
	}

	// cache_hit_rate: cacheRead / (cacheRead + cacheCreation)
	cacheTotal := totals.CacheReadTokens + totals.CacheCreationTokens
	if cacheTotal > 0 {
		rate := float64(totals.CacheReadTokens) / float64(cacheTotal)
		resp.CacheHitRate = &rate
	}

	// burn_rate: average daily cost from ccusage daily entries
	if len(report.Daily) > 0 {
		burnRate := totals.TotalCost / float64(len(report.Daily))
		resp.BurnRate = &burnRate

		// projected_total: burn_rate * days_in_current_month
		now := time.Now()
		daysInMonth := float64(time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day())
		projected := burnRate * daysInMonth
		resp.ProjectedTotal = &projected
	}

	// billing_window_spent: total cost from ccusage
	if totals.TotalCost > 0 {
		spent := totals.TotalCost
		resp.BillingWindowSpent = &spent
	}

	// Enrich by_model from ccusage daily modelsUsed (frequency count, no cost attribution)
	if len(resp.ByModel) == 0 {
		modelSeen := make(map[string]bool)
		for _, d := range report.Daily {
			for _, m := range d.ModelsUsed {
				modelSeen[m] = true
			}
		}
		// Add models with zero cost — signals to TUI which models are in use
		for m := range modelSeen {
			resp.ByModel[m] = 0
		}
	}
}

func runCostShow(cmd *cobra.Command, args []string) error {
	// Validate limit parameter
	if cmd.Flags().Changed("limit") && costLimitFlag <= 0 {
		return fmt.Errorf("limit must be a positive number")
	}
	if cmd.Flags().Changed("offset") && costOffsetFlag < 0 {
		return fmt.Errorf("offset must be non-negative")
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	// Determine agent filter: --agent flag takes precedence, then positional arg
	agentID := costShowAgentFlag
	if agentID == "" && len(args) > 0 {
		agentID = args[0]
	}

	// Check for JSON output — build summary from daemon API
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		return runCostShowJSON(cmd, c, agentID)
	}

	// For table output, use the agent or workspace summary
	if agentID != "" {
		return runCostShowAgent(cmd, c, agentID)
	}

	return runCostShowAll(cmd, c)
}

func runCostShowJSON(cmd *cobra.Command, c *client.Client, agentID string) error {
	byAgent := make(map[string]float64)
	byTeam := make(map[string]float64)
	byModel := make(map[string]float64)

	agentSummaries, err := c.Costs.SummaryByAgent(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get agent summaries: %w", err)
	}
	for _, s := range agentSummaries {
		byAgent[s.AgentID] = s.TotalCostUSD
	}

	teamSummaries, err := c.Costs.SummaryByTeam(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get team summaries: %w", err)
	}
	for _, s := range teamSummaries {
		byTeam[s.TeamID] = s.TotalCostUSD
	}

	modelSummaries, err := c.Costs.SummaryByModel(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get model summaries: %w", err)
	}
	for _, s := range modelSummaries {
		byModel[s.Model] = s.TotalCostUSD
	}

	ws, err := c.Costs.WorkspaceSummary(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get cost summary: %w", err)
	}

	response := &costShowResponse{
		ByAgent:           byAgent,
		ByTeam:            byTeam,
		ByModel:           byModel,
		TotalInputTokens:  ws.InputTokens,
		TotalOutputTokens: ws.OutputTokens,
		TotalCost:         ws.TotalCostUSD,
	}

	// Enrich with ccusage data (graceful — nil if unavailable)
	ccReport := fetchCCUsageDailyReport(cmd.Context())
	enrichWithCCUsage(response, ccReport)

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(response)
}

func runCostShowAgent(cmd *cobra.Command, c *client.Client, agentID string) error {
	detail, err := c.Costs.AgentSummary(cmd.Context(), agentID)
	if err != nil {
		return fmt.Errorf("failed to get agent cost detail: %w", err)
	}

	if detail.Summary.RecordCount == 0 {
		fmt.Println("No cost records found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "DATE\tCOST\tINPUT\tOUTPUT\tTOTAL\tRECORDS")

	for _, d := range detail.Daily {
		_, _ = fmt.Fprintf(w, "%s\t$%.4f\t%d\t%d\t%d\t%d\n",
			d.Date,
			d.CostUSD,
			d.InputTokens,
			d.OutputTokens,
			d.TotalTokens,
			d.RecordCount,
		)
	}

	if flushErr := w.Flush(); flushErr != nil {
		return flushErr
	}

	fmt.Printf("\nTotal: $%.4f (%d records)\n", detail.Summary.TotalCostUSD, detail.Summary.RecordCount)
	return nil
}

func runCostShowAll(cmd *cobra.Command, c *client.Client) error {
	summaries, err := c.Costs.SummaryByAgent(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get cost summaries: %w", err)
	}

	if len(summaries) == 0 {
		fmt.Println("No cost records found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "AGENT\tCOST\tINPUT\tOUTPUT\tTOTAL\tRECORDS")

	for _, s := range summaries {
		_, _ = fmt.Fprintf(w, "%s\t$%.4f\t%d\t%d\t%d\t%d\n",
			s.AgentID,
			s.TotalCostUSD,
			s.InputTokens,
			s.OutputTokens,
			s.TotalTokens,
			s.RecordCount,
		)
	}

	return w.Flush()
}
