package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/doctor"
	"github.com/rpuneet/mycel/pkg/ui"
)

var doctorCmd = &cobra.Command{
	Use:     "doctor",
	Aliases: []string{"dr"},
	Short:   "Health checks and diagnostics",
	Long: `Run health checks on your mycel repo and dependencies.

Checks workspace config, agent state, databases, tools, and git worktrees.

Categories:
  workspace   state directory, preferences.json, role files
  database    SQLite integrity and table existence
  agents      Running agents, stale sessions, missing worktrees
  tools       tmux, git, and AI provider installations
  git         Worktree validity and orphaned worktrees

Examples:
  mycel doctor                          # Full health check
  mycel doctor check workspace          # Check specific category
  mycel doctor fix                      # Auto-fix fixable issues
  mycel doctor fix --dry-run            # Preview fixes
  mycel doctor fix --category git       # Fix specific category

Exit codes:
  0  All checks passed or only warnings
  1  One or more checks failed`,
	SilenceUsage: true,
	RunE:         runDoctor,
}

var doctorCheckCmd = &cobra.Command{
	Use:       "check [category]",
	Short:     "Check a specific health category",
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: doctor.ValidCategories(),
	RunE:      runDoctorCheck,
}

var doctorFixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Auto-fix fixable issues",
	Long: `Attempt to automatically repair fixable issues found by 'mycel doctor'.

Fixable issues include:
  - Orphaned git worktrees
  - Missing workspace directories

Use --dry-run to preview actions without making changes.

Examples:
  mycel doctor fix                      # Fix all fixable issues
  mycel doctor fix --dry-run            # Preview fixes
  mycel doctor fix --category git       # Fix specific category`,
	RunE: runDoctorFix,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.AddCommand(doctorCheckCmd)
	doctorCmd.AddCommand(doctorFixCmd)

	doctorFixCmd.Flags().Bool("dry-run", false, "Preview fixes without making changes")
	doctorFixCmd.Flags().String("category", "", "Fix only the specified category")
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	// Try bcd API first
	c := getClient()
	apiReport, apiErr := c.Doctor.RunAll(ctx)
	if apiErr == nil && apiReport != nil {
		printClientReport(apiReport)
		fail := countClientFails(apiReport)
		if fail > 0 {
			return fmt.Errorf("health check failed")
		}
		return nil
	}

	// Offline fallback: use direct pkg/doctor
	ws, err := getRepo()
	if err != nil {
		// No workspace: run tools-only check
		fmt.Println("mycel doctor")
		fmt.Println(strings.Repeat("─", 40))
		fmt.Println()
		fmt.Println(ui.YellowText("⚠") + " No mycel-adopted repo found — running tools check only")
		fmt.Println()
		cat := doctor.CheckTools(ctx, nil)
		printCategory(cat)
		fmt.Println()
		return nil
	}

	report := doctor.RunAll(ctx, ws)
	printReport(report)

	_, _, fail := report.Summary()
	if fail > 0 {
		return fmt.Errorf("health check failed")
	}
	return nil
}

func runDoctorCheck(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	if len(args) == 0 {
		return cmd.Help()
	}

	name := args[0]

	// Try bcd API first
	c := getClient()
	apiCat, apiErr := c.Doctor.ByCategory(ctx, name)
	if apiErr == nil && apiCat != nil {
		printClientCategory(apiCat)
		fmt.Println()
		fail := countClientCategoryFails(apiCat)
		if fail > 0 {
			return fmt.Errorf("check failed")
		}
		return nil
	}

	// Offline fallback
	ws, wsErr := getRepo()

	// Tools check works without a workspace
	if wsErr != nil && name != "tools" {
		return errNoRepo(wsErr)
	}

	var cat *doctor.CategoryReport
	if wsErr != nil {
		c := doctor.CheckTools(ctx, nil)
		cat = &c
	} else {
		cat = doctor.CategoryByName(ctx, ws, name)
		if cat == nil {
			return fmt.Errorf("unknown category %q — valid categories: %s",
				name, strings.Join(doctor.ValidCategories(), ", "))
		}
	}

	printCategory(*cat)
	fmt.Println()

	_, _, fail := cat.Counts()
	if fail > 0 {
		return fmt.Errorf("check failed")
	}
	return nil
}

func runDoctorFix(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return err
	}
	categoryFilter, err := cmd.Flags().GetString("category")
	if err != nil {
		return err
	}

	// Fix always uses direct pkg/doctor (requires local workspace access)
	ws, wsErr := requireRepo()
	if wsErr != nil {
		return wsErr
	}

	if dryRun {
		fmt.Println(ui.DimText("Dry-run mode — no changes will be made"))
		fmt.Println()
	}

	var fixes []doctor.FixResult

	if categoryFilter != "" {
		cat := doctor.CategoryByName(ctx, ws, categoryFilter)
		if cat == nil {
			return fmt.Errorf("unknown category %q — valid categories: %s",
				categoryFilter, strings.Join(doctor.ValidCategories(), ", "))
		}
		fixes = doctor.FixCategory(ctx, ws, cat, dryRun)
	} else {
		report := doctor.RunAll(ctx, ws)
		fixes = doctor.Fix(ctx, ws, report, dryRun)
	}

	if len(fixes) == 0 {
		fmt.Println(ui.GreenText("✓") + " Nothing to fix")
		return nil
	}

	for _, f := range fixes {
		icon := ui.GreenText("✓")
		if !f.Success {
			icon = ui.RedText("✗")
		}
		suffix := ""
		if f.Message != "" {
			suffix = "  " + ui.DimText(f.Message)
		}
		fmt.Printf("  %s %s%s\n", icon, f.Action, suffix)
	}
	fmt.Println()
	return nil
}

// ─── Output helpers ───────────────────────────────────────────────────────────

func printReport(report *doctor.Report) {
	fmt.Println("mycel doctor")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()

	for _, cat := range report.Categories {
		printCategory(cat)
		fmt.Println()
	}

	ok, warn, fail := report.Summary()
	summary := fmt.Sprintf("Summary: %d passed, %d failed, %d warnings",
		ok, fail, warn)

	if fail > 0 {
		fmt.Println(ui.RedText(summary))
		fmt.Println()
		fmt.Println("Run 'mycel doctor fix' to auto-repair fixable issues.")
	} else if warn > 0 {
		fmt.Println(ui.YellowText(summary))
	} else {
		fmt.Println(ui.GreenText(summary))
	}
}

func printCategory(cat doctor.CategoryReport) {
	fmt.Println(ui.BoldText(cat.Name))
	for _, item := range cat.Items {
		var icon string
		switch item.Severity {
		case doctor.SeverityOK:
			icon = ui.GreenText("✓")
		case doctor.SeverityWarn:
			icon = ui.YellowText("⚠")
		default:
			icon = ui.RedText("✗")
		}

		name := item.Name
		switch item.Severity {
		case doctor.SeverityFail:
			name = ui.RedText(name)
		case doctor.SeverityWarn:
			name = ui.YellowText(name)
		}

		fmt.Printf("  %s %-35s %s\n", icon, name, item.Message)
		if item.Fix != "" && (item.Severity == doctor.SeverityFail || item.Severity == doctor.SeverityWarn) {
			fmt.Printf("    %s %s\n", ui.DimText("→"), ui.DimText(item.Fix))
		}
	}
}

// ─── Client report helpers (for API-based output) ─────────────────────────────

func printClientReport(report *client.DoctorReport) {
	fmt.Println("mycel doctor")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()

	for i := range report.Categories {
		printClientCategory(&report.Categories[i])
		fmt.Println()
	}

	ok, warn, fail := countClientSummary(report)
	summary := fmt.Sprintf("Summary: %d passed, %d failed, %d warnings",
		ok, fail, warn)

	if fail > 0 {
		fmt.Println(ui.RedText(summary))
		fmt.Println()
		fmt.Println("Run 'mycel doctor fix' to auto-repair fixable issues.")
	} else if warn > 0 {
		fmt.Println(ui.YellowText(summary))
	} else {
		fmt.Println(ui.GreenText(summary))
	}
}

func printClientCategory(cat *client.DoctorCategory) {
	fmt.Println(ui.BoldText(cat.Name))
	for _, item := range cat.Items {
		var icon string
		switch item.Severity {
		case "ok":
			icon = ui.GreenText("✓")
		case "warn":
			icon = ui.YellowText("⚠")
		default:
			icon = ui.RedText("✗")
		}

		name := item.Name
		switch item.Severity {
		case "fail":
			name = ui.RedText(name)
		case "warn":
			name = ui.YellowText(name)
		}

		fmt.Printf("  %s %-35s %s\n", icon, name, item.Message)
		if item.Fix != "" && (item.Severity == "fail" || item.Severity == "warn") {
			fmt.Printf("    %s %s\n", ui.DimText("→"), ui.DimText(item.Fix))
		}
	}
}

func countClientFails(report *client.DoctorReport) int {
	_, _, fail := countClientSummary(report)
	return fail
}

func countClientCategoryFails(cat *client.DoctorCategory) int {
	fail := 0
	for _, item := range cat.Items {
		if item.Severity == "fail" {
			fail++
		}
	}
	return fail
}

func countClientSummary(report *client.DoctorReport) (ok, warn, fail int) {
	for _, cat := range report.Categories {
		for _, item := range cat.Items {
			switch item.Severity {
			case "ok":
				ok++
			case "warn":
				warn++
			case "fail":
				fail++
			}
		}
	}
	return
}
