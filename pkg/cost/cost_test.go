package cost

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	// Use a MYCEL_HOME under t.TempDir so the global-state-dir branch
	// resolves deterministically.
	bcHome := t.TempDir()
	t.Setenv("MYCEL_HOME", bcHome)

	store := NewStore("/tmp/test")
	if store == nil {
		t.Fatal("NewStore returned nil")
	}
	// Path should now be under ~/.bc/workspaces/<id>/costs.db (M11+).
	if !strings.Contains(store.path, "/workspaces/") {
		t.Errorf("path %q should contain '/workspaces/' segment", store.path)
	}
	if !strings.HasSuffix(store.path, "/costs.db") {
		t.Errorf("path %q should end with '/costs.db'", store.path)
	}
}

func TestStoreOpenClose(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if store.DB() == nil {
		t.Error("DB() should not be nil after Open")
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestStoreRecord(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	record, err := store.Record(context.Background(), "engineer-01", "team-a", "claude-3-opus", 1000, 500, 0.05)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	if record.AgentID != "engineer-01" {
		t.Errorf("AgentID = %q, want %q", record.AgentID, "engineer-01")
	}
	if record.TeamID != "team-a" {
		t.Errorf("TeamID = %q, want %q", record.TeamID, "team-a")
	}
	if record.Model != "claude-3-opus" {
		t.Errorf("Model = %q, want %q", record.Model, "claude-3-opus")
	}
	if record.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want %d", record.InputTokens, 1000)
	}
	if record.OutputTokens != 500 {
		t.Errorf("OutputTokens = %d, want %d", record.OutputTokens, 500)
	}
	if record.TotalTokens != 1500 {
		t.Errorf("TotalTokens = %d, want %d", record.TotalTokens, 1500)
	}
	if record.CostUSD != 0.05 {
		t.Errorf("CostUSD = %f, want %f", record.CostUSD, 0.05)
	}
	if record.ID == 0 {
		t.Error("ID should not be 0")
	}
	if record.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestStoreRecordNoTeam(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	record, err := store.Record(context.Background(), "engineer-01", "", "claude-3-sonnet", 500, 250, 0.01)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	if record.TeamID != "" {
		t.Errorf("TeamID = %q, want empty", record.TeamID)
	}
}

func TestStoreGetByID(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	record, _ := store.Record(context.Background(), "engineer-01", "team-a", "claude-3-opus", 1000, 500, 0.05)

	got, err := store.GetByID(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.AgentID != "engineer-01" {
		t.Errorf("AgentID = %q, want %q", got.AgentID, "engineer-01")
	}
}

func TestStoreGetByIDNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	got, err := store.GetByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("GetByID should not error: %v", err)
	}
	if got != nil {
		t.Error("GetByID should return nil for non-existent ID")
	}
}

func TestStoreGetByAgent(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Record(context.Background(), "engineer-01", "", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "engineer-01", "", "model-b", 200, 100, 0.02)
	_, _ = store.Record(context.Background(), "engineer-02", "", "model-a", 300, 150, 0.03)

	records, err := store.GetByAgent(context.Background(), "engineer-01", 10)
	if err != nil {
		t.Fatalf("GetByAgent failed: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("len(records) = %d, want 2", len(records))
	}
}

func TestStoreGetByTeam(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Record(context.Background(), "engineer-01", "team-a", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "engineer-02", "team-a", "model-a", 200, 100, 0.02)
	_, _ = store.Record(context.Background(), "engineer-03", "team-b", "model-a", 300, 150, 0.03)

	records, err := store.GetByTeam(context.Background(), "team-a", 10)
	if err != nil {
		t.Fatalf("GetByTeam failed: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("len(records) = %d, want 2", len(records))
	}
}

func TestStoreGetAll(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "agent-2", "", "model-a", 200, 100, 0.02)
	_, _ = store.Record(context.Background(), "agent-3", "", "model-a", 300, 150, 0.03)

	records, err := store.GetAll(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("len(records) = %d, want 3", len(records))
	}
}

func TestStoreSummaryByAgent(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Record(context.Background(), "engineer-01", "", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "engineer-01", "", "model-a", 200, 100, 0.02)
	_, _ = store.Record(context.Background(), "engineer-02", "", "model-a", 300, 150, 0.03)

	summaries, err := store.SummaryByAgent(context.Background())
	if err != nil {
		t.Fatalf("SummaryByAgent failed: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("len(summaries) = %d, want 2", len(summaries))
	}

	// Find engineer-01 summary
	var eng01 *Summary
	for _, s := range summaries {
		if s.AgentID == "engineer-01" {
			eng01 = s
			break
		}
	}
	if eng01 == nil {
		t.Fatal("engineer-01 not found in summaries")
	}
	if eng01.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", eng01.InputTokens)
	}
	if eng01.TotalCostUSD != 0.03 {
		t.Errorf("TotalCostUSD = %f, want 0.03", eng01.TotalCostUSD)
	}
	if eng01.RecordCount != 2 {
		t.Errorf("RecordCount = %d, want 2", eng01.RecordCount)
	}
}

func TestStoreSummaryByTeam(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Record(context.Background(), "agent-1", "team-a", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "agent-2", "team-a", "model-a", 200, 100, 0.02)
	_, _ = store.Record(context.Background(), "agent-3", "team-b", "model-a", 300, 150, 0.03)
	_, _ = store.Record(context.Background(), "agent-4", "", "model-a", 400, 200, 0.04) // No team

	summaries, err := store.SummaryByTeam(context.Background())
	if err != nil {
		t.Fatalf("SummaryByTeam failed: %v", err)
	}
	if len(summaries) != 2 {
		t.Errorf("len(summaries) = %d, want 2 (records without team should be excluded)", len(summaries))
	}
}

func TestStoreSummaryByModel(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Record(context.Background(), "agent-1", "", "claude-3-opus", 1000, 500, 0.10)
	_, _ = store.Record(context.Background(), "agent-2", "", "claude-3-opus", 2000, 1000, 0.20)
	_, _ = store.Record(context.Background(), "agent-3", "", "claude-3-sonnet", 500, 250, 0.01)

	summaries, err := store.SummaryByModel(context.Background())
	if err != nil {
		t.Fatalf("SummaryByModel failed: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("len(summaries) = %d, want 2", len(summaries))
	}

	// Should be sorted by cost DESC
	if summaries[0].Model != "claude-3-opus" {
		t.Errorf("First model = %q, want claude-3-opus (highest cost)", summaries[0].Model)
	}
}

func TestStoreWorkspaceSummary(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "agent-2", "", "model-b", 200, 100, 0.02)
	_, _ = store.Record(context.Background(), "agent-3", "", "model-c", 300, 150, 0.03)

	summary, err := store.WorkspaceSummary(context.Background())
	if err != nil {
		t.Fatalf("WorkspaceSummary failed: %v", err)
	}

	if summary.InputTokens != 600 {
		t.Errorf("InputTokens = %d, want 600", summary.InputTokens)
	}
	if summary.OutputTokens != 300 {
		t.Errorf("OutputTokens = %d, want 300", summary.OutputTokens)
	}
	if summary.TotalTokens != 900 {
		t.Errorf("TotalTokens = %d, want 900", summary.TotalTokens)
	}
	if summary.TotalCostUSD != 0.06 {
		t.Errorf("TotalCostUSD = %f, want 0.06", summary.TotalCostUSD)
	}
	if summary.RecordCount != 3 {
		t.Errorf("RecordCount = %d, want 3", summary.RecordCount)
	}
}

func TestStoreWorkspaceSummaryEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	summary, err := store.WorkspaceSummary(context.Background())
	if err != nil {
		t.Fatalf("WorkspaceSummary failed: %v", err)
	}

	if summary.TotalCostUSD != 0 {
		t.Errorf("TotalCostUSD = %f, want 0", summary.TotalCostUSD)
	}
	if summary.RecordCount != 0 {
		t.Errorf("RecordCount = %d, want 0", summary.RecordCount)
	}
}

func TestStoreAgentSummary(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Record(context.Background(), "engineer-01", "", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "engineer-01", "", "model-b", 200, 100, 0.02)
	_, _ = store.Record(context.Background(), "engineer-02", "", "model-a", 300, 150, 0.03)

	summary, err := store.AgentSummary(context.Background(), "engineer-01")
	if err != nil {
		t.Fatalf("AgentSummary failed: %v", err)
	}

	if summary.AgentID != "engineer-01" {
		t.Errorf("AgentID = %q, want engineer-01", summary.AgentID)
	}
	if summary.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", summary.InputTokens)
	}
	if summary.TotalCostUSD != 0.03 {
		t.Errorf("TotalCostUSD = %f, want 0.03", summary.TotalCostUSD)
	}
	if summary.RecordCount != 2 {
		t.Errorf("RecordCount = %d, want 2", summary.RecordCount)
	}
}

func TestStoreTeamSummary(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Record(context.Background(), "agent-1", "team-a", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "agent-2", "team-a", "model-a", 200, 100, 0.02)
	_, _ = store.Record(context.Background(), "agent-3", "team-b", "model-a", 300, 150, 0.03)

	summary, err := store.TeamSummary(context.Background(), "team-a")
	if err != nil {
		t.Fatalf("TeamSummary failed: %v", err)
	}

	if summary.TeamID != "team-a" {
		t.Errorf("TeamID = %q, want team-a", summary.TeamID)
	}
	if summary.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", summary.InputTokens)
	}
	if summary.TotalCostUSD != 0.03 {
		t.Errorf("TotalCostUSD = %f, want 0.03", summary.TotalCostUSD)
	}
	if summary.RecordCount != 2 {
		t.Errorf("RecordCount = %d, want 2", summary.RecordCount)
	}
}

func TestStoreClear(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "agent-2", "", "model-a", 200, 100, 0.02)

	if err := store.Clear(context.Background()); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	records, err := store.GetAll(context.Background(), 100)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("len(records) = %d, want 0 after Clear", len(records))
	}
}

func TestRecordStruct(t *testing.T) {
	r := Record{
		ID:          1,
		TotalTokens: 1500,
	}

	if r.ID != 1 {
		t.Errorf("ID = %d, want 1", r.ID)
	}
	if r.TotalTokens != 1500 {
		t.Errorf("TotalTokens = %d, want 1500", r.TotalTokens)
	}
}

func TestSummaryStruct(t *testing.T) {
	s := Summary{
		AgentID:     "agent-1",
		RecordCount: 10,
	}

	if s.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want agent-1", s.AgentID)
	}
	if s.RecordCount != 10 {
		t.Errorf("RecordCount = %d, want 10", s.RecordCount)
	}
}

func TestGetDailyCosts(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Add some records
	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "agent-2", "", "model-a", 200, 100, 0.02)

	// Get daily costs for the last day
	dailyCosts, err := store.GetDailyCosts(context.Background(), time.Now().AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("GetDailyCosts failed: %v", err)
	}

	// Should have at least one day of data
	if len(dailyCosts) < 1 {
		t.Error("expected at least 1 day of cost data")
	}
}

func TestGetDailyCostsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Get daily costs with no data
	dailyCosts, err := store.GetDailyCosts(context.Background(), time.Now().AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("GetDailyCosts failed: %v", err)
	}

	if len(dailyCosts) != 0 {
		t.Errorf("expected 0 days, got %d", len(dailyCosts))
	}
}

func TestGetAgentDailyCosts(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Add records for multiple agents
	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "agent-2", "", "model-a", 200, 100, 0.02)

	// Get agent daily costs
	agentCosts, err := store.GetAgentDailyCosts(context.Background(), time.Now().AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("GetAgentDailyCosts failed: %v", err)
	}

	// Should have data for both agents
	if len(agentCosts) < 2 {
		t.Errorf("expected at least 2 agent cost entries, got %d", len(agentCosts))
	}
}

func TestGetSummarySince(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "agent-2", "", "model-a", 200, 100, 0.02)

	summary, err := store.GetSummarySince(context.Background(), time.Now().AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("GetSummarySince failed: %v", err)
	}

	if summary.RecordCount != 2 {
		t.Errorf("RecordCount = %d, want 2", summary.RecordCount)
	}
	if summary.TotalCostUSD != 0.03 {
		t.Errorf("TotalCostUSD = %f, want 0.03", summary.TotalCostUSD)
	}
}

func TestGetAgentSummarySince(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 0.01)
	_, _ = store.Record(context.Background(), "agent-2", "", "model-a", 200, 100, 0.02)

	summaries, err := store.GetAgentSummarySince(context.Background(), time.Now().AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("GetAgentSummarySince failed: %v", err)
	}

	if len(summaries) != 2 {
		t.Errorf("expected 2 agents, got %d", len(summaries))
	}
}

func TestProjectCost(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Add some records
	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 0.10)
	_, _ = store.Record(context.Background(), "agent-2", "", "model-a", 200, 100, 0.20)

	// Project for 7 days based on 7 days of history
	proj, err := store.ProjectCost(context.Background(), 7, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("ProjectCost failed: %v", err)
	}

	// Should have 1 day of data
	if proj.DaysAnalyzed < 1 {
		t.Errorf("DaysAnalyzed = %d, expected >= 1", proj.DaysAnalyzed)
	}

	// Total historical should be approximately 0.30
	if proj.TotalHistorical < 0.29 || proj.TotalHistorical > 0.31 {
		t.Errorf("TotalHistorical = %f, want ~0.30", proj.TotalHistorical)
	}
}

func TestProjectCostEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Project with no data
	proj, err := store.ProjectCost(context.Background(), 7, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("ProjectCost failed: %v", err)
	}

	if proj.DaysAnalyzed != 0 {
		t.Errorf("DaysAnalyzed = %d, want 0", proj.DaysAnalyzed)
	}
	if proj.ProjectedCost != 0 {
		t.Errorf("ProjectedCost = %f, want 0", proj.ProjectedCost)
	}
}

func TestDailyCostStruct(t *testing.T) {
	dc := DailyCost{
		Date:    "2026-01-01",
		CostUSD: 1.50,
	}

	if dc.Date != "2026-01-01" {
		t.Errorf("Date = %q, want 2026-01-01", dc.Date)
	}
	if dc.CostUSD != 1.50 {
		t.Errorf("CostUSD = %f, want 1.50", dc.CostUSD)
	}
}

func TestProjectionStruct(t *testing.T) {
	proj := Projection{
		DailyAvgCost:  1.00,
		ProjectedCost: 7.00,
		DaysAnalyzed:  7,
	}

	if proj.DailyAvgCost != 1.00 {
		t.Errorf("DailyAvgCost = %f, want 1.00", proj.DailyAvgCost)
	}
	if proj.ProjectedCost != 7.00 {
		t.Errorf("ProjectedCost = %f, want 7.00", proj.ProjectedCost)
	}
	if proj.DaysAnalyzed != 7 {
		t.Errorf("DaysAnalyzed = %d, want 7", proj.DaysAnalyzed)
	}
}

func TestSetBudget(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	budget, err := store.SetBudget(context.Background(), "workspace", BudgetPeriodDaily, 100.0, 0.8, true)
	if err != nil {
		t.Fatalf("SetBudget failed: %v", err)
	}

	if budget == nil {
		t.Fatal("SetBudget returned nil budget")
	}
	if budget.Scope != "workspace" {
		t.Errorf("Scope = %q, want workspace", budget.Scope)
	}
	if budget.Period != BudgetPeriodDaily {
		t.Errorf("Period = %q, want daily", budget.Period)
	}
	if budget.LimitUSD != 100.0 {
		t.Errorf("LimitUSD = %f, want 100.0", budget.LimitUSD)
	}
	if budget.AlertAt != 0.8 {
		t.Errorf("AlertAt = %f, want 0.8", budget.AlertAt)
	}
	if !budget.HardStop {
		t.Error("HardStop should be true")
	}
}

func TestSetBudgetUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create initial budget
	_, err := store.SetBudget(context.Background(), "agent:eng-01", BudgetPeriodWeekly, 50.0, 0.7, false)
	if err != nil {
		t.Fatalf("SetBudget failed: %v", err)
	}

	// Update the budget
	budget, err := store.SetBudget(context.Background(), "agent:eng-01", BudgetPeriodMonthly, 200.0, 0.9, true)
	if err != nil {
		t.Fatalf("SetBudget update failed: %v", err)
	}

	if budget.Period != BudgetPeriodMonthly {
		t.Errorf("Period = %q, want monthly", budget.Period)
	}
	if budget.LimitUSD != 200.0 {
		t.Errorf("LimitUSD = %f, want 200.0", budget.LimitUSD)
	}
	if !budget.HardStop {
		t.Error("HardStop should be true after update")
	}
}

func TestGetBudget(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create a budget
	_, err := store.SetBudget(context.Background(), "team:backend", BudgetPeriodWeekly, 500.0, 0.75, false)
	if err != nil {
		t.Fatalf("SetBudget failed: %v", err)
	}

	// Get the budget
	budget, err := store.GetBudget(context.Background(), "team:backend")
	if err != nil {
		t.Fatalf("GetBudget failed: %v", err)
	}

	if budget == nil {
		t.Fatal("GetBudget returned nil")
	}
	if budget.Scope != "team:backend" {
		t.Errorf("Scope = %q, want team:backend", budget.Scope)
	}
	if budget.LimitUSD != 500.0 {
		t.Errorf("LimitUSD = %f, want 500.0", budget.LimitUSD)
	}
}

func TestGetBudgetNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	budget, err := store.GetBudget(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetBudget should not error for missing budget: %v", err)
	}
	if budget != nil {
		t.Error("GetBudget should return nil for missing budget")
	}
}

func TestGetAllBudgets(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create multiple budgets
	_, _ = store.SetBudget(context.Background(), "workspace", BudgetPeriodMonthly, 1000.0, 0.8, true)
	_, _ = store.SetBudget(context.Background(), "agent:eng-01", BudgetPeriodDaily, 50.0, 0.9, false)
	_, _ = store.SetBudget(context.Background(), "team:backend", BudgetPeriodWeekly, 300.0, 0.7, false)

	budgets, err := store.GetAllBudgets(context.Background())
	if err != nil {
		t.Fatalf("GetAllBudgets failed: %v", err)
	}

	if len(budgets) != 3 {
		t.Errorf("len(budgets) = %d, want 3", len(budgets))
	}
}

func TestGetAllBudgetsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	budgets, err := store.GetAllBudgets(context.Background())
	if err != nil {
		t.Fatalf("GetAllBudgets failed: %v", err)
	}

	if len(budgets) != 0 {
		t.Errorf("len(budgets) = %d, want 0", len(budgets))
	}
}

func TestDeleteBudget(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create a budget
	_, err := store.SetBudget(context.Background(), "workspace", BudgetPeriodDaily, 100.0, 0.8, false)
	if err != nil {
		t.Fatalf("SetBudget failed: %v", err)
	}

	// Delete it
	err = store.DeleteBudget(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("DeleteBudget failed: %v", err)
	}

	// Verify it's gone
	budget, err := store.GetBudget(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("GetBudget failed: %v", err)
	}
	if budget != nil {
		t.Error("Budget should be deleted")
	}
}

func TestDeleteBudgetNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	err := store.DeleteBudget(context.Background(), "nonexistent")
	if err == nil {
		t.Error("DeleteBudget should fail for nonexistent budget")
	}
}

func TestCheckBudget(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create a budget
	_, err := store.SetBudget(context.Background(), "workspace", BudgetPeriodDaily, 100.0, 0.8, true)
	if err != nil {
		t.Fatalf("SetBudget failed: %v", err)
	}

	// Add some cost records
	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 50.0)
	_, _ = store.Record(context.Background(), "agent-2", "", "model-a", 200, 100, 30.0)

	// Check budget status
	status, err := store.CheckBudget(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("CheckBudget failed: %v", err)
	}

	if status == nil {
		t.Fatal("CheckBudget returned nil status")
	}
	if status.Budget == nil {
		t.Fatal("CheckBudget status has nil Budget")
	}
	if status.CurrentSpend != 80.0 {
		t.Errorf("CurrentSpend = %f, want 80.0", status.CurrentSpend)
	}
	if status.Remaining != 20.0 {
		t.Errorf("Remaining = %f, want 20.0", status.Remaining)
	}
	if status.PercentUsed != 0.8 {
		t.Errorf("PercentUsed = %f, want 0.8", status.PercentUsed)
	}
	if status.IsOverBudget {
		t.Error("Should not be over budget")
	}
	if !status.IsNearLimit {
		t.Error("Should be near limit (80% >= 80% alert threshold)")
	}
}

func TestCheckBudgetNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	status, err := store.CheckBudget(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("CheckBudget should not error for missing budget: %v", err)
	}
	if status != nil {
		t.Error("CheckBudget should return nil for missing budget")
	}
}

func TestCheckBudgetOverLimit(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create a small budget
	_, err := store.SetBudget(context.Background(), "workspace", BudgetPeriodDaily, 10.0, 0.5, true)
	if err != nil {
		t.Fatalf("SetBudget failed: %v", err)
	}

	// Exceed the budget
	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 15.0)

	status, err := store.CheckBudget(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("CheckBudget failed: %v", err)
	}

	if !status.IsOverBudget {
		t.Error("Should be over budget")
	}
	// Remaining is clamped to 0 when over budget
	if status.Remaining != 0 {
		t.Errorf("Remaining = %f, want 0 (clamped)", status.Remaining)
	}
	if status.PercentUsed < 1.0 {
		t.Errorf("PercentUsed = %f, want >= 1.0", status.PercentUsed)
	}
}

func TestBudgetPeriodConstants(t *testing.T) {
	if BudgetPeriodDaily != "daily" {
		t.Errorf("BudgetPeriodDaily = %q, want daily", BudgetPeriodDaily)
	}
	if BudgetPeriodWeekly != "weekly" {
		t.Errorf("BudgetPeriodWeekly = %q, want weekly", BudgetPeriodWeekly)
	}
	if BudgetPeriodMonthly != "monthly" {
		t.Errorf("BudgetPeriodMonthly = %q, want monthly", BudgetPeriodMonthly)
	}
}

func TestBudgetStruct(t *testing.T) {
	b := Budget{
		Scope:    "workspace",
		Period:   BudgetPeriodDaily,
		LimitUSD: 100.0,
		AlertAt:  0.8,
		HardStop: true,
	}

	if b.Scope != "workspace" {
		t.Errorf("Scope = %q, want workspace", b.Scope)
	}
	if b.Period != BudgetPeriodDaily {
		t.Errorf("Period = %q, want daily", b.Period)
	}
	if b.LimitUSD != 100.0 {
		t.Errorf("LimitUSD = %f, want 100.0", b.LimitUSD)
	}
	if b.AlertAt != 0.8 {
		t.Errorf("AlertAt = %f, want 0.8", b.AlertAt)
	}
	if !b.HardStop {
		t.Error("HardStop should be true")
	}
}

func TestBudgetStatusStruct(t *testing.T) {
	status := BudgetStatus{
		CurrentSpend: 80.0,
		Remaining:    20.0,
		PercentUsed:  0.8,
		IsOverBudget: false,
		IsNearLimit:  true,
	}

	if status.CurrentSpend != 80.0 {
		t.Errorf("CurrentSpend = %f, want 80.0", status.CurrentSpend)
	}
	if status.Remaining != 20.0 {
		t.Errorf("Remaining = %f, want 20.0", status.Remaining)
	}
	if status.PercentUsed != 0.8 {
		t.Errorf("PercentUsed = %f, want 0.8", status.PercentUsed)
	}
	if status.IsOverBudget {
		t.Error("IsOverBudget should be false")
	}
	if !status.IsNearLimit {
		t.Error("IsNearLimit should be true")
	}
}

// --- Additional coverage tests (#1236) ---

// TestStoreCloseActive tests Close on an active store
func TestStoreCloseActive(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if err := store.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Record something first to ensure DB is active
	_, recErr := store.Record(context.Background(), "agent-1", "", "gpt-4", 100, 50, 0.01)
	if recErr != nil {
		t.Fatalf("Record: %v", recErr)
	}

	// Close should succeed
	if closeErr := store.Close(); closeErr != nil {
		t.Errorf("Close: %v", closeErr)
	}
}

// TestStoreCloseNilDB tests Close with nil db
func TestStoreCloseNilDB(t *testing.T) {
	store := &Store{db: nil}
	if err := store.Close(); err != nil {
		t.Errorf("Close nil db: %v", err)
	}
}

// TestGetByAgentDefaultLimit tests GetByAgent with zero limit (uses default)
func TestGetByAgentDefaultLimit(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if err := store.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Add records
	for i := 0; i < 5; i++ {
		_, recErr := store.Record(context.Background(), "agent-default", "", "gpt-4", int64(100*i), 50, 0.01)
		if recErr != nil {
			t.Fatalf("Record %d: %v", i, recErr)
		}
	}

	// Get with 0 limit (should use default 100)
	records, getErr := store.GetByAgent(context.Background(), "agent-default", 0)
	if getErr != nil {
		t.Fatalf("GetByAgent: %v", getErr)
	}

	if len(records) != 5 {
		t.Errorf("GetByAgent returned %d records, want 5", len(records))
	}
}

// TestGetByAgentWithLimit tests GetByAgent with specific limit
func TestGetByAgentWithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if err := store.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Add 10 records
	for i := 0; i < 10; i++ {
		_, recErr := store.Record(context.Background(), "agent-limit", "", "gpt-4", int64(100), 50, 0.01)
		if recErr != nil {
			t.Fatalf("Record %d: %v", i, recErr)
		}
	}

	// Get with limit 3
	records, getErr := store.GetByAgent(context.Background(), "agent-limit", 3)
	if getErr != nil {
		t.Fatalf("GetByAgent: %v", getErr)
	}

	if len(records) != 3 {
		t.Errorf("GetByAgent with limit 3 returned %d records", len(records))
	}
}

// TestGetByTeamDefaultLimit tests GetByTeam with zero limit
func TestGetByTeamDefaultLimit(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if err := store.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Add records with team
	for i := 0; i < 5; i++ {
		_, recErr := store.Record(context.Background(), "agent-team", "backend", "gpt-4", int64(100), 50, 0.01)
		if recErr != nil {
			t.Fatalf("Record %d: %v", i, recErr)
		}
	}

	// Get with 0 limit
	records, getErr := store.GetByTeam(context.Background(), "backend", 0)
	if getErr != nil {
		t.Fatalf("GetByTeam: %v", getErr)
	}

	if len(records) != 5 {
		t.Errorf("GetByTeam returned %d records, want 5", len(records))
	}
}

// TestGetByTeamWithLimit tests GetByTeam with specific limit
func TestGetByTeamWithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if err := store.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Add 10 records
	for i := 0; i < 10; i++ {
		_, recErr := store.Record(context.Background(), "agent-team2", "frontend", "gpt-4", int64(100), 50, 0.01)
		if recErr != nil {
			t.Fatalf("Record %d: %v", i, recErr)
		}
	}

	// Get with limit 2
	records, getErr := store.GetByTeam(context.Background(), "frontend", 2)
	if getErr != nil {
		t.Fatalf("GetByTeam: %v", getErr)
	}

	if len(records) != 2 {
		t.Errorf("GetByTeam with limit 2 returned %d records", len(records))
	}
}

// TestGetAllDefaultLimit tests GetAll with zero limit
func TestGetAllDefaultLimit(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if err := store.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Add records
	for i := 0; i < 5; i++ {
		_, recErr := store.Record(context.Background(), "agent-all", "", "gpt-4", int64(100), 50, 0.01)
		if recErr != nil {
			t.Fatalf("Record %d: %v", i, recErr)
		}
	}

	// Get with 0 limit
	records, getErr := store.GetAll(context.Background(), 0)
	if getErr != nil {
		t.Fatalf("GetAll: %v", getErr)
	}

	if len(records) != 5 {
		t.Errorf("GetAll returned %d records, want 5", len(records))
	}
}

// TestGetAllWithLimit tests GetAll with specific limit
func TestGetAllWithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if err := store.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Add 10 records
	for i := 0; i < 10; i++ {
		_, recErr := store.Record(context.Background(), "agent-all2", "", "gpt-4", int64(100), 50, 0.01)
		if recErr != nil {
			t.Fatalf("Record %d: %v", i, recErr)
		}
	}

	// Get with limit 4
	records, getErr := store.GetAll(context.Background(), 4)
	if getErr != nil {
		t.Fatalf("GetAll: %v", getErr)
	}

	if len(records) != 4 {
		t.Errorf("GetAll with limit 4 returned %d records", len(records))
	}
}

// TestGetByAgentEmpty tests GetByAgent for non-existent agent
func TestGetByAgentEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if err := store.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = store.Close() }()

	records, getErr := store.GetByAgent(context.Background(), "nonexistent", 10)
	if getErr != nil {
		t.Fatalf("GetByAgent: %v", getErr)
	}

	if len(records) != 0 {
		t.Errorf("GetByAgent nonexistent returned %d records, want 0", len(records))
	}
}

// TestGetByTeamEmpty tests GetByTeam for non-existent team
func TestGetByTeamEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if err := store.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = store.Close() }()

	records, getErr := store.GetByTeam(context.Background(), "nonexistent-team", 10)
	if getErr != nil {
		t.Fatalf("GetByTeam: %v", getErr)
	}

	if len(records) != 0 {
		t.Errorf("GetByTeam nonexistent returned %d records, want 0", len(records))
	}
}

// TestCheckBudgetWeeklyPeriod tests CheckBudget with weekly period
func TestCheckBudgetWeeklyPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create weekly budget
	_, err := store.SetBudget(context.Background(), "workspace", BudgetPeriodWeekly, 500.0, 0.75, false)
	if err != nil {
		t.Fatalf("SetBudget failed: %v", err)
	}

	// Add records
	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 100.0)
	_, _ = store.Record(context.Background(), "agent-2", "", "model-a", 200, 100, 150.0)

	status, err := store.CheckBudget(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("CheckBudget failed: %v", err)
	}
	if status == nil {
		t.Fatal("status should not be nil")
	}
	if status.CurrentSpend != 250.0 {
		t.Errorf("CurrentSpend = %f, want 250.0", status.CurrentSpend)
	}
	if status.Remaining != 250.0 {
		t.Errorf("Remaining = %f, want 250.0", status.Remaining)
	}
}

// TestCheckBudgetMonthlyPeriod tests CheckBudget with monthly period
func TestCheckBudgetMonthlyPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create monthly budget
	_, err := store.SetBudget(context.Background(), "workspace", BudgetPeriodMonthly, 1000.0, 0.9, true)
	if err != nil {
		t.Fatalf("SetBudget failed: %v", err)
	}

	// Add records
	_, _ = store.Record(context.Background(), "agent-1", "", "model-a", 100, 50, 200.0)

	status, err := store.CheckBudget(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("CheckBudget failed: %v", err)
	}
	if status == nil {
		t.Fatal("status should not be nil")
	}
	if status.CurrentSpend != 200.0 {
		t.Errorf("CurrentSpend = %f, want 200.0", status.CurrentSpend)
	}
}

// TestCheckBudgetAgentScope tests CheckBudget with agent scope
func TestCheckBudgetAgentScope(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create agent-scoped budget
	_, err := store.SetBudget(context.Background(), "agent:eng-01", BudgetPeriodDaily, 50.0, 0.8, true)
	if err != nil {
		t.Fatalf("SetBudget failed: %v", err)
	}

	// Add records for different agents
	_, _ = store.Record(context.Background(), "eng-01", "", "model-a", 100, 50, 20.0)
	_, _ = store.Record(context.Background(), "eng-02", "", "model-a", 200, 100, 30.0) // different agent

	status, err := store.CheckBudget(context.Background(), "agent:eng-01")
	if err != nil {
		t.Fatalf("CheckBudget failed: %v", err)
	}
	if status == nil {
		t.Fatal("status should not be nil")
	}
	// Should only count eng-01's spend
	if status.CurrentSpend != 20.0 {
		t.Errorf("CurrentSpend = %f, want 20.0", status.CurrentSpend)
	}
}

// TestCheckBudgetTeamScope tests CheckBudget with team scope
func TestCheckBudgetTeamScope(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	if err := store.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create team-scoped budget
	_, err := store.SetBudget(context.Background(), "team:backend", BudgetPeriodDaily, 200.0, 0.7, false)
	if err != nil {
		t.Fatalf("SetBudget failed: %v", err)
	}

	// Add records for different teams
	_, _ = store.Record(context.Background(), "eng-01", "backend", "model-a", 100, 50, 40.0)
	_, _ = store.Record(context.Background(), "eng-02", "backend", "model-a", 200, 100, 60.0)
	_, _ = store.Record(context.Background(), "eng-03", "frontend", "model-a", 300, 150, 80.0) // different team

	status, err := store.CheckBudget(context.Background(), "team:backend")
	if err != nil {
		t.Fatalf("CheckBudget failed: %v", err)
	}
	if status == nil {
		t.Fatal("status should not be nil")
	}
	// Should only count backend team's spend
	if status.CurrentSpend != 100.0 {
		t.Errorf("CurrentSpend = %f, want 100.0", status.CurrentSpend)
	}
}
