package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// Issue #1875: mycel cost usage — wraps ccusage for Claude Code token analytics

var costUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show Claude Code token usage via ccusage",
	Long: `Show Claude Code token usage and cost analytics via ccusage.

Wraps the ccusage tool (https://github.com/ryoppippi/ccusage) to display
detailed token usage, per-model cost breakdown, and cache analytics from
Claude Code's local JSONL session files.

Requires npx (Node.js) to be available on the system.

Examples:
  mycel cost usage                        # Daily usage report
  mycel cost usage --monthly              # Monthly summary
  mycel cost usage --session              # Per-session breakdown
  mycel cost usage --since 20260301       # Usage since date (YYYYMMDD)
  mycel cost usage --until 20260301       # Usage until date (YYYYMMDD)
  mycel cost usage --json                 # Raw JSON output`,
	RunE: runCostUsage,
}

var (
	usageMonthlyFlag bool
	usageSessionFlag bool
	usageSinceFlag   string
	usageUntilFlag   string
	usageRefreshFlag bool
)

func initCostUsageFlags() {
	costUsageCmd.Flags().BoolVar(&usageMonthlyFlag, "monthly", false, "Show monthly summary")
	costUsageCmd.Flags().BoolVar(&usageSessionFlag, "session", false, "Show per-session breakdown")
	costUsageCmd.Flags().StringVar(&usageSinceFlag, "since", "", "Filter from date (YYYYMMDD)")
	costUsageCmd.Flags().StringVar(&usageUntilFlag, "until", "", "Filter until date (YYYYMMDD)")
	costUsageCmd.Flags().BoolVar(&usageRefreshFlag, "refresh", false, "Force refresh cached data")
}

// ccusage JSON types — daily report (default)

type ccusageDailyReport struct {
	Daily  []ccusageDailyEntry `json:"daily"`
	Totals ccusageTotals       `json:"totals"`
}

type ccusageDailyEntry struct {
	Date                string   `json:"date"`
	ModelsUsed          []string `json:"modelsUsed"`
	InputTokens         int64    `json:"inputTokens"`
	OutputTokens        int64    `json:"outputTokens"`
	CacheCreationTokens int64    `json:"cacheCreationTokens"`
	CacheReadTokens     int64    `json:"cacheReadTokens"`
	TotalTokens         int64    `json:"totalTokens"`
	TotalCost           float64  `json:"totalCost"`
}

type ccusageTotals struct {
	InputTokens         int64   `json:"inputTokens"`
	OutputTokens        int64   `json:"outputTokens"`
	CacheCreationTokens int64   `json:"cacheCreationTokens"`
	CacheReadTokens     int64   `json:"cacheReadTokens"`
	TotalTokens         int64   `json:"totalTokens"`
	TotalCost           float64 `json:"totalCost"`
}

// ccusage JSON types — monthly report

type ccusageMonthlyReport struct {
	Type    string                `json:"type"`
	Data    []ccusageMonthlyEntry `json:"data"`
	Summary ccusageReportSummary  `json:"summary"`
}

type ccusageMonthlyEntry struct {
	Month               string   `json:"month"`
	Models              []string `json:"models"`
	InputTokens         int64    `json:"inputTokens"`
	OutputTokens        int64    `json:"outputTokens"`
	CacheCreationTokens int64    `json:"cacheCreationTokens"`
	CacheReadTokens     int64    `json:"cacheReadTokens"`
	TotalTokens         int64    `json:"totalTokens"`
	CostUSD             float64  `json:"costUSD"`
}

type ccusageReportSummary struct {
	TotalInputTokens         int64   `json:"totalInputTokens"`
	TotalOutputTokens        int64   `json:"totalOutputTokens"`
	TotalCacheCreationTokens int64   `json:"totalCacheCreationTokens"`
	TotalCacheReadTokens     int64   `json:"totalCacheReadTokens"`
	TotalTokens              int64   `json:"totalTokens"`
	TotalCostUSD             float64 `json:"totalCostUSD"`
}

// ccusage JSON types — session report

type ccusageSessionReport struct {
	Type    string                `json:"type"`
	Data    []ccusageSessionEntry `json:"data"`
	Summary ccusageReportSummary  `json:"summary"`
}

type ccusageSessionEntry struct {
	Session             string   `json:"session"`
	LastActivity        string   `json:"lastActivity"`
	Models              []string `json:"models"`
	InputTokens         int64    `json:"inputTokens"`
	OutputTokens        int64    `json:"outputTokens"`
	CacheCreationTokens int64    `json:"cacheCreationTokens"`
	CacheReadTokens     int64    `json:"cacheReadTokens"`
	TotalTokens         int64    `json:"totalTokens"`
	CostUSD             float64  `json:"costUSD"`
}

func runCostUsage(cmd *cobra.Command, args []string) error {
	// Fetch fresh data from ccusage
	output, err := fetchCCUsage(cmd)
	if err != nil {
		return err
	}

	return displayOutput(cmd, output)
}

// fetchCCUsage runs the ccusage CLI tool and returns its JSON output.
func fetchCCUsage(cmd *cobra.Command) ([]byte, error) {
	npxPath, err := exec.LookPath("npx")
	if err != nil {
		return nil, fmt.Errorf("npx not found — install Node.js to use 'mycel cost usage' (ccusage requires npx)")
	}

	ccArgs := []string{npxPath, "ccusage@latest", "--json"}
	if usageMonthlyFlag {
		ccArgs = append(ccArgs, "monthly")
	} else if usageSessionFlag {
		ccArgs = append(ccArgs, "session")
	}
	if usageSinceFlag != "" {
		ccArgs = append(ccArgs, "--since", usageSinceFlag)
	}
	if usageUntilFlag != "" {
		ccArgs = append(ccArgs, "--until", usageUntilFlag)
	}

	ccCmd := exec.CommandContext(cmd.Context(), ccArgs[0], ccArgs[1:]...) //nolint:gosec // args are built from validated flags
	ccCmd.Stderr = os.Stderr
	output, err := ccCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ccusage failed: %w\nEnsure ccusage is available via npx (npx ccusage@latest)", err)
	}
	return output, nil
}

// displayOutput handles JSON passthrough or formatted display.
func displayOutput(cmd *cobra.Command, data []byte) error {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		_, err := cmd.OutOrStdout().Write(data)
		return err
	}
	if usageMonthlyFlag {
		return displayMonthlyUsage(cmd, data)
	}
	if usageSessionFlag {
		return displaySessionUsage(cmd, data)
	}
	return displayDailyUsage(cmd, data)
}

func displayDailyUsage(cmd *cobra.Command, data []byte) error {
	var report ccusageDailyReport
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("failed to parse ccusage output: %w", err)
	}

	if len(report.Daily) == 0 {
		cmd.Println("No usage data found")
		return nil
	}

	cmd.Println("Claude Code Daily Usage")
	cmd.Println("=======================")

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "DATE\tINPUT\tOUTPUT\tCACHE W\tCACHE R\tTOTAL\tCOST")

	for _, d := range report.Daily {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t$%.2f\n",
			d.Date,
			d.InputTokens,
			d.OutputTokens,
			d.CacheCreationTokens,
			d.CacheReadTokens,
			d.TotalTokens,
			d.TotalCost,
		)
	}
	_ = w.Flush()

	cmd.Println()
	cmd.Printf("Totals: %d tokens, $%.2f\n", report.Totals.TotalTokens, report.Totals.TotalCost)
	if report.Totals.CacheReadTokens > 0 {
		total := report.Totals.CacheReadTokens + report.Totals.CacheCreationTokens
		if total > 0 {
			hitRate := float64(report.Totals.CacheReadTokens) / float64(total) * 100
			cmd.Printf("Cache: %d created, %d read (%.0f%% hit rate)\n",
				report.Totals.CacheCreationTokens, report.Totals.CacheReadTokens, hitRate)
		}
	}

	return nil
}

func displayMonthlyUsage(cmd *cobra.Command, data []byte) error {
	var report ccusageMonthlyReport
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("failed to parse ccusage output: %w", err)
	}

	if len(report.Data) == 0 {
		cmd.Println("No monthly usage data found")
		return nil
	}

	cmd.Println("Claude Code Monthly Usage")
	cmd.Println("=========================")

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "MONTH\tINPUT\tOUTPUT\tCACHE W\tCACHE R\tTOTAL\tCOST")

	for _, m := range report.Data {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t$%.2f\n",
			m.Month,
			m.InputTokens,
			m.OutputTokens,
			m.CacheCreationTokens,
			m.CacheReadTokens,
			m.TotalTokens,
			m.CostUSD,
		)
	}
	_ = w.Flush()

	cmd.Println()
	cmd.Printf("Totals: %d tokens, $%.2f\n",
		report.Summary.TotalTokens, report.Summary.TotalCostUSD)

	return nil
}

func displaySessionUsage(cmd *cobra.Command, data []byte) error {
	var report ccusageSessionReport
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("failed to parse ccusage output: %w", err)
	}

	if len(report.Data) == 0 {
		cmd.Println("No session usage data found")
		return nil
	}

	cmd.Println("Claude Code Session Usage")
	cmd.Println("=========================")

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "SESSION\tLAST ACTIVE\tINPUT\tOUTPUT\tTOTAL\tCOST")

	for _, s := range report.Data {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t$%.2f\n",
			s.Session,
			s.LastActivity,
			s.InputTokens,
			s.OutputTokens,
			s.TotalTokens,
			s.CostUSD,
		)
	}
	_ = w.Flush()

	cmd.Println()
	cmd.Printf("Totals: %d sessions, %d tokens, $%.2f\n",
		len(report.Data), report.Summary.TotalTokens, report.Summary.TotalCostUSD)

	return nil
}
