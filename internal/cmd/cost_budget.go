package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
)

// Issue #1648: Extracted from cost.go for better code organization
// Budget management commands

var costBudgetCmd = &cobra.Command{
	Use:   "budget",
	Short: "Manage cost budgets",
	Long: `Commands for managing cost budgets and limits.

Examples:
  mycel cost budget show
  mycel cost budget set 100.00
  mycel cost budget set 50.00 --agent engineer-01
  mycel cost budget set 500.00 --period monthly --alert-at 0.9`,
}

var costBudgetSetCmd = &cobra.Command{
	Use:   "set <amount>",
	Short: "Set a cost budget",
	Long: `Set a cost budget for the repo, agent, or team.

Examples:
  mycel cost budget set 100.00                          # Set workspace budget to $100
  mycel cost budget set 50.00 --agent engineer-01       # Set agent budget
  mycel cost budget set 500.00 --team engineering       # Set team budget
  mycel cost budget set 100.00 --period weekly          # Weekly budget
  mycel cost budget set 100.00 --alert-at 0.9           # Alert at 90%
  mycel cost budget set 100.00 --hard-stop              # Stop when limit reached`,
	Args: cobra.ExactArgs(1),
	RunE: runCostBudgetSet,
}

var costBudgetShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show budget status",
	Long: `Show current budget configuration and status.

Examples:
  mycel cost budget show                   # Show all budgets
  mycel cost budget show --agent eng-01    # Show agent budget`,
	RunE: runCostBudgetShow,
}

var costBudgetDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a budget",
	Long: `Delete a budget configuration.

Examples:
  mycel cost budget delete                  # Delete workspace budget
  mycel cost budget delete --agent eng-01   # Delete agent budget`,
	RunE: runCostBudgetDelete,
}

// Budget flags
var (
	budgetAgentFlag   string
	budgetTeamFlag    string
	budgetPeriodFlag  string
	budgetAlertAtFlag float64
	budgetHardStop    bool
)

func initCostBudgetFlags() {
	costBudgetSetCmd.Flags().StringVar(&budgetAgentFlag, "agent", "", "Set budget for specific agent")
	costBudgetSetCmd.Flags().StringVar(&budgetTeamFlag, "team", "", "Set budget for specific team")
	costBudgetSetCmd.Flags().StringVar(&budgetPeriodFlag, "period", "monthly", "Budget period (daily, weekly, monthly)")
	costBudgetSetCmd.Flags().Float64Var(&budgetAlertAtFlag, "alert-at", 0.8, "Alert when usage reaches this percentage (0.0-1.0)")
	costBudgetSetCmd.Flags().BoolVar(&budgetHardStop, "hard-stop", false, "Stop operations when budget is exceeded")

	costBudgetShowCmd.Flags().StringVar(&budgetAgentFlag, "agent", "", "Show budget for specific agent")
	costBudgetShowCmd.Flags().StringVar(&budgetTeamFlag, "team", "", "Show budget for specific team")

	costBudgetDeleteCmd.Flags().StringVar(&budgetAgentFlag, "agent", "", "Delete budget for specific agent")
	costBudgetDeleteCmd.Flags().StringVar(&budgetTeamFlag, "team", "", "Delete budget for specific team")

	costBudgetCmd.AddCommand(costBudgetSetCmd)
	costBudgetCmd.AddCommand(costBudgetShowCmd)
	costBudgetCmd.AddCommand(costBudgetDeleteCmd)
}

func getBudgetScope() string {
	if budgetAgentFlag != "" {
		return "agent:" + budgetAgentFlag
	}
	if budgetTeamFlag != "" {
		return "team:" + budgetTeamFlag
	}
	return "workspace"
}

func runCostBudgetSet(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	// Parse amount
	var limitUSD float64
	if _, parseErr := fmt.Sscanf(args[0], "%f", &limitUSD); parseErr != nil {
		return fmt.Errorf("invalid amount: %s", args[0])
	}
	if limitUSD <= 0 {
		return fmt.Errorf("budget amount must be positive")
	}

	// Validate period
	switch budgetPeriodFlag {
	case "daily", "weekly", "monthly":
		// Valid
	default:
		return fmt.Errorf("invalid period: %s (must be daily, weekly, or monthly)", budgetPeriodFlag)
	}

	// Validate alert threshold
	if budgetAlertAtFlag < 0 || budgetAlertAtFlag > 1 {
		return fmt.Errorf("alert-at must be between 0.0 and 1.0")
	}

	scope := getBudgetScope()
	budget, err := c.Costs.SetBudget(cmd.Context(), &client.SetBudgetReq{
		Scope:    scope,
		Period:   budgetPeriodFlag,
		LimitUSD: limitUSD,
		AlertAt:  budgetAlertAtFlag,
		HardStop: budgetHardStop,
	})
	if err != nil {
		return fmt.Errorf("failed to set budget: %w", err)
	}

	fmt.Printf("Budget set for %s:\n", scope)
	fmt.Printf("  Limit:     $%.2f/%s\n", budget.LimitUSD, budget.Period)
	fmt.Printf("  Alert at:  %.0f%%\n", budget.AlertAt*100)
	fmt.Printf("  Hard stop: %v\n", budget.HardStop)

	return nil
}

func runCostBudgetShow(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	scope := getBudgetScope()

	// If showing specific scope
	if budgetAgentFlag != "" || budgetTeamFlag != "" || scope == "workspace" {
		status, checkErr := c.Costs.CheckBudget(cmd.Context(), scope)
		if checkErr != nil {
			return fmt.Errorf("failed to check budget: %w", checkErr)
		}

		if status == nil {
			fmt.Printf("No budget configured for %s\n", scope)
			return nil
		}

		printBudgetStatus(scope, status)
		return nil
	}

	// Show all budgets
	budgets, err := c.Costs.ListBudgets(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get budgets: %w", err)
	}

	if len(budgets) == 0 {
		fmt.Println("No budgets configured")
		fmt.Println("\nUse 'mycel cost budget set <amount>' to set a budget")
		return nil
	}

	fmt.Println("Configured Budgets")
	fmt.Println("==================")

	for _, b := range budgets {
		status, _ := c.Costs.CheckBudget(cmd.Context(), b.Scope)
		if status != nil {
			printBudgetStatus(b.Scope, status)
			fmt.Println()
		}
	}

	return nil
}

func printBudgetStatus(scope string, status *client.CostBudgetStatus) {
	fmt.Printf("Budget: %s\n", scope)
	fmt.Printf("  Period:    %s\n", status.Budget.Period)
	fmt.Printf("  Limit:     $%.2f\n", status.Budget.LimitUSD)
	fmt.Printf("  Spent:     $%.2f (%.1f%%)\n", status.CurrentSpend, status.PercentUsed*100)
	fmt.Printf("  Remaining: $%.2f\n", status.Remaining)

	if status.IsOverBudget {
		fmt.Println("  Status:    ⚠️  OVER BUDGET")
	} else if status.IsNearLimit {
		fmt.Printf("  Status:    ⚠️  Near limit (alert at %.0f%%)\n", status.Budget.AlertAt*100)
	} else {
		fmt.Println("  Status:    ✓ Within budget")
	}

	if status.Budget.HardStop {
		fmt.Println("  Hard stop: Enabled")
	}
}

func runCostBudgetDelete(cmd *cobra.Command, args []string) error {
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	scope := getBudgetScope()

	if deleteErr := c.Costs.DeleteBudget(cmd.Context(), scope); deleteErr != nil {
		return fmt.Errorf("failed to delete budget: %w", deleteErr)
	}

	fmt.Printf("Budget deleted for %s\n", scope)
	return nil
}
