package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// costReportCmd rolls up the user-global cost ledger (~/.mycel/costs.db)
// across workspaces. It is a direct-filesystem read — no daemon
// required — so it works even when bcd is not running.
var costReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Report cost totals across workspaces",
	Long: `Report cost totals from the user-global cost ledger (~/.mycel/costs.db).

By default prints per-workspace breakdown. Use --by to change grouping:

  mycel cost report                  # per-workspace totals
  mycel cost report --by workspace   # per-workspace totals
  mycel cost report --by project     # per-project totals (workspace name grouping)
  mycel cost report --since 30d      # only include records from last 30 days`,
	RunE: runCostReport,
}

var (
	costReportBy    string
	costReportSince string
)

func init() {
	costReportCmd.Flags().StringVar(&costReportBy, "by", "workspace", "Grouping: workspace | project")
	costReportCmd.Flags().StringVar(&costReportSince, "since", "", "Include records since (e.g. 7d, 30d, 2026-01-01)")
	costCmd.AddCommand(costReportCmd)
}

func runCostReport(cmd *cobra.Command, _ []string) error {
	path, err := workspace.GlobalCostsDB()
	if err != nil {
		return fmt.Errorf("resolve global costs path: %w", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return fmt.Errorf("no user-global cost ledger at %s — start bcd once to create it", path)
	}

	store, err := cost.OpenGlobalStore(path)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	since, err := parseSinceFlag(costReportSince)
	if err != nil {
		return err
	}

	switch costReportBy {
	case "workspace", "":
		return printCostByWorkspace(cmd.Context(), store, since)
	case "project":
		return printCostByProject(cmd.Context(), store, since)
	default:
		return fmt.Errorf("unknown --by %q (want: workspace, project)", costReportBy)
	}
}

func printCostByWorkspace(ctx context.Context, store *cost.Store, since time.Time) error {
	byRepo, err := store.SumByRepo(ctx, since)
	if err != nil {
		return err
	}

	// Resolve repo paths to names via the registry for friendlier
	// output.
	names := map[string]string{}
	if reg, regErr := workspace.LoadRegistry(); regErr == nil && reg != nil {
		for _, e := range reg.Workspaces {
			names[e.Path] = e.Name
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "REPO\tPATH\tTOTAL")

	keys := make([]string, 0, len(byRepo))
	for k := range byRepo {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return byRepo[keys[i]] > byRepo[keys[j]] })

	var grand float64
	for _, k := range keys {
		total := byRepo[k]
		grand += total
		name := names[k]
		if name == "" {
			if k == "" {
				name = "(unattributed)"
			} else {
				name = "(unknown)"
			}
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t$%.4f\n", name, k, total)
	}
	_, _ = fmt.Fprintf(w, "TOTAL\t\t$%.4f\n", grand)
	return w.Flush()
}

func printCostByProject(ctx context.Context, store *cost.Store, since time.Time) error {
	resolve := func(repo string) string {
		reg, err := workspace.LoadRegistry()
		if err != nil || reg == nil {
			return ""
		}
		for _, e := range reg.Workspaces {
			if e.Path == repo {
				return e.Name
			}
		}
		return ""
	}
	byProj, err := store.SumByProject(ctx, since, resolve)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PROJECT\tTOTAL")
	keys := make([]string, 0, len(byProj))
	for k := range byProj {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return byProj[keys[i]] > byProj[keys[j]] })

	var grand float64
	for _, k := range keys {
		grand += byProj[k]
		_, _ = fmt.Fprintf(w, "%s\t$%.4f\n", k, byProj[k])
	}
	_, _ = fmt.Fprintf(w, "TOTAL\t$%.4f\n", grand)
	return w.Flush()
}

// parseSinceFlag parses a --since argument: empty returns zero (all
// history); "7d" / "30d" relative; otherwise RFC3339 or YYYY-MM-DD.
func parseSinceFlag(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var days int
		_, err := fmt.Sscanf(s, "%dd", &days)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse --since %q: %w", s, err)
		}
		return time.Now().AddDate(0, 0, -days), nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(-d), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unrecognized --since value %q (want: 7d, 24h, 2026-01-01, or RFC3339)", s)
}
