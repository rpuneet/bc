package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/provider"
)

// costReportCmd rolls up costs across repos, computed directly from
// provider session files (source-direct). It is a filesystem read — no
// daemon required — so it works even when the daemon is not running.
var costReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Report cost totals across repos",
	Long: `Report cost totals computed directly from provider session files.

By default prints per-repo breakdown. Use --by to change grouping:

  mycel cost report                  # per-repo totals
  mycel cost report --by repo        # per-repo totals
  mycel cost report --by project     # per-project totals (repo name grouping)
  mycel cost report --since 30d      # only include records from last 30 days`,
	RunE: runCostReport,
}

var (
	costReportBy    string
	costReportSince string
)

func init() {
	costReportCmd.Flags().StringVar(&costReportBy, "by", "repo", "Grouping: repo | project")
	costReportCmd.Flags().StringVar(&costReportSince, "since", "", "Include records since (e.g. 7d, 30d, 2026-01-01)")
	costCmd.AddCommand(costReportCmd)
}

func runCostReport(cmd *cobra.Command, _ []string) error {
	agentsDir, err := home.AgentsDir()
	if err != nil {
		return fmt.Errorf("resolve agents dir: %w", err)
	}
	userHome, _ := os.UserHomeDir() //nolint:errcheck // empty home just skips host sessions

	svc := cost.NewService(provider.DefaultRegistry, cost.Options{
		Home:      userHome,
		AgentsDir: agentsDir,
	}, nil)

	since, err := parseSinceFlag(costReportSince)
	if err != nil {
		return err
	}

	switch costReportBy {
	case "repo", "":
		return printCostByRepo(cmd.Context(), svc, since)
	case "project":
		return printCostByProject(cmd.Context(), svc, since)
	default:
		return fmt.Errorf("unknown --by %q (want: repo, project)", costReportBy)
	}
}

func printCostByRepo(ctx context.Context, svc *cost.Service, since time.Time) error {
	byRepo, err := svc.SumByRepo(ctx, since)
	if err != nil {
		return err
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
		name := repoLabel(k)
		_, _ = fmt.Fprintf(w, "%s\t%s\t$%.4f\n", name, k, total)
	}
	_, _ = fmt.Fprintf(w, "TOTAL\t\t$%.4f\n", grand)
	return w.Flush()
}

// repoLabel maps a repo path to a human-readable label: the repo
// directory basename, or "(unattributed)" for empty paths.
func repoLabel(repo string) string {
	if repo == "" {
		return "(unattributed)"
	}
	if base := filepath.Base(repo); base != "." && base != string(filepath.Separator) {
		return base
	}
	return repo
}

func printCostByProject(ctx context.Context, svc *cost.Service, since time.Time) error {
	resolve := func(repo string) string {
		if repo == "" {
			return ""
		}
		return repoLabel(repo)
	}
	byProj, err := svc.SumByProject(ctx, since, resolve)
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
