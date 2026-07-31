package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/stats"
	"github.com/rpuneet/mycel/pkg/ui"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show repo statistics",
	Long: `Display statistics about the current repo's agents including work
item metrics, agent utilization, and completion rates.

Examples:
  mycel stats             # human-readable summary
  mycel stats --json      # JSON output for scripting
  mycel stats --save      # save stats snapshot to the state dir`,
	RunE: runStats,
}

var (
	statsJSON bool
	statsSave bool
)

func init() {
	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output as JSON")
	statsCmd.Flags().BoolVar(&statsSave, "save", false, "Save stats snapshot to disk")
	rootCmd.AddCommand(statsCmd)
}

func runStats(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	// Try the daemon API first (skip when --save is used, as save requires local access)
	if !statsSave {
		c := getClient()
		summary, apiErr := c.Stats.Summary(ctx)
		if apiErr == nil {
			return runStatsFromAPI(ctx, c, summary)
		}
	}

	// Fallback: direct pkg/stats access
	h, err := getRepo()
	if err != nil {
		return errNoRepo(err)
	}

	s, err := stats.Load(h.StateDir())
	if err != nil {
		return fmt.Errorf("failed to load stats: %w", err)
	}

	if statsSave {
		if err := s.Save(); err != nil {
			return fmt.Errorf("failed to save stats: %w", err)
		}
		fmt.Println("Stats saved to .mycel/stats.json")
	}

	if statsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}

	fmt.Print(s.Summary())

	// Show utilization as a quick indicator
	util := s.Utilization()
	if s.Agents.ActiveAgents > 0 {
		fmt.Printf("\nUtilization: %.0f%% (%d/%d agents working)\n",
			util*100, s.Agents.Working, s.Agents.ActiveAgents)
	}

	return nil
}

func runStatsFromAPI(ctx context.Context, c *client.Client, summary *client.SummaryStats) error {
	if statsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}

	system, sysErr := c.Stats.System(ctx)

	fmt.Println(ui.BoldText("Repo Stats"))
	fmt.Println()
	fmt.Printf("  Agents:   %d total, %d running, %d stopped\n",
		summary.AgentsTotal, summary.AgentsRunning, summary.AgentsStopped)
	fmt.Printf("  Channels: %d (%d messages)\n",
		summary.ChannelsTotal, summary.MessagesTotal)
	fmt.Printf("  Roles:    %d\n", summary.RolesTotal)
	fmt.Printf("  Tools:    %d\n", summary.ToolsTotal)
	if summary.TotalCostUSD > 0 {
		fmt.Printf("  Cost:     $%.2f\n", summary.TotalCostUSD)
	}

	if sysErr == nil && system != nil {
		fmt.Println()
		fmt.Println(ui.BoldText("System"))
		fmt.Printf("  Host:     %s (%s/%s)\n", system.Hostname, system.OS, system.Arch)
		fmt.Printf("  CPUs:     %d\n", system.CPUs)
		fmt.Printf("  Memory:   %.1f%%\n", system.MemoryPercent)
		fmt.Printf("  Disk:     %.1f%%\n", system.DiskPercent)
		fmt.Printf("  Uptime:   %ds\n", system.UptimeSeconds)
	}

	if summary.AgentsRunning > 0 {
		util := float64(summary.AgentsRunning) / float64(summary.AgentsTotal) * 100
		fmt.Printf("\nUtilization: %.0f%% (%d/%d agents running)\n",
			util, summary.AgentsRunning, summary.AgentsTotal)
	}

	return nil
}
