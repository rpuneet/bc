package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/server/handlers"
)

// Cost command tests use executeIntegrationCmd which captures os.Stdout
// because cost.go uses fmt.Printf directly rather than cmd.OutOrStdout().
//
// Costs are source-direct: there is no ledger to seed. These tests
// fabricate Claude Code JSONL session transcripts and serve them through
// the REAL cost.Service + REAL /api/costs handlers wired into the
// package-level fake bcd, so the full CLI → daemon → JSONL path is
// exercised without a live daemon.

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

// memBudgetStore is an in-memory cost.BudgetStore for the fake bcd.
type memBudgetStore struct {
	m  map[string]cost.BudgetConfig
	mu sync.Mutex
}

func (s *memBudgetStore) All() (map[string]cost.BudgetConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]cost.BudgetConfig, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out, nil
}

func (s *memBudgetStore) Set(scope string, b cost.BudgetConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[scope] = b
	return nil
}

func (s *memBudgetStore) Delete(scope string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[scope]; !ok {
		return fmt.Errorf("no budget for scope %q", scope)
	}
	delete(s.m, scope)
	return nil
}

// setupCostBcd points the package-level fake bcd at real /api/costs
// handlers backed by a real cost.Service that reads Claude JSONL
// transcripts from an isolated home. Returns the agents dir fixtures go
// into (<home>/agents/<agent>/session/claude/projects/...).
func setupCostBcd(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	agentsDir := filepath.Join(home, "agents")
	if err := os.MkdirAll(agentsDir, 0o750); err != nil {
		t.Fatalf("create agents dir: %v", err)
	}

	svc := cost.NewService(provider.DefaultRegistry, cost.Options{
		Home:      home,
		AgentsDir: agentsDir,
		CacheTTL:  time.Nanosecond, // rescan every request — fixtures are written mid-test
	}, &memBudgetStore{m: make(map[string]cost.BudgetConfig)})

	mux := http.NewServeMux()
	handlers.NewCostHandler(svc).Register(mux)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	setTestBcdHandler(t, mux.ServeHTTP)
	return agentsDir
}

// claudeUsageLine returns one Claude Code JSONL assistant line with usage.
func claudeUsageLine(session, ts, cwd, model string, in, out, cacheW, cacheR int64) string {
	return fmt.Sprintf(`{"type":"assistant","sessionId":%q,"timestamp":%q,"cwd":%q,"message":{"model":%q,"usage":{"input_tokens":%d,"output_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d}}}`,
		session, ts, cwd, model, in, out, cacheW, cacheR)
}

// writeAgentTranscript writes a Claude session transcript for a docker
// agent entity: <agentsDir>/<agent>/session/claude/projects/proj/<file>.
func writeAgentTranscript(t *testing.T, agentsDir, agentName, file string, lines ...string) {
	t.Helper()
	dir := filepath.Join(agentsDir, agentName, "session", "claude", "projects", "proj")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("create transcript dir: %v", err)
	}
	data := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, file), []byte(data), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
}

// writeHostTranscript writes a Claude session transcript under the host
// home: <home>/.claude/projects/proj/<file>.
func writeHostTranscript(t *testing.T, home, file string, lines ...string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", "proj")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("create transcript dir: %v", err)
	}
	data := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, file), []byte(data), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
}

// nowTS is an RFC3339 timestamp inside every budget period (now, UTC).
func nowTS() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// failingCCUsage makes ccusage unavailable so --json tests don't shell
// out to npx. Restores the original runner on cleanup.
func failingCCUsage(t *testing.T) {
	t.Helper()
	orig := ccusageRunner
	ccusageRunner = func(_ context.Context) ([]byte, error) {
		return nil, fmt.Errorf("npx not found")
	}
	t.Cleanup(func() { ccusageRunner = orig })
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// --- cost show (source-direct via daemon API) ---

func TestCostShowEmpty(t *testing.T) {
	setupCostBcd(t)

	stdout, _, err := executeIntegrationCmd("cost", "show")
	if err != nil {
		t.Fatalf("cost show failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "No cost records found") {
		t.Errorf("expected 'No cost records found', got: %s", stdout)
	}
}

func TestCostShowWithRecords(t *testing.T) {
	agentsDir := setupCostBcd(t)

	writeAgentTranscript(t, agentsDir, "engineer-01", "s1.jsonl",
		claudeUsageLine("s1", nowTS(), "/repo/a", "claude-sonnet-4-20250514", 1000, 500, 0, 0))
	writeAgentTranscript(t, agentsDir, "engineer-02", "s2.jsonl",
		claudeUsageLine("s2", nowTS(), "/repo/b", "claude-sonnet-4-20250514", 2000, 1000, 0, 0))

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
	if !strings.Contains(stdout, "AGENT") {
		t.Errorf("output should contain per-agent table header: %s", stdout)
	}
}

func TestCostShowByAgent(t *testing.T) {
	agentsDir := setupCostBcd(t)

	// engineer-01: 1000 in + 2000 out on sonnet-4 ($3/M in, $15/M out)
	// = 0.003 + 0.030 = $0.0330
	writeAgentTranscript(t, agentsDir, "engineer-01", "s1.jsonl",
		claudeUsageLine("s1", nowTS(), "/repo/a", "claude-sonnet-4-20250514", 1000, 2000, 0, 0))
	writeAgentTranscript(t, agentsDir, "engineer-02", "s2.jsonl",
		claudeUsageLine("s2", nowTS(), "/repo/b", "claude-sonnet-4-20250514", 500000, 500000, 0, 0))

	stdout, _, err := executeIntegrationCmd("cost", "show", "engineer-01")
	if err != nil {
		t.Fatalf("cost show agent failed: %v\nOutput: %s", err, stdout)
	}
	// The agent detail total must reflect only engineer-01's spend.
	if !strings.Contains(stdout, "$0.0330") {
		t.Errorf("expected engineer-01 total $0.0330, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 records") {
		t.Errorf("expected 1 record for engineer-01, got: %s", stdout)
	}
}

func TestCostShowNonExistentAgent(t *testing.T) {
	setupCostBcd(t)
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

func TestCostShowNegativeLimit(t *testing.T) {
	setupCostBcd(t)
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
	setupCostBcd(t)
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
	agentsDir := setupCostBcd(t)
	failingCCUsage(t)
	resetCostFlags()
	defer resetCostFlags()

	// sonnet-4 pricing: 1000 in ($0.003) + 500 out ($0.0075) = $0.0105
	writeAgentTranscript(t, agentsDir, "engineer-01", "s1.jsonl",
		claudeUsageLine("s1", nowTS(), "/repo/a", "claude-sonnet-4-20250514", 1000, 500, 0, 0))

	stdout, _, err := executeIntegrationCmd("cost", "show", "--json")
	if err != nil {
		t.Fatalf("cost show --json failed: %v\nOutput: %s", err, stdout)
	}

	var resp costShowResponse
	if unmarshalErr := json.Unmarshal([]byte(stdout), &resp); unmarshalErr != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", unmarshalErr, stdout)
	}
	if !approxEqual(resp.ByAgent["engineer-01"], 0.0105) {
		t.Errorf("ByAgent[engineer-01] = %f, want 0.0105", resp.ByAgent["engineer-01"])
	}
	if !approxEqual(resp.TotalCost, 0.0105) {
		t.Errorf("TotalCost = %f, want 0.0105", resp.TotalCost)
	}
	if _, ok := resp.ByModel["claude-sonnet-4-20250514"]; !ok {
		t.Errorf("ByModel missing claude-sonnet-4-20250514: %v", resp.ByModel)
	}
	if resp.TotalInputTokens != 1000 || resp.TotalOutputTokens != 500 {
		t.Errorf("tokens = %d/%d, want 1000/500", resp.TotalInputTokens, resp.TotalOutputTokens)
	}
}

func TestCostShowJSON_HostSessionAttribution(t *testing.T) {
	agentsDir := setupCostBcd(t)
	failingCCUsage(t)

	// Host (tmux) sessions live under <home>/.claude/projects and are
	// attributed by CWD basename.
	home := filepath.Dir(agentsDir)
	writeHostTranscript(t, home, "host.jsonl",
		claudeUsageLine("h1", nowTS(), "/work/myrepo", "claude-sonnet-4-20250514", 100, 50, 0, 0))

	stdout, _, err := executeIntegrationCmd("cost", "show", "--json")
	if err != nil {
		t.Fatalf("cost show --json failed: %v\nOutput: %s", err, stdout)
	}
	var resp costShowResponse
	if unmarshalErr := json.Unmarshal([]byte(stdout), &resp); unmarshalErr != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", unmarshalErr, stdout)
	}
	if _, ok := resp.ByAgent["myrepo"]; !ok {
		t.Errorf("host session should be attributed to CWD basename 'myrepo': %v", resp.ByAgent)
	}
	// sonnet-4: 100 in + 50 out = 0.0003 + 0.00075 = 0.00105
	if !approxEqual(resp.TotalCost, 0.00105) {
		t.Errorf("TotalCost = %f, want 0.00105", resp.TotalCost)
	}
}

// --- Budget tests ---

func TestCostBudgetSetWorkspace(t *testing.T) {
	setupCostBcd(t)
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
	setupCostBcd(t)
	resetBudgetFlags()
	defer resetBudgetFlags()

	stdout, _, err := executeIntegrationCmd("cost", "budget", "set", "50.00", "--agent", "eng-01")
	if err != nil {
		t.Fatalf("cost budget set --agent failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "50.00") {
		t.Errorf("expected budget amount: %s", stdout)
	}
	if !strings.Contains(stdout, "agent:eng-01") {
		t.Errorf("expected agent scope in output: %s", stdout)
	}
}

func TestCostBudgetSetTeam(t *testing.T) {
	setupCostBcd(t)
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
	setupCostBcd(t)

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
	setupCostBcd(t)
	resetBudgetFlags()
	defer resetBudgetFlags()

	_, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00", "--period", "yearly")
	if err == nil {
		t.Error("expected error for invalid period")
	}
}

func TestCostBudgetSetAlertAt(t *testing.T) {
	setupCostBcd(t)
	resetBudgetFlags()
	defer resetBudgetFlags()

	stdout, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00", "--alert-at", "0.9")
	if err != nil {
		t.Fatalf("cost budget set --alert-at failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "90%") {
		t.Errorf("expected 90%% alert threshold in output: %s", stdout)
	}
}

func TestCostBudgetSetAlertAtInvalid(t *testing.T) {
	setupCostBcd(t)

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
	setupCostBcd(t)
	resetBudgetFlags()
	defer resetBudgetFlags()

	stdout, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00", "--hard-stop")
	if err != nil {
		t.Fatalf("cost budget set --hard-stop failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "Hard stop: true") {
		t.Errorf("expected hard stop enabled in output: %s", stdout)
	}
}

func TestCostBudgetSetZeroAmount(t *testing.T) {
	setupCostBcd(t)
	resetBudgetFlags()
	defer resetBudgetFlags()

	_, _, err := executeIntegrationCmd("cost", "budget", "set", "0")
	if err == nil {
		t.Error("expected error for zero budget")
	}
}

func TestCostBudgetSetNegativeAmount(t *testing.T) {
	setupCostBcd(t)
	resetBudgetFlags()
	defer resetBudgetFlags()

	_, _, err := executeIntegrationCmd("cost", "budget", "set", "-50.00")
	if err == nil {
		t.Error("expected error for negative budget")
	}
}

func TestCostBudgetSetInvalidAmount(t *testing.T) {
	setupCostBcd(t)
	resetBudgetFlags()
	defer resetBudgetFlags()

	_, _, err := executeIntegrationCmd("cost", "budget", "set", "abc")
	if err == nil {
		t.Error("expected error for non-numeric budget")
	}
}

func TestCostBudgetShowNoBudgets(t *testing.T) {
	setupCostBcd(t)
	resetBudgetFlags()
	defer resetBudgetFlags()

	// No budget configured for the workspace scope: the daemon answers
	// 404 and the CLI surfaces it.
	_, _, err := executeIntegrationCmd("cost", "budget", "show")
	if err == nil {
		t.Fatal("expected error when no budget is configured")
	}
	if !strings.Contains(err.Error(), "no budget configured") {
		t.Errorf("expected 'no budget configured' error, got: %v", err)
	}
}

func TestCostBudgetShowWorkspace(t *testing.T) {
	setupCostBcd(t)
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
	if !strings.Contains(stdout, "workspace") {
		t.Errorf("expected workspace scope in show output: %s", stdout)
	}
}

func TestCostBudgetShowWithSpending(t *testing.T) {
	agentsDir := setupCostBcd(t)
	resetBudgetFlags()
	defer resetBudgetFlags()

	// Set budget
	_, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00")
	if err != nil {
		t.Fatalf("failed to set budget: %v", err)
	}

	// Spend $25: claude-fable-5 is $10/M input → 2.5M input tokens.
	writeAgentTranscript(t, agentsDir, "eng-01", "s1.jsonl",
		claudeUsageLine("s1", nowTS(), "/repo/a", "claude-fable-5", 2_500_000, 0, 0, 0))

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
	agentsDir := setupCostBcd(t)
	resetBudgetFlags()
	defer resetBudgetFlags()

	// Set budget with 80% alert
	_, _, err := executeIntegrationCmd("cost", "budget", "set", "100.00", "--alert-at", "0.8")
	if err != nil {
		t.Fatalf("failed to set budget: %v", err)
	}

	// Spend $85: 8.5M input tokens on claude-fable-5 ($10/M input).
	writeAgentTranscript(t, agentsDir, "eng-01", "s1.jsonl",
		claudeUsageLine("s1", nowTS(), "/repo/a", "claude-fable-5", 8_500_000, 0, 0, 0))

	resetBudgetFlags()
	stdout, _, err := executeIntegrationCmd("cost", "budget", "show")
	if err != nil {
		t.Fatalf("cost budget show failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "Near limit") {
		t.Errorf("expected near-limit warning in output: %s", stdout)
	}
	if !strings.Contains(stdout, "85.00") {
		t.Errorf("expected current spend in output: %s", stdout)
	}
}

func TestCostBudgetShowOverBudget(t *testing.T) {
	agentsDir := setupCostBcd(t)
	resetBudgetFlags()
	defer resetBudgetFlags()

	// Set small budget
	_, _, err := executeIntegrationCmd("cost", "budget", "set", "10.00")
	if err != nil {
		t.Fatalf("failed to set budget: %v", err)
	}

	// Spend $15: 1.5M input tokens on claude-fable-5 ($10/M input).
	writeAgentTranscript(t, agentsDir, "eng-01", "s1.jsonl",
		claudeUsageLine("s1", nowTS(), "/repo/a", "claude-fable-5", 1_500_000, 0, 0, 0))

	resetBudgetFlags()
	stdout, _, err := executeIntegrationCmd("cost", "budget", "show")
	if err != nil {
		t.Fatalf("cost budget show failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "OVER BUDGET") {
		t.Errorf("expected over-budget warning in output: %s", stdout)
	}
	if !strings.Contains(stdout, "15.00") {
		t.Errorf("expected current spend in output: %s", stdout)
	}
}

func TestCostBudgetDelete(t *testing.T) {
	setupCostBcd(t)
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
	setupCostBcd(t)
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

	// Verify deleted: show now reports no budget for the scope
	resetBudgetFlags()
	_, _, err = executeIntegrationCmd("cost", "budget", "show", "--agent", "eng-01")
	if err == nil {
		t.Fatal("expected no-budget error after delete")
	}
	if !strings.Contains(err.Error(), "no budget configured") {
		t.Errorf("expected 'no budget configured' error after delete, got: %v", err)
	}
}

func TestCostBudgetUpdateExisting(t *testing.T) {
	setupCostBcd(t)
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
	setupCostBcd(t)
	resetBudgetFlags()
	defer resetBudgetFlags()

	_, _, err := executeIntegrationCmd("cost", "budget", "set")
	if err == nil {
		t.Error("expected error for missing amount")
	}
}

func TestCostBudgetShowAgentSpecific(t *testing.T) {
	setupCostBcd(t)
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
	setupCostBcd(t)
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

// --- mycel cost report (source-direct, no daemon) ---

func TestCostReportByRepo(t *testing.T) {
	// cost report builds its own cost.Service from MYCEL_HOME's agents
	// dir plus the user home — no daemon involved. Isolate both.
	home := t.TempDir()
	t.Setenv("MYCEL_HOME", home)
	t.Setenv("HOME", t.TempDir()) // keep the real ~/.claude out of the scan

	agentsDir := filepath.Join(home, "agents")
	if err := os.MkdirAll(agentsDir, 0o750); err != nil {
		t.Fatalf("create agents dir: %v", err)
	}
	// builder worked in /work/repo-a: 1000 in + 2000 out on sonnet-4
	// = $0.0330
	writeAgentTranscript(t, agentsDir, "builder", "s1.jsonl",
		claudeUsageLine("s1", nowTS(), "/work/repo-a", "claude-sonnet-4-20250514", 1000, 2000, 0, 0))

	stdout, _, err := executeIntegrationCmd("cost", "report")
	if err != nil {
		t.Fatalf("cost report failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "repo-a") {
		t.Errorf("expected repo label 'repo-a' in output: %s", stdout)
	}
	if !strings.Contains(stdout, "/work/repo-a") {
		t.Errorf("expected repo path in output: %s", stdout)
	}
	if !strings.Contains(stdout, "$0.0330") {
		t.Errorf("expected sonnet-4 priced total $0.0330 in output: %s", stdout)
	}
}

func TestCostReportByProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MYCEL_HOME", home)
	t.Setenv("HOME", t.TempDir())

	agentsDir := filepath.Join(home, "agents")
	if err := os.MkdirAll(agentsDir, 0o750); err != nil {
		t.Fatalf("create agents dir: %v", err)
	}
	// Two agents in the same repo roll up into one project total:
	// (1000 in + 2000 out) + (2000 in + 4000 out) = $0.0330 + $0.0660
	writeAgentTranscript(t, agentsDir, "builder", "s1.jsonl",
		claudeUsageLine("s1", nowTS(), "/work/repo-a", "claude-sonnet-4-20250514", 1000, 2000, 0, 0))
	writeAgentTranscript(t, agentsDir, "tester", "s2.jsonl",
		claudeUsageLine("s2", nowTS(), "/work/repo-a", "claude-sonnet-4-20250514", 2000, 4000, 0, 0))

	stdout, _, err := executeIntegrationCmd("cost", "report", "--by", "project")
	if err != nil {
		t.Fatalf("cost report --by project failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "repo-a") {
		t.Errorf("expected project 'repo-a' in output: %s", stdout)
	}
	if !strings.Contains(stdout, "$0.0990") {
		t.Errorf("expected rolled-up project total $0.0990 in output: %s", stdout)
	}
}

func TestCostReportInvalidBy(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())

	_, _, err := executeIntegrationCmd("cost", "report", "--by", "bogus")
	if err == nil {
		t.Fatal("expected error for unknown --by grouping")
	}
	if !strings.Contains(err.Error(), "unknown --by") {
		t.Errorf("expected unknown --by error, got: %v", err)
	}
}

func TestParseSinceFlag(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"", false},
		{"7d", false},
		{"24h", false},
		{"2026-01-01", false},
		{"2026-01-01T00:00:00Z", false},
		{"bogus", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parseSinceFlag(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSinceFlag(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
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

	// Totals should be overridden from ccusage (internal sources were empty)
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

	// by_model should have models from ccusage (since internal sources were empty)
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

func TestEnrichWithCCUsage_SourcesHaveData(t *testing.T) {
	// When the source-direct scan found data, totals must NOT be
	// overridden by ccusage.
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

	// TotalCost should NOT be overridden since sources had data
	if resp.TotalCost != 0.05 {
		t.Errorf("TotalCost = %f, want 0.05 (should not be overridden)", resp.TotalCost)
	}

	// ByModel should NOT be overridden since sources had data
	if len(resp.ByModel) != 1 {
		t.Errorf("ByModel should keep source data, got %d entries", len(resp.ByModel))
	}

	// ccusage-derived fields should still be set
	if resp.BurnRate == nil {
		t.Fatal("BurnRate should be set even with source data")
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
	setupCostBcd(t) // no fixtures — sources are empty

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

	// Verify totals from ccusage (sources empty)
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
	agentsDir := setupCostBcd(t)
	failingCCUsage(t)

	// Seed a source-direct record: 1000 in + 500 out on sonnet-4 = $0.0105
	writeAgentTranscript(t, agentsDir, "eng-01", "s1.jsonl",
		claudeUsageLine("s1", nowTS(), "/repo/a", "claude-sonnet-4-20250514", 1000, 500, 0, 0))

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

	// Source-direct data should still be present
	if !approxEqual(resp.TotalCost, 0.0105) {
		t.Errorf("TotalCost = %f, want 0.0105", resp.TotalCost)
	}
	if !approxEqual(resp.ByAgent["eng-01"], 0.0105) {
		t.Errorf("ByAgent[eng-01] = %f, want 0.0105", resp.ByAgent["eng-01"])
	}
}

func TestCostShowJSON_MixedSourcesAndCCUsage(t *testing.T) {
	agentsDir := setupCostBcd(t)

	// Mock ccusage runner
	origRunner := ccusageRunner
	defer func() { ccusageRunner = origRunner }()

	ccusageRunner = func(_ context.Context) ([]byte, error) {
		return []byte(`{
			"daily": [{"date":"2026-03-01","inputTokens":5000,"outputTokens":25000,"cacheCreationTokens":500,"cacheReadTokens":4500,"totalTokens":35000,"totalCost":15.00,"modelsUsed":["opus"]}],
			"totals": {"inputTokens":5000,"outputTokens":25000,"cacheCreationTokens":500,"cacheReadTokens":4500,"totalTokens":35000,"totalCost":15.00}
		}`), nil
	}

	// Seed source-direct records on two models:
	//   eng-01 opus-4: 1000 in ($0.015) + 500 out ($0.0375) = $0.0525
	//   eng-02 sonnet-4: 2000 in ($0.006) + 1000 out ($0.015) = $0.0210
	writeAgentTranscript(t, agentsDir, "eng-01", "s1.jsonl",
		claudeUsageLine("s1", nowTS(), "/repo/a", "claude-opus-4-20250514", 1000, 500, 0, 0))
	writeAgentTranscript(t, agentsDir, "eng-02", "s2.jsonl",
		claudeUsageLine("s2", nowTS(), "/repo/b", "claude-sonnet-4-20250514", 2000, 1000, 0, 0))

	stdout, _, err := executeIntegrationCmd("cost", "show", "--json")
	if err != nil {
		t.Fatalf("cost show --json failed: %v\nOutput: %s", err, stdout)
	}

	var resp costShowResponse
	if unmarshalErr := json.Unmarshal([]byte(stdout), &resp); unmarshalErr != nil {
		t.Fatalf("failed to unmarshal JSON: %v\nOutput: %s", unmarshalErr, stdout)
	}

	// Sources have data — totals should NOT be overridden by ccusage
	if !approxEqual(resp.TotalCost, 0.0735) {
		t.Errorf("TotalCost = %f, want 0.0735 (from sources)", resp.TotalCost)
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

	// by_model from the source scan should be preserved (not overridden)
	if len(resp.ByModel) != 2 {
		t.Errorf("ByModel has %d entries, want 2 (from sources)", len(resp.ByModel))
	}
}
