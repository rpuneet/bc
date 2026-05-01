package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/cost"
)

// Cost command tests use executeIntegrationCmd which captures os.Stdout
// because cost.go uses fmt.Printf directly rather than cmd.OutOrStdout()

// resetCostFlags resets the cost command flags between tests
func resetCostFlags() {
	costLimitFlag = 20
}

// resetBudgetFlags resets the budget command flags between tests
func resetBudgetFlags() {
	budgetAgentFlag = ""
	budgetTeamFlag = ""
	budgetPeriodFlag = "monthly"
	budgetAlertAtFlag = 0.8
	budgetHardStop = false
}

func TestCostShowEmpty(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	stdout, _, err := executeIntegrationCmd("cost", "show")
	if err != nil {
		t.Fatalf("cost show failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "No cost records found") {
		t.Errorf("expected 'No cost records found', got: %s", stdout)
	}
}

func TestCostShowWithRecords(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Create cost records
	store := cost.NewStore(wsDir)
	if err := store.Open(); err != nil {
		t.Fatalf("failed to open cost store: %v", err)
	}

	_, err := store.Record(context.Background(), "engineer-01", "", "claude-3-opus", 1000, 500, 0.05)
	if err != nil {
		t.Fatalf("failed to record cost: %v", err)
	}
	_, err = store.Record(context.Background(), "engineer-02", "", "claude-3-sonnet", 2000, 1000, 0.03)
	if err != nil {
		t.Fatalf("failed to record cost: %v", err)
	}
	_ = store.Close()

	stdout, _, cmdErr := executeIntegrationCmd("cost", "show")
	if cmdErr != nil {
		t.Fatalf("cost show failed: %v\nOutput: %s", cmdErr, stdout)
	}
	if !strings.Contains(stdout, "engineer-01") {
		t.Errorf("output should contain engineer-01: %s", stdout)
	}
	if !strings.Contains(stdout, "engineer-02") {
		t.Errorf("output should contain engineer-02: %s", stdout)
	}
	if !strings.Contains(stdout, "claude-3-opus") {
		t.Errorf("output should contain model: %s", stdout)
	}
}

func TestCostShowByAgent(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	store := cost.NewStore(wsDir)
	if err := store.Open(); err != nil {
		t.Fatalf("failed to open cost store: %v", err)
	}

	_, _ = store.Record(context.Background(), "engineer-01", "", "claude-3-opus", 1000, 500, 0.05)
	_, _ = store.Record(context.Background(), "engineer-02", "", "claude-3-sonnet", 2000, 1000, 0.03)
	_ = store.Close()

	stdout, _, err := executeIntegrationCmd("cost", "show", "engineer-01")
	if err != nil {
		t.Fatalf("cost show agent failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "engineer-01") {
		t.Errorf("output should contain engineer-01: %s", stdout)
	}
	// Should not contain records from other agents
	if strings.Contains(stdout, "engineer-02") {
		t.Errorf("output should not contain engineer-02: %s", stdout)
	}
}

func TestCostShowLimit(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	store := cost.NewStore(wsDir)
	if err := store.Open(); err != nil {
		t.Fatalf("failed to open cost store: %v", err)
	}

	// Create 5 records
	for i := 0; i < 5; i++ {
		_, _ = store.Record(context.Background(), "engineer-01", "", "claude-3-opus", int64(1000+i*100), 500, 0.05)
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}
	_ = store.Close()

	stdout, _, err := executeIntegrationCmd("cost", "show", "--limit", "2")
	if err != nil {
		t.Fatalf("cost show --limit failed: %v\nOutput: %s", err, stdout)
	}

	// Count lines with engineer-01 (excluding header)
	lines := strings.Split(stdout, "\n")
	count := 0
	for _, line := range lines {
		if strings.Contains(line, "engineer-01") {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 records with --limit 2, got %d\nOutput: %s", count, stdout)
	}
}

func TestCostShowNegativeLimit(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetCostFlags()
	defer resetCostFlags()

	_, _, err := executeIntegrationCmd("cost", "show", "--limit", "-5")
	if err == nil {
		t.Error("expected error for negative limit")
	}
	if !strings.Contains(err.Error(), "must be a positive number") {
		t.Errorf("error should mention positive number: %v", err)
	}
}

func TestCostShowZeroLimit(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetCostFlags()
	defer resetCostFlags()

	_, _, err := executeIntegrationCmd("cost", "show", "--limit", "0")
	if err == nil {
		t.Error("expected error for zero limit")
	}
	if !strings.Contains(err.Error(), "must be a positive number") {
		t.Errorf("error should mention positive number: %v", err)
	}
}

func TestCostShowJSON(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetCostFlags()
	defer resetCostFlags()

	store := cost.NewStore(wsDir)
	if err := store.Open(); err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	_, _ = store.Record(context.Background(), "engineer-01", "", "claude-3-opus", 1000, 500, 0.05)
	_ = store.Close()

	stdout, _, err := executeIntegrationCmd("cost", "show", "--json")
	if err != nil {
		t.Fatalf("cost show --json failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "by_agent") {
		t.Errorf("expected JSON structure with by_agent, got: %s", stdout)
	}
	if !strings.Contains(stdout, "total_cost") {
		t.Errorf("expected total_cost in JSON, got: %s", stdout)
	}
}

func TestCostShowNonExistentAgent(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetCostFlags()
	defer resetCostFlags()

	stdout, _, err := executeIntegrationCmd("cost", "show", "nonexistent-agent")
	if err != nil {
		t.Fatalf("cost show nonexistent-agent should not error: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "No cost records found") {
		t.Errorf("expected no records message, got: %s", stdout)
	}
}

func TestCostShowLargeLimit(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetCostFlags()
	defer resetCostFlags()

	store := cost.NewStore(wsDir)
	if err := store.Open(); err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	// Create many records
	for i := 0; i < 50; i++ {
		_, _ = store.Record(context.Background(), "engineer-01", "", "claude-3-opus", int64(1000+i), 500, 0.05)
	}
	_ = store.Close()

	stdout, _, err := executeIntegrationCmd("cost", "show", "--limit", "100")
	if err != nil {
		t.Fatalf("cost show --limit 100 failed: %v\nOutput: %s", err, stdout)
	}

	// Count records
	lines := strings.Split(stdout, "\n")
	count := 0
	for _, line := range lines {
		if strings.Contains(line, "engineer-01") {
			count++
		}
	}
	if count != 50 {
		t.Errorf("expected 50 records, got %d", count)
	}
}

func TestCostShowMultipleModels(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetCostFlags()
	defer resetCostFlags()

	store := cost.NewStore(wsDir)
	if err := store.Open(); err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	// Create records with multiple models
	_, _ = store.Record(context.Background(), "eng-01", "", "claude-3-opus", 1000, 500, 0.10)
	_, _ = store.Record(context.Background(), "eng-01", "", "claude-3-sonnet", 1000, 500, 0.05)
	_, _ = store.Record(context.Background(), "eng-01", "", "claude-3-haiku", 1000, 500, 0.01)
	_, _ = store.Record(context.Background(), "eng-01", "", "gpt-4", 1000, 500, 0.08)
	_ = store.Close()

	stdout, _, err := executeIntegrationCmd("cost", "show")
	if err != nil {
		t.Fatalf("cost show failed: %v\nOutput: %s", err, stdout)
	}
	// Verify all models appear
	models := []string{"claude-3-opus", "claude-3-sonnet", "claude-3-haiku", "gpt-4"}
	for _, m := range models {
		if !strings.Contains(stdout, m) {
			t.Errorf("expected model %s in output: %s", m, stdout)
		}
	}
}

// Budget tests

func TestCostBudgetSetWorkspace(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	stdout, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00")
	if err != nil {
		t.Fatalf("cost budget set failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "100.00") {
		t.Errorf("expected budget amount in output: %s", stdout)
	}
}

func TestCostBudgetSetAgent(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	stdout, _, err := executeIntegrationCmd("cost", "budget", "set", "50.00", "--agent", "eng-01")
	if err != nil {
		t.Fatalf("cost budget set --agent failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "50.00") {
		t.Errorf("expected budget amount: %s", stdout)
	}
}

func TestCostBudgetSetTeam(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	stdout, _, err := executeIntegrationCmd("cost", "budget", "set", "500.00", "--team", "engineering")
	if err != nil {
		t.Fatalf("cost budget set --team failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "500.00") {
		t.Errorf("expected budget amount: %s", stdout)
	}
}

func TestCostBudgetSetPeriods(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	periods := []string{"daily", "weekly", "monthly"}
	for _, period := range periods {
		t.Run(period, func(t *testing.T) {
			resetBudgetFlags()
			stdout, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00", "--period", period)
			if err != nil {
				t.Fatalf("cost budget set --period %s failed: %v\nOutput: %s", period, err, stdout)
			}
			if !strings.Contains(stdout, period) {
				t.Errorf("expected period %s in output: %s", period, stdout)
			}
		})
	}
}

func TestCostBudgetSetInvalidPeriod(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	_, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00", "--period", "yearly")
	if err == nil {
		t.Error("expected error for invalid period")
	}
}

func TestCostBudgetSetAlertAt(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	stdout, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00", "--alert-at", "0.9")
	if err != nil {
		t.Fatalf("cost budget set --alert-at failed: %v\nOutput: %s", err, stdout)
	}
}

func TestCostBudgetSetAlertAtInvalid(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	tests := []struct {
		name    string
		alertAt string
	}{
		{"negative", "-0.1"},
		{"over_one", "1.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetBudgetFlags()
			_, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00", "--alert-at", tt.alertAt)
			if err == nil {
				t.Errorf("expected error for alert-at=%s", tt.alertAt)
			}
		})
	}
}

func TestCostBudgetSetHardStop(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	stdout, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00", "--hard-stop")
	if err != nil {
		t.Fatalf("cost budget set --hard-stop failed: %v\nOutput: %s", err, stdout)
	}
}

func TestCostBudgetSetZeroAmount(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	_, _, err := executeIntegrationCmd("cost", "budget", "set", "0")
	if err == nil {
		t.Error("expected error for zero budget")
	}
}

func TestCostBudgetSetNegativeAmount(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	_, _, err := executeIntegrationCmd("cost", "budget", "set", "-50.00")
	if err == nil {
		t.Error("expected error for negative budget")
	}
}

func TestCostBudgetSetInvalidAmount(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	_, _, err := executeIntegrationCmd("cost", "budget", "set", "abc")
	if err == nil {
		t.Error("expected error for non-numeric budget")
	}
}

func TestCostBudgetShowNoBudgets(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	stdout, _, err := executeIntegrationCmd("cost", "budget", "show")
	if err != nil {
		t.Fatalf("cost budget show failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "No budget") {
		t.Errorf("expected 'No budget' message, got: %s", stdout)
	}
}

func TestCostBudgetShowWorkspace(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	// Set a budget first
	_, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00")
	if err != nil {
		t.Fatalf("failed to set budget: %v", err)
	}

	resetBudgetFlags()
	stdout, _, err := executeIntegrationCmd("cost", "budget", "show")
	if err != nil {
		t.Fatalf("cost budget show failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "100.00") {
		t.Errorf("expected budget amount in show output: %s", stdout)
	}
}

func TestCostBudgetShowWithSpending(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	// Set budget
	_, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00")
	if err != nil {
		t.Fatalf("failed to set budget: %v", err)
	}

	// Add some spending
	store := cost.NewStore(wsDir)
	if storeErr := store.Open(); storeErr != nil {
		t.Fatalf("failed to open store: %v", storeErr)
	}
	_, _ = store.Record(context.Background(), "eng-01", "", "claude-3-opus", 1000, 500, 25.00)
	_ = store.Close()

	resetBudgetFlags()
	stdout, _, err := executeIntegrationCmd("cost", "budget", "show")
	if err != nil {
		t.Fatalf("cost budget show failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "25.00") || !strings.Contains(stdout, "100.00") {
		t.Errorf("expected spending and budget in output: %s", stdout)
	}
}

func TestCostBudgetShowNearLimit(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	// Set budget with 80% alert
	_, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00", "--alert-at", "0.8")
	if err != nil {
		t.Fatalf("failed to set budget: %v", err)
	}

	// Spend 85%
	store := cost.NewStore(wsDir)
	if storeErr := store.Open(); storeErr != nil {
		t.Fatalf("failed to open store: %v", storeErr)
	}
	_, _ = store.Record(context.Background(), "eng-01", "", "claude-3-opus", 1000, 500, 85.00)
	_ = store.Close()

	resetBudgetFlags()
	stdout, _, err := executeIntegrationCmd("cost", "budget", "show")
	if err != nil {
		t.Fatalf("cost budget show failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "NEAR LIMIT") && !strings.Contains(stdout, "WARNING") && !strings.Contains(stdout, "85") {
		t.Errorf("expected near-limit warning in output: %s", stdout)
	}
}

func TestCostBudgetShowOverBudget(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	// Set small budget
	_, _, err := executeIntegrationCmd("cost", "budget", "set", "10.00")
	if err != nil {
		t.Fatalf("failed to set budget: %v", err)
	}

	// Spend more than budget
	store := cost.NewStore(wsDir)
	if storeErr := store.Open(); storeErr != nil {
		t.Fatalf("failed to open store: %v", storeErr)
	}
	_, _ = store.Record(context.Background(), "eng-01", "", "claude-3-opus", 1000, 500, 15.00)
	_ = store.Close()

	resetBudgetFlags()
	stdout, _, err := executeIntegrationCmd("cost", "budget", "show")
	if err != nil {
		t.Fatalf("cost budget show failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "OVER BUDGET") && !strings.Contains(stdout, "15") {
		t.Errorf("expected over-budget warning in output: %s", stdout)
	}
}

func TestCostBudgetDelete(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	// Set then delete
	_, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00")
	if err != nil {
		t.Fatalf("failed to set budget: %v", err)
	}

	resetBudgetFlags()
	_, _, err = executeIntegrationCmd("cost", "budget", "delete")
	if err != nil {
		t.Fatalf("cost budget delete failed: %v", err)
	}
}

func TestCostBudgetDeleteAgent(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	// Set agent budget
	_, _, err := executeIntegrationCmd("cost", "budget", "set", "50.00", "--agent", "eng-01")
	if err != nil {
		t.Fatalf("failed to set agent budget: %v", err)
	}

	resetBudgetFlags()
	_, _, err = executeIntegrationCmd("cost", "budget", "delete", "--agent", "eng-01")
	if err != nil {
		t.Fatalf("cost budget delete --agent failed: %v", err)
	}

	// Verify deleted
	resetBudgetFlags()
	stdout, _, err := executeIntegrationCmd("cost", "budget", "show", "--agent", "eng-01")
	if err != nil {
		t.Fatalf("cost budget show failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "No budget") {
		t.Errorf("expected no budget after delete: %s", stdout)
	}
}

func TestCostBudgetUpdateExisting(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	// Set initial budget
	_, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00")
	if err != nil {
		t.Fatalf("failed to set initial budget: %v", err)
	}

	resetBudgetFlags()
	// Update to new amount
	stdout, _, err := executeIntegrationCmd("cost", "budget", "set", "200.00")
	if err != nil {
		t.Fatalf("failed to update budget: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "200.00") {
		t.Errorf("expected updated amount 200.00, got: %s", stdout)
	}
}

func TestCostBudgetSetMissingAmount(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	_, _, err := executeIntegrationCmd("cost", "budget", "set")
	if err == nil {
		t.Error("expected error for missing amount")
	}
}

func TestCostBudgetShowAgentSpecific(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	// Set agent budget
	_, _, err := executeIntegrationCmd("cost", "budget", "set", "75.00", "--agent", "eng-01")
	if err != nil {
		t.Fatalf("failed to set agent budget: %v", err)
	}

	resetBudgetFlags()
	stdout, _, err := executeIntegrationCmd("cost", "budget", "show", "--agent", "eng-01")
	if err != nil {
		t.Fatalf("cost budget show --agent failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "75.00") {
		t.Errorf("expected budget amount 75.00, got: %s", stdout)
	}
}

func TestCostBudgetShowTeamSpecific(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()
	resetBudgetFlags()
	defer resetBudgetFlags()

	// Set team budget
	_, _, err := executeIntegrationCmd("cost", "budget", "set", "300.00", "--team", "engineering")
	if err != nil {
		t.Fatalf("failed to set team budget: %v", err)
	}

	resetBudgetFlags()
	stdout, _, err := executeIntegrationCmd("cost", "budget", "show", "--team", "engineering")
	if err != nil {
		t.Fatalf("cost budget show --team failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "300.00") {
		t.Errorf("expected budget amount 300.00, got: %s", stdout)
	}
}

// --- ccusage enrichment tests ---

func TestEnrichWithCCUsage(t *testing.T) {
	resp := &costShowResponse{
		ByAgent:           make(map[string]float64),
		ByTeam:            make(map[string]float64),
		ByModel:           make(map[string]float64),
		TotalInputTokens:  0,
		TotalOutputTokens: 0,
		TotalCost:         0,
	}

	report := &ccusageDailyReport{
		Daily: []ccusageDailyEntry{
			{
				Date:                "2026-03-01",
				ModelsUsed:          []string{"claude-opus-4-20250514", "claude-sonnet-4-20250514"},
				InputTokens:         1000,
				OutputTokens:        5000,
				CacheCreationTokens: 200,
				CacheReadTokens:     800,
				TotalTokens:         7000,
				TotalCost:           3.50,
			},
			{
				Date:                "2026-03-02",
				ModelsUsed:          []string{"claude-opus-4-20250514"},
				InputTokens:         500,
				OutputTokens:        2500,
				CacheCreationTokens: 100,
				CacheReadTokens:     400,
				TotalTokens:         3500,
				TotalCost:           1.75,
			},
		},
		Totals: ccusageTotals{
			InputTokens:         1500,
			OutputTokens:        7500,
			CacheCreationTokens: 300,
			CacheReadTokens:     1200,
			TotalTokens:         10500,
			TotalCost:           5.25,
		},
	}

	enrichWithCCUsage(resp, report)

	// Totals should be overridden from ccusage (internal DB was empty)
	if resp.TotalCost != 5.25 {
		t.Errorf("TotalCost = %f, want 5.25", resp.TotalCost)
	}
	if resp.TotalInputTokens != 1500 {
		t.Errorf("TotalInputTokens = %d, want 1500", resp.TotalInputTokens)
	}
	if resp.TotalOutputTokens != 7500 {
		t.Errorf("TotalOutputTokens = %d, want 7500", resp.TotalOutputTokens)
	}

	// cache_hit_rate = 1200 / (1200 + 300) = 0.8
	if resp.CacheHitRate == nil {
		t.Fatal("CacheHitRate is nil")
	}
	if *resp.CacheHitRate != 0.8 {
		t.Errorf("CacheHitRate = %f, want 0.8", *resp.CacheHitRate)
	}

	// burn_rate = 5.25 / 2 = 2.625
	if resp.BurnRate == nil {
		t.Fatal("BurnRate is nil")
	}
	if *resp.BurnRate != 2.625 {
		t.Errorf("BurnRate = %f, want 2.625", *resp.BurnRate)
	}

	// projected_total = burn_rate * days_in_current_month
	if resp.ProjectedTotal == nil {
		t.Fatal("ProjectedTotal is nil")
	}
	now := time.Now()
	daysInMonth := float64(time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day())
	expectedProjected := 2.625 * daysInMonth
	if *resp.ProjectedTotal != expectedProjected {
		t.Errorf("ProjectedTotal = %f, want %f", *resp.ProjectedTotal, expectedProjected)
	}

	// billing_window_spent
	if resp.BillingWindowSpent == nil {
		t.Fatal("BillingWindowSpent is nil")
	}
	if *resp.BillingWindowSpent != 5.25 {
		t.Errorf("BillingWindowSpent = %f, want 5.25", *resp.BillingWindowSpent)
	}

	// by_model should have models from ccusage (since internal DB was empty)
	if len(resp.ByModel) != 2 {
		t.Errorf("ByModel has %d entries, want 2", len(resp.ByModel))
	}
	if _, ok := resp.ByModel["claude-opus-4-20250514"]; !ok {
		t.Error("ByModel missing claude-opus-4-20250514")
	}
	if _, ok := resp.ByModel["claude-sonnet-4-20250514"]; !ok {
		t.Error("ByModel missing claude-sonnet-4-20250514")
	}
}

func TestEnrichWithCCUsage_NilReport(t *testing.T) {
	resp := &costShowResponse{
		ByAgent:           make(map[string]float64),
		ByTeam:            make(map[string]float64),
		ByModel:           make(map[string]float64),
		TotalInputTokens:  100,
		TotalOutputTokens: 200,
		TotalCost:         0.05,
	}

	enrichWithCCUsage(resp, nil)

	// Nothing should change
	if resp.TotalCost != 0.05 {
		t.Errorf("TotalCost = %f, want 0.05", resp.TotalCost)
	}
	if resp.CacheHitRate != nil {
		t.Error("CacheHitRate should be nil when report is nil")
	}
	if resp.BurnRate != nil {
		t.Error("BurnRate should be nil when report is nil")
	}
	if resp.ProjectedTotal != nil {
		t.Error("ProjectedTotal should be nil when report is nil")
	}
	if resp.BillingWindowSpent != nil {
		t.Error("BillingWindowSpent should be nil when report is nil")
	}
}

func TestEnrichWithCCUsage_NoCache(t *testing.T) {
	resp := &costShowResponse{
		ByAgent:           make(map[string]float64),
		ByTeam:            make(map[string]float64),
		ByModel:           make(map[string]float64),
		TotalInputTokens:  0,
		TotalOutputTokens: 0,
		TotalCost:         0,
	}

	report := &ccusageDailyReport{
		Daily: []ccusageDailyEntry{
			{Date: "2026-03-01", TotalTokens: 1000, TotalCost: 2.00},
		},
		Totals: ccusageTotals{
			InputTokens:         500,
			OutputTokens:        500,
			CacheCreationTokens: 0,
			CacheReadTokens:     0,
			TotalTokens:         1000,
			TotalCost:           2.00,
		},
	}

	enrichWithCCUsage(resp, report)

	// cache_hit_rate should be nil when no cache tokens
	if resp.CacheHitRate != nil {
		t.Errorf("CacheHitRate should be nil with no cache, got %f", *resp.CacheHitRate)
	}

	// burn_rate and projected_total should still be set
	if resp.BurnRate == nil {
		t.Fatal("BurnRate should not be nil")
	}
	if *resp.BurnRate != 2.00 {
		t.Errorf("BurnRate = %f, want 2.00", *resp.BurnRate)
	}
}

func TestEnrichWithCCUsage_InternalDBHasData(t *testing.T) {
	// When internal DB has data, totals should NOT be overridden
	resp := &costShowResponse{
		ByAgent:           map[string]float64{"eng-01": 0.05},
		ByTeam:            make(map[string]float64),
		ByModel:           map[string]float64{"claude-opus": 0.05},
		TotalInputTokens:  1000,
		TotalOutputTokens: 500,
		TotalCost:         0.05,
	}

	report := &ccusageDailyReport{
		Daily: []ccusageDailyEntry{
			{Date: "2026-03-01", ModelsUsed: []string{"opus"}, TotalCost: 10.00},
		},
		Totals: ccusageTotals{
			InputTokens:  5000,
			OutputTokens: 25000,
			TotalCost:    10.00,
		},
	}

	enrichWithCCUsage(resp, report)

	// TotalCost should NOT be overridden since internal DB had data
	if resp.TotalCost != 0.05 {
		t.Errorf("TotalCost = %f, want 0.05 (should not be overridden)", resp.TotalCost)
	}

	// ByModel should NOT be overridden since internal DB had data
	if len(resp.ByModel) != 1 {
		t.Errorf("ByModel should keep internal DB data, got %d entries", len(resp.ByModel))
	}

	// ccusage-derived fields should still be set
	if resp.BurnRate == nil {
		t.Fatal("BurnRate should be set even with internal DB data")
	}
	if resp.BillingWindowSpent == nil {
		t.Fatal("BillingWindowSpent should be set")
	}
	if *resp.BillingWindowSpent != 10.00 {
		t.Errorf("BillingWindowSpent = %f, want 10.00", *resp.BillingWindowSpent)
	}
}

func TestEnrichWithCCUsage_EmptyDaily(t *testing.T) {
	resp := &costShowResponse{
		ByAgent:           make(map[string]float64),
		ByTeam:            make(map[string]float64),
		ByModel:           make(map[string]float64),
		TotalInputTokens:  0,
		TotalOutputTokens: 0,
		TotalCost:         0,
	}

	report := &ccusageDailyReport{
		Daily:  []ccusageDailyEntry{},
		Totals: ccusageTotals{TotalCost: 0},
	}

	enrichWithCCUsage(resp, report)

	// No burn_rate or projected_total with empty daily entries
	if resp.BurnRate != nil {
		t.Error("BurnRate should be nil with empty daily entries")
	}
	if resp.ProjectedTotal != nil {
		t.Error("ProjectedTotal should be nil with empty daily entries")
	}
	if resp.BillingWindowSpent != nil {
		t.Error("BillingWindowSpent should be nil with zero cost")
	}
}

func TestFetchCCUsageDailyReport_MockRunner(t *testing.T) {
	// Save and restore original runner
	origRunner := ccusageRunner
	defer func() { ccusageRunner = origRunner }()

	t.Run("valid_response", func(t *testing.T) {
		ccusageRunner = func(_ context.Context) ([]byte, error) {
			return []byte(`{
				"daily": [{"date":"2026-03-01","inputTokens":100,"outputTokens":200,"cacheCreationTokens":10,"cacheReadTokens":50,"totalTokens":360,"totalCost":1.50,"modelsUsed":["opus"]}],
				"totals": {"inputTokens":100,"outputTokens":200,"cacheCreationTokens":10,"cacheReadTokens":50,"totalTokens":360,"totalCost":1.50}
			}`), nil
		}

		report := fetchCCUsageDailyReport(context.Background())
		if report == nil {
			t.Fatal("expected non-nil report")
		}
		if len(report.Daily) != 1 {
			t.Errorf("Daily entries = %d, want 1", len(report.Daily))
		}
		if report.Totals.TotalCost != 1.50 {
			t.Errorf("TotalCost = %f, want 1.50", report.Totals.TotalCost)
		}
	})

	t.Run("runner_error", func(t *testing.T) {
		ccusageRunner = func(_ context.Context) ([]byte, error) {
			return nil, fmt.Errorf("npx not found")
		}

		report := fetchCCUsageDailyReport(context.Background())
		if report != nil {
			t.Error("expected nil report when runner fails")
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		ccusageRunner = func(_ context.Context) ([]byte, error) {
			return []byte("not json"), nil
		}

		report := fetchCCUsageDailyReport(context.Background())
		if report != nil {
			t.Error("expected nil report for invalid JSON")
		}
	})
}

func TestCostShowJSON_WithCCUsageEnrichment(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Mock ccusage runner
	origRunner := ccusageRunner
	defer func() { ccusageRunner = origRunner }()

	ccusageRunner = func(_ context.Context) ([]byte, error) {
		return []byte(`{
			"daily": [
				{"date":"2026-03-01","inputTokens":1000,"outputTokens":5000,"cacheCreationTokens":200,"cacheReadTokens":800,"totalTokens":7000,"totalCost":3.50,"modelsUsed":["claude-opus-4-20250514"]},
				{"date":"2026-03-02","inputTokens":500,"outputTokens":2500,"cacheCreationTokens":100,"cacheReadTokens":400,"totalTokens":3500,"totalCost":1.75,"modelsUsed":["claude-opus-4-20250514","claude-sonnet-4-20250514"]}
			],
			"totals": {"inputTokens":1500,"outputTokens":7500,"cacheCreationTokens":300,"cacheReadTokens":1200,"totalTokens":10500,"totalCost":5.25}
		}`), nil
	}

	stdout, _, err := executeIntegrationCmd("cost", "show", "--json")
	if err != nil {
		t.Fatalf("cost show --json failed: %v\nOutput: %s", err, stdout)
	}

	var resp costShowResponse
	if unmarshalErr := json.Unmarshal([]byte(stdout), &resp); unmarshalErr != nil {
		t.Fatalf("failed to unmarshal JSON: %v\nOutput: %s", unmarshalErr, stdout)
	}

	// Verify ccusage enrichment fields are present
	if resp.CacheHitRate == nil {
		t.Error("CacheHitRate missing from JSON output")
	} else if *resp.CacheHitRate != 0.8 {
		t.Errorf("CacheHitRate = %f, want 0.8", *resp.CacheHitRate)
	}

	if resp.BurnRate == nil {
		t.Error("BurnRate missing from JSON output")
	} else if *resp.BurnRate != 2.625 {
		t.Errorf("BurnRate = %f, want 2.625", *resp.BurnRate)
	}

	if resp.ProjectedTotal == nil {
		t.Error("ProjectedTotal missing from JSON output")
	}

	if resp.BillingWindowSpent == nil {
		t.Error("BillingWindowSpent missing from JSON output")
	} else if *resp.BillingWindowSpent != 5.25 {
		t.Errorf("BillingWindowSpent = %f, want 5.25", *resp.BillingWindowSpent)
	}

	// Verify totals from ccusage (internal DB empty)
	if resp.TotalCost != 5.25 {
		t.Errorf("TotalCost = %f, want 5.25", resp.TotalCost)
	}
	if resp.TotalInputTokens != 1500 {
		t.Errorf("TotalInputTokens = %d, want 1500", resp.TotalInputTokens)
	}

	// Verify by_model populated from ccusage
	if len(resp.ByModel) != 2 {
		t.Errorf("ByModel has %d entries, want 2", len(resp.ByModel))
	}
}

func TestCostShowJSON_CCUsageUnavailable(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Mock ccusage runner to fail (simulates npx not installed)
	origRunner := ccusageRunner
	defer func() { ccusageRunner = origRunner }()

	ccusageRunner = func(_ context.Context) ([]byte, error) {
		return nil, fmt.Errorf("npx not found")
	}

	// Seed some internal DB records
	store := cost.NewStore(wsDir)
	if openErr := store.Open(); openErr != nil {
		t.Fatalf("failed to open cost store: %v", openErr)
	}
	_, _ = store.Record(context.Background(), "eng-01", "", "claude-opus", 1000, 500, 0.05)
	_ = store.Close()

	stdout, _, err := executeIntegrationCmd("cost", "show", "--json")
	if err != nil {
		t.Fatalf("cost show --json failed: %v\nOutput: %s", err, stdout)
	}

	var resp costShowResponse
	if unmarshalErr := json.Unmarshal([]byte(stdout), &resp); unmarshalErr != nil {
		t.Fatalf("failed to unmarshal JSON: %v\nOutput: %s", unmarshalErr, stdout)
	}

	// Should gracefully degrade — no ccusage fields
	if resp.CacheHitRate != nil {
		t.Error("CacheHitRate should be nil when ccusage unavailable")
	}
	if resp.BurnRate != nil {
		t.Error("BurnRate should be nil when ccusage unavailable")
	}
	if resp.ProjectedTotal != nil {
		t.Error("ProjectedTotal should be nil when ccusage unavailable")
	}
	if resp.BillingWindowSpent != nil {
		t.Error("BillingWindowSpent should be nil when ccusage unavailable")
	}

	// Internal DB data should still be present
	if resp.TotalCost != 0.05 {
		t.Errorf("TotalCost = %f, want 0.05", resp.TotalCost)
	}
	if resp.ByAgent["eng-01"] != 0.05 {
		t.Errorf("ByAgent[eng-01] = %f, want 0.05", resp.ByAgent["eng-01"])
	}
}

func TestCostShowJSON_MixedDBAndCCUsage(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Mock ccusage runner
	origRunner := ccusageRunner
	defer func() { ccusageRunner = origRunner }()

	ccusageRunner = func(_ context.Context) ([]byte, error) {
		return []byte(`{
			"daily": [{"date":"2026-03-01","inputTokens":5000,"outputTokens":25000,"cacheCreationTokens":500,"cacheReadTokens":4500,"totalTokens":35000,"totalCost":15.00,"modelsUsed":["opus"]}],
			"totals": {"inputTokens":5000,"outputTokens":25000,"cacheCreationTokens":500,"cacheReadTokens":4500,"totalTokens":35000,"totalCost":15.00}
		}`), nil
	}

	// Seed internal DB with records
	store := cost.NewStore(wsDir)
	if openErr := store.Open(); openErr != nil {
		t.Fatalf("failed to open cost store: %v", openErr)
	}
	_, _ = store.Record(context.Background(), "eng-01", "", "claude-opus", 1000, 500, 0.05)
	_, _ = store.Record(context.Background(), "eng-02", "", "claude-sonnet", 2000, 1000, 0.03)
	_ = store.Close()

	stdout, _, err := executeIntegrationCmd("cost", "show", "--json")
	if err != nil {
		t.Fatalf("cost show --json failed: %v\nOutput: %s", err, stdout)
	}

	var resp costShowResponse
	if unmarshalErr := json.Unmarshal([]byte(stdout), &resp); unmarshalErr != nil {
		t.Fatalf("failed to unmarshal JSON: %v\nOutput: %s", unmarshalErr, stdout)
	}

	// Internal DB has data — totals should NOT be overridden
	if resp.TotalCost != 0.08 {
		t.Errorf("TotalCost = %f, want 0.08 (from internal DB)", resp.TotalCost)
	}

	// But ccusage enrichment fields should still be present
	if resp.CacheHitRate == nil {
		t.Error("CacheHitRate should be present")
	} else if *resp.CacheHitRate != 0.9 {
		t.Errorf("CacheHitRate = %f, want 0.9", *resp.CacheHitRate)
	}

	if resp.BillingWindowSpent == nil {
		t.Error("BillingWindowSpent should be present")
	} else if *resp.BillingWindowSpent != 15.00 {
		t.Errorf("BillingWindowSpent = %f, want 15.00", *resp.BillingWindowSpent)
	}

	// by_model from internal DB should be preserved (not overridden)
	if len(resp.ByModel) != 2 {
		t.Errorf("ByModel has %d entries, want 2 (from internal DB)", len(resp.ByModel))
	}
}
