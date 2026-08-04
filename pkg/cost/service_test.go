package cost

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/provider"
)

// stubProvider implements provider.Provider + provider.CostReader with
// canned entries so Service aggregation is tested without touching the
// filesystem.
type stubProvider struct {
	entries []provider.CostEntry
	reads   int
}

func (s *stubProvider) Name() string                             { return "stub" }
func (s *stubProvider) Description() string                      { return "stub" }
func (s *stubProvider) Command() string                          { return "stub" }
func (s *stubProvider) Binary() string                           { return "stub" }
func (s *stubProvider) InstallHint() string                      { return "" }
func (s *stubProvider) BuildCommand(provider.CommandOpts) string { return "stub" }
func (s *stubProvider) IsInstalled(context.Context) bool         { return true }
func (s *stubProvider) Version(context.Context) string           { return "1" }
func (s *stubProvider) ReadCosts(_ context.Context, _ provider.CostReadOptions) ([]provider.CostEntry, error) {
	s.reads++
	return s.entries, nil
}

func day(d int, hour int) time.Time {
	return time.Date(2026, 7, d, hour, 0, 0, 0, time.UTC)
}

func newStubService(entries []provider.CostEntry, budgets BudgetStore) (*Service, *stubProvider) {
	stub := &stubProvider{entries: entries}
	reg := provider.NewRegistry()
	reg.Register(stub)
	return NewService(reg, Options{}, budgets), stub
}

func sampleEntries() []provider.CostEntry {
	return []provider.CostEntry{
		{Timestamp: day(1, 10), Agent: "a1", Repo: "/r1", Model: "m1", InputTokens: 100, OutputTokens: 50, CacheReadTokens: 10, CacheWriteTokens: 5, CostUSD: 1.0},
		{Timestamp: day(1, 12), Agent: "a1", Repo: "/r1", Model: "m2", InputTokens: 200, OutputTokens: 100, CostUSD: 2.0},
		{Timestamp: day(2, 10), Agent: "a2", Repo: "/r2", Model: "m1", InputTokens: 300, OutputTokens: 150, CostUSD: 4.0},
		{Timestamp: day(3, 10), Agent: "a2", Repo: "", Model: "m1", InputTokens: 10, OutputTokens: 5, CostUSD: 0.5},
	}
}

func TestEmptySummaryDeclaresPricedBasis(t *testing.T) {
	svc, _ := newStubService(nil, nil)
	sum, err := svc.TotalSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sum.CostBasis != CostBasisPriced {
		t.Errorf("empty summary cost_basis = %q, want %q", sum.CostBasis, CostBasisPriced)
	}
}

func TestTotalSummaryAggregates(t *testing.T) {
	svc, _ := newStubService(sampleEntries(), nil)
	sum, err := svc.TotalSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sum.InputTokens != 610 || sum.OutputTokens != 305 {
		t.Errorf("tokens wrong: %+v", sum)
	}
	if sum.TotalTokens != 915 {
		t.Errorf("total tokens must be input+output, got %d", sum.TotalTokens)
	}
	if sum.CacheReadTokens != 10 || sum.CacheWriteTokens != 5 {
		t.Errorf("cache tokens wrong: %+v", sum)
	}
	if math.Abs(sum.TotalCostUSD-7.5) > 1e-9 || sum.RecordCount != 4 {
		t.Errorf("cost/count wrong: %+v", sum)
	}
	if sum.CostBasis != CostBasisPriced {
		t.Errorf("cost_basis = %q, want %q", sum.CostBasis, CostBasisPriced)
	}
}

func TestSummaryByAgentSortedByCost(t *testing.T) {
	svc, _ := newStubService(sampleEntries(), nil)
	sums, err := svc.SummaryByAgent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != 2 {
		t.Fatalf("want 2 agents, got %d", len(sums))
	}
	if sums[0].AgentID != "a2" || sums[1].AgentID != "a1" {
		t.Errorf("not sorted by cost desc: %+v", sums)
	}
	if math.Abs(sums[1].TotalCostUSD-3.0) > 1e-9 {
		t.Errorf("a1 cost = %v, want 3.0", sums[1].TotalCostUSD)
	}
}

func TestSummaryByTeamIsAlwaysEmpty(t *testing.T) {
	svc, _ := newStubService(sampleEntries(), nil)
	sums, err := svc.SummaryByTeam(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sums == nil || len(sums) != 0 {
		t.Errorf("team summaries must be empty non-nil, got %+v", sums)
	}
}

func TestAgentSummaryFilters(t *testing.T) {
	svc, _ := newStubService(sampleEntries(), nil)
	sum, err := svc.AgentSummary(context.Background(), "a1")
	if err != nil {
		t.Fatal(err)
	}
	if sum.AgentID != "a1" || sum.RecordCount != 2 || math.Abs(sum.TotalCostUSD-3.0) > 1e-9 {
		t.Errorf("agent summary wrong: %+v", sum)
	}
}

func TestDailyCostsGroupAndSort(t *testing.T) {
	svc, _ := newStubService(sampleEntries(), nil)
	daily, err := svc.GetDailyCosts(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(daily) != 3 {
		t.Fatalf("want 3 days, got %d", len(daily))
	}
	if daily[0].Date != "2026-07-01" || daily[0].RecordCount != 2 || math.Abs(daily[0].CostUSD-3.0) > 1e-9 {
		t.Errorf("day 1 wrong: %+v", daily[0])
	}
	// Since filter drops old days.
	daily, err = svc.GetDailyCosts(context.Background(), day(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(daily) != 2 {
		t.Errorf("since filter wrong: %+v", daily)
	}
}

func TestSumByRepoAndProject(t *testing.T) {
	svc, _ := newStubService(sampleEntries(), nil)
	byRepo, err := svc.SumByRepo(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(byRepo["/r1"]-3.0) > 1e-9 || math.Abs(byRepo["/r2"]-4.0) > 1e-9 || math.Abs(byRepo[""]-0.5) > 1e-9 {
		t.Errorf("byRepo wrong: %+v", byRepo)
	}

	byProj, err := svc.SumByProject(context.Background(), time.Time{}, RepoLabel)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(byProj["r1"]-3.0) > 1e-9 || math.Abs(byProj["unattributed"]-0.5) > 1e-9 {
		t.Errorf("byProject wrong: %+v", byProj)
	}
}

func TestEntriesCacheAndRefresh(t *testing.T) {
	svc, stub := newStubService(sampleEntries(), nil)
	ctx := context.Background()

	if _, err := svc.Entries(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Entries(ctx); err != nil {
		t.Fatal(err)
	}
	if stub.reads != 1 {
		t.Errorf("cache miss: %d reads, want 1", stub.reads)
	}

	n, err := svc.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 || stub.reads != 2 {
		t.Errorf("refresh: n=%d reads=%d", n, stub.reads)
	}
}

// slowProvider is a CostReader whose scan blocks briefly and counts its
// invocations atomically, so concurrent Entries callers can assert the
// scan was single-flighted rather than run once per caller.
type slowProvider struct {
	entries []provider.CostEntry
	reads   atomic.Int64
	delay   time.Duration
}

func (s *slowProvider) Name() string                             { return "slow" }
func (s *slowProvider) Description() string                      { return "slow" }
func (s *slowProvider) Command() string                          { return "slow" }
func (s *slowProvider) Binary() string                           { return "slow" }
func (s *slowProvider) InstallHint() string                      { return "" }
func (s *slowProvider) BuildCommand(provider.CommandOpts) string { return "slow" }
func (s *slowProvider) IsInstalled(context.Context) bool         { return true }
func (s *slowProvider) Version(context.Context) string           { return "1" }
func (s *slowProvider) ReadCosts(_ context.Context, _ provider.CostReadOptions) ([]provider.CostEntry, error) {
	s.reads.Add(1)
	time.Sleep(s.delay)
	return s.entries, nil
}

// TestEntriesSingleFlight asserts that a burst of concurrent cache-miss
// Entries calls collapses into exactly one underlying scan. This guards
// the fix for the multi-core cost-scan burn: without single-flight, a
// page load (drawer + agent list + cost widgets) launched N parallel
// full transcript re-parses.
func TestEntriesSingleFlight(t *testing.T) {
	slow := &slowProvider{entries: sampleEntries(), delay: 50 * time.Millisecond}
	reg := provider.NewRegistry()
	reg.Register(slow)
	svc := NewService(reg, Options{}, nil)
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := svc.Entries(ctx); err != nil {
				t.Errorf("Entries: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := slow.reads.Load(); got != 1 {
		t.Errorf("concurrent cache-miss scans: %d reads, want 1 (single-flight)", got)
	}
}

// TestEntriesTTLExpiry asserts the cache serves repeat reads without
// re-scanning until the TTL lapses, then re-scans exactly once.
func TestEntriesTTLExpiry(t *testing.T) {
	slow := &slowProvider{entries: sampleEntries()}
	reg := provider.NewRegistry()
	reg.Register(slow)
	svc := NewService(reg, Options{CacheTTL: 40 * time.Millisecond}, nil)
	ctx := context.Background()

	if _, err := svc.Entries(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Entries(ctx); err != nil {
		t.Fatal(err)
	}
	if got := slow.reads.Load(); got != 1 {
		t.Fatalf("within TTL: %d reads, want 1", got)
	}

	time.Sleep(60 * time.Millisecond)
	if _, err := svc.Entries(ctx); err != nil {
		t.Fatal(err)
	}
	if got := slow.reads.Load(); got != 2 {
		t.Errorf("after TTL: %d reads, want 2", got)
	}
}

// memBudgets is an in-memory BudgetStore.
type memBudgets struct {
	m map[string]BudgetConfig
}

func (b *memBudgets) All() (map[string]BudgetConfig, error) { return b.m, nil }
func (b *memBudgets) Set(scope string, cfg BudgetConfig) error {
	b.m[scope] = cfg
	return nil
}
func (b *memBudgets) Delete(scope string) error {
	if _, ok := b.m[scope]; !ok {
		return fmt.Errorf("budget not found for scope %q", scope)
	}
	delete(b.m, scope)
	return nil
}

func TestBudgetLifecycleAndCheck(t *testing.T) {
	// Entries dated this month so the monthly period window includes them.
	now := time.Now().UTC()
	entries := []provider.CostEntry{
		{Timestamp: now.Add(-time.Minute), Agent: "a1", CostUSD: 8.0},
		{Timestamp: now.Add(-2 * time.Minute), Agent: "a2", CostUSD: 1.0},
	}
	store := &memBudgets{m: map[string]BudgetConfig{}}
	svc, _ := newStubService(entries, store)
	ctx := context.Background()

	if _, err := svc.SetBudget(ctx, "workspace", BudgetPeriodMonthly, 10, 0.8, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetBudget(ctx, "agent:a1", BudgetPeriodMonthly, 5, 0.5, true); err != nil {
		t.Fatal(err)
	}

	all, err := svc.GetAllBudgets(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("GetAllBudgets: %v %+v", err, all)
	}

	h, err := svc.CheckBudget(ctx, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(h.CurrentSpend-9.0) > 1e-9 || !h.IsNearLimit || h.IsOverBudget {
		t.Errorf("workspace status wrong: %+v", h)
	}

	ag, err := svc.CheckBudget(ctx, "agent:a1")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(ag.CurrentSpend-8.0) > 1e-9 || !ag.IsOverBudget {
		t.Errorf("agent status wrong: %+v", ag)
	}

	if status, err := svc.CheckBudget(ctx, "missing"); err != nil || status != nil {
		t.Errorf("missing budget must be (nil,nil), got %+v %v", status, err)
	}

	if err := svc.DeleteBudget(ctx, "agent:a1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteBudget(ctx, "agent:a1"); err == nil {
		t.Error("double delete must error")
	}
}

func TestProjectCost(t *testing.T) {
	now := time.Now().UTC()
	entries := []provider.CostEntry{
		{Timestamp: now.Add(-48 * time.Hour), CostUSD: 2.0},
		{Timestamp: now.Add(-24 * time.Hour), CostUSD: 4.0},
	}
	svc, _ := newStubService(entries, nil)
	proj, err := svc.ProjectCost(context.Background(), 30, 10*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// The average spreads over the elapsed calendar window (clamped to the
	// observed span), NOT just active days — an active-day average
	// projected over calendar days over-estimates idle fleets. $6 over an
	// ~2-day span → ~$3/day here; the invariant that matters is that a
	// sparse history is never averaged as if every day were active.
	if proj.DaysAnalyzed != 2 || proj.TotalHistorical != 6.0 {
		t.Errorf("projection wrong: %+v", proj)
	}
	if proj.DailyAvgCost > 3.01 || proj.DailyAvgCost < 6.0/30 {
		t.Errorf("daily avg outside sane bounds: %+v", proj)
	}
	if math.Abs(proj.ProjectedCost-proj.DailyAvgCost*10) > 1e-9 {
		t.Errorf("projection must be dailyAvg x days: %+v", proj)
	}
}

func TestProjectCostIdleFleetNotOverestimated(t *testing.T) {
	now := time.Now().UTC()
	// $10 across 2 active days deep in a 30-day window: the old
	// active-day average would project $150/30d; calendar-window math
	// must land near the true ~$10/30d.
	entries := []provider.CostEntry{
		{Timestamp: now.Add(-29 * 24 * time.Hour), CostUSD: 5.0},
		{Timestamp: now.Add(-28 * 24 * time.Hour), CostUSD: 5.0},
	}
	svc, _ := newStubService(entries, nil)
	proj, err := svc.ProjectCost(context.Background(), 30, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if proj.ProjectedCost > 12.0 {
		t.Errorf("idle fleet over-estimated: %+v", proj)
	}
}
