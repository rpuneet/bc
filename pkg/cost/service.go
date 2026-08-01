package cost

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/rpuneet/mycel/pkg/provider"
)

// DefaultCacheTTL is how long a merged entry scan stays fresh before
// the next query re-reads the sources. Scanning is source-direct
// (re-parsing provider transcripts), so a short TTL under UI polling
// meant near-continuous re-scans. The per-file mtime cache in the
// Claude reader makes warm scans cheap, and a several-minute TTL keeps
// idle daemons quiet; ?refresh=1 forces an immediate re-read.
const DefaultCacheTTL = 5 * time.Minute

// Options configures a Service.
type Options struct {
	// Home is the user's home directory (host provider sessions).
	Home string
	// AgentsDir is the mycel agent entity root (~/.mycel/agents).
	AgentsDir string
	// CacheTTL overrides DefaultCacheTTL when > 0.
	CacheTTL time.Duration
}

// Service computes cost analytics directly from provider session files.
// Every read method operates on a briefly-cached merged entry list;
// Refresh forces a re-scan.
type Service struct { //nolint:govet // grouped by role (deps / cache) over 16 bytes of packing
	registry *provider.Registry
	budgets  BudgetStore
	opts     Options
	ttl      time.Duration

	mu        sync.Mutex
	entries   []provider.CostEntry
	scannedAt time.Time
	// sf collapses concurrent cache-miss scans into a single scan whose
	// result every caller shares — a page load fires the drawer, agent
	// list, and cost widgets at once, and without this each would kick
	// off its own multi-second transcript re-parse.
	sf singleflight.Group
}

// NewService creates a Service reading from every provider in registry
// that implements provider.CostReader. budgets may be nil — budget
// methods then return errors.
func NewService(registry *provider.Registry, opts Options, budgets BudgetStore) *Service {
	ttl := opts.CacheTTL
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &Service{
		registry: registry,
		budgets:  budgets,
		opts:     opts,
		ttl:      ttl,
	}
}

// Refresh drops the cache and re-reads every source. Returns the
// number of merged entries.
func (s *Service) Refresh(ctx context.Context) (int, error) {
	entries, err := s.scan(ctx)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	s.entries = entries
	s.scannedAt = time.Now()
	s.mu.Unlock()
	return len(entries), nil
}

// Entries returns the merged entry list, re-scanning the sources when
// the cache is older than the TTL. Concurrent cache misses are collapsed
// into a single scan via singleflight so a burst of requests can never
// launch parallel transcript re-parses.
func (s *Service) Entries(ctx context.Context) ([]provider.CostEntry, error) {
	if entries, ok := s.cachedEntries(); ok {
		return entries, nil
	}

	v, err, _ := s.sf.Do("scan", func() (interface{}, error) {
		// Re-check under the flight: a scan that finished while we
		// waited for the singleflight slot may have refreshed the cache.
		if entries, ok := s.cachedEntries(); ok {
			return entries, nil
		}
		entries, scanErr := s.scan(ctx)
		if scanErr != nil {
			return nil, scanErr
		}
		s.mu.Lock()
		s.entries = entries
		s.scannedAt = time.Now()
		s.mu.Unlock()
		return entries, nil
	})
	if err != nil {
		return nil, err
	}
	entries, _ := v.([]provider.CostEntry) //nolint:errcheck // scan always returns []provider.CostEntry or an error
	return entries, nil
}

// cachedEntries returns the cached entry list when it is still within
// the TTL, reporting false when a re-scan is due.
func (s *Service) cachedEntries() ([]provider.CostEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.scannedAt.IsZero() && time.Since(s.scannedAt) < s.ttl {
		return s.entries, true
	}
	return nil, false
}

// scan fans out to every CostReader provider and merges the results.
func (s *Service) scan(ctx context.Context) ([]provider.CostEntry, error) {
	if s.registry == nil {
		return nil, nil
	}
	readOpts := provider.CostReadOptions{
		Home:      s.opts.Home,
		AgentsDir: s.opts.AgentsDir,
	}
	var merged []provider.CostEntry
	for _, p := range s.registry.List() {
		reader, ok := p.(provider.CostReader)
		if !ok {
			continue
		}
		entries, err := reader.ReadCosts(ctx, readOpts)
		if err != nil {
			// A single broken source must not blank out all analytics —
			// but a canceled context must propagate.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		merged = append(merged, entries...)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Timestamp.Before(merged[j].Timestamp) })
	return merged, nil
}

// ─── Summaries ───────────────────────────────────────────────────────────────

func addEntry(sum *Summary, e provider.CostEntry) {
	sum.InputTokens += e.InputTokens
	sum.OutputTokens += e.OutputTokens
	sum.CacheReadTokens += e.CacheReadTokens
	sum.CacheWriteTokens += e.CacheWriteTokens
	sum.TotalTokens += e.InputTokens + e.OutputTokens
	sum.TotalCostUSD += e.CostUSD
	sum.RecordCount++
}

// TotalSummary returns the total cost summary across all sources.
func (s *Service) TotalSummary(ctx context.Context) (*Summary, error) {
	entries, err := s.Entries(ctx)
	if err != nil {
		return nil, err
	}
	var sum Summary
	for _, e := range entries {
		addEntry(&sum, e)
	}
	return &sum, nil
}

// GetSummarySince returns a summary of costs since the given time.
func (s *Service) GetSummarySince(ctx context.Context, since time.Time) (*Summary, error) {
	entries, err := s.Entries(ctx)
	if err != nil {
		return nil, err
	}
	var sum Summary
	for _, e := range entries {
		if e.Timestamp.Before(since) {
			continue
		}
		addEntry(&sum, e)
	}
	return &sum, nil
}

// SummaryByAgent returns aggregated costs per agent, highest cost first.
func (s *Service) SummaryByAgent(ctx context.Context) ([]*Summary, error) {
	return s.groupedSummaries(ctx, time.Time{}, func(e provider.CostEntry) string { return e.Agent },
		func(sum *Summary, key string) { sum.AgentID = key })
}

// GetAgentSummarySince returns per-agent summaries since the given time.
func (s *Service) GetAgentSummarySince(ctx context.Context, since time.Time) ([]*Summary, error) {
	return s.groupedSummaries(ctx, since, func(e provider.CostEntry) string { return e.Agent },
		func(sum *Summary, key string) { sum.AgentID = key })
}

// SummaryByModel returns aggregated costs per model, highest cost first.
func (s *Service) SummaryByModel(ctx context.Context) ([]*Summary, error) {
	return s.GetModelSummarySince(ctx, time.Time{})
}

// GetModelSummarySince returns per-model summaries since the given time.
func (s *Service) GetModelSummarySince(ctx context.Context, since time.Time) ([]*Summary, error) {
	return s.GetAgentModelSummarySince(ctx, "", since)
}

// GetAgentModelSummarySince returns per-model summaries since the given
// time, restricted to one cost agent id when agentID is non-empty. The id
// is the recorded session id (plain agent name on fresh installs, a
// legacy-prefixed id otherwise) — callers resolve it from the per-agent
// listing, mirroring the web's alias handling.
func (s *Service) GetAgentModelSummarySince(ctx context.Context, agentID string, since time.Time) ([]*Summary, error) {
	entries, err := s.Entries(ctx)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*Summary{}
	for _, e := range entries {
		if agentID != "" && e.Agent != agentID {
			continue
		}
		if !since.IsZero() && e.Timestamp.Before(since) {
			continue
		}
		sum, ok := byKey[e.Model]
		if !ok {
			sum = &Summary{Model: e.Model}
			byKey[e.Model] = sum
		}
		addEntry(sum, e)
	}
	out := make([]*Summary, 0, len(byKey))
	for _, sum := range byKey {
		out = append(out, sum)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalCostUSD > out[j].TotalCostUSD })
	return out, nil
}

// SummaryByTeam returns aggregated costs per team. Provider session
// files carry no team attribution, so the result is always empty; the
// method exists so API responses keep their shape.
func (s *Service) SummaryByTeam(_ context.Context) ([]*Summary, error) {
	return []*Summary{}, nil
}

// AgentSummary returns the cost summary for a specific agent.
func (s *Service) AgentSummary(ctx context.Context, agentID string) (*Summary, error) {
	entries, err := s.Entries(ctx)
	if err != nil {
		return nil, err
	}
	sum := Summary{AgentID: agentID}
	for _, e := range entries {
		if e.Agent != agentID {
			continue
		}
		addEntry(&sum, e)
	}
	return &sum, nil
}

// groupedSummaries aggregates entries since `since` by key(e), sorted
// by total cost descending.
func (s *Service) groupedSummaries(ctx context.Context, since time.Time, key func(provider.CostEntry) string, label func(*Summary, string)) ([]*Summary, error) {
	entries, err := s.Entries(ctx)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*Summary{}
	for _, e := range entries {
		if !since.IsZero() && e.Timestamp.Before(since) {
			continue
		}
		k := key(e)
		sum, ok := byKey[k]
		if !ok {
			sum = &Summary{}
			label(sum, k)
			byKey[k] = sum
		}
		addEntry(sum, e)
	}
	out := make([]*Summary, 0, len(byKey))
	for _, sum := range byKey {
		out = append(out, sum)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalCostUSD > out[j].TotalCostUSD })
	return out, nil
}

// ─── Daily breakdowns ────────────────────────────────────────────────────────

// GetDailyCosts returns daily cost totals since the given time,
// ascending by day.
func (s *Service) GetDailyCosts(ctx context.Context, since time.Time) ([]*DailyCost, error) {
	entries, err := s.Entries(ctx)
	if err != nil {
		return nil, err
	}
	byDay := map[string]*DailyCost{}
	for _, e := range entries {
		if e.Timestamp.Before(since) {
			continue
		}
		day := e.Timestamp.UTC().Format("2006-01-02")
		dc, ok := byDay[day]
		if !ok {
			dc = &DailyCost{Date: day}
			byDay[day] = dc
		}
		dc.CostUSD += e.CostUSD
		dc.TotalTokens += e.InputTokens + e.OutputTokens
		dc.InputTokens += e.InputTokens
		dc.OutputTokens += e.OutputTokens
		dc.RecordCount++
	}
	out := make([]*DailyCost, 0, len(byDay))
	for _, dc := range byDay {
		out = append(out, dc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

// GetAgentDailyCosts returns daily cost totals per agent since the
// given time, ordered by agent then day.
func (s *Service) GetAgentDailyCosts(ctx context.Context, since time.Time) ([]*AgentDailyCost, error) {
	entries, err := s.Entries(ctx)
	if err != nil {
		return nil, err
	}
	type key struct{ agent, day string }
	byKey := map[key]*AgentDailyCost{}
	for _, e := range entries {
		if e.Timestamp.Before(since) {
			continue
		}
		k := key{agent: e.Agent, day: e.Timestamp.UTC().Format("2006-01-02")}
		adc, ok := byKey[k]
		if !ok {
			adc = &AgentDailyCost{AgentID: k.agent, Date: k.day}
			byKey[k] = adc
		}
		adc.CostUSD += e.CostUSD
		adc.TotalTokens += e.InputTokens + e.OutputTokens
		adc.InputTokens += e.InputTokens
		adc.OutputTokens += e.OutputTokens
		adc.RecordCount++
	}
	out := make([]*AgentDailyCost, 0, len(byKey))
	for _, adc := range byKey {
		out = append(out, adc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AgentID != out[j].AgentID {
			return out[i].AgentID < out[j].AgentID
		}
		return out[i].Date < out[j].Date
	})
	return out, nil
}

// ProjectCost calculates a projected cost based on historical daily average.
func (s *Service) ProjectCost(ctx context.Context, lookbackDays int, projectDuration time.Duration) (*Projection, error) {
	since := time.Now().AddDate(0, 0, -lookbackDays)
	dailyCosts, err := s.GetDailyCosts(ctx, since)
	if err != nil {
		return nil, err
	}

	proj := &Projection{
		Duration:     projectDuration,
		DaysAnalyzed: len(dailyCosts),
	}

	if len(dailyCosts) == 0 {
		return proj, nil
	}

	for _, dc := range dailyCosts {
		proj.TotalHistorical += dc.CostUSD
	}
	// Average over the CALENDAR window, not just active days — dailyCosts
	// only contains days with activity, and projecting an active-day
	// average over calendar days over-estimates idle fleets. Clamp the
	// window to the observed span so short histories don't under-estimate.
	windowDays := float64(lookbackDays)
	if first := earliestDay(dailyCosts); !first.IsZero() {
		if elapsed := time.Since(first).Hours() / 24; elapsed >= 1 && elapsed < windowDays {
			windowDays = elapsed
		}
	}
	if windowDays < 1 {
		windowDays = 1
	}
	proj.DailyAvgCost = proj.TotalHistorical / windowDays

	projectDays := projectDuration.Hours() / 24
	proj.ProjectedCost = proj.DailyAvgCost * projectDays

	return proj, nil
}

// earliestDay returns the oldest day present in the slice (dates are
// YYYY-MM-DD strings; unparseable entries are ignored).
func earliestDay(days []*DailyCost) time.Time {
	var first time.Time
	for _, d := range days {
		t, err := time.Parse("2006-01-02", d.Date)
		if err != nil {
			continue
		}
		if first.IsZero() || t.Before(first) {
			first = t
		}
	}
	return first
}

// ─── Repo rollups ────────────────────────────────────────────────────────────

// SumByRepo returns total cost USD aggregated by the session working
// directory ("repo") for entries since the given time. Sessions
// without a recorded working dir land under the empty-string key.
func (s *Service) SumByRepo(ctx context.Context, since time.Time) (map[string]float64, error) {
	entries, err := s.Entries(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for _, e := range entries {
		if e.Timestamp.Before(since) {
			continue
		}
		out[e.Repo] += e.CostUSD
	}
	return out, nil
}

// RepoNameResolver converts a repo path to its human-readable name.
type RepoNameResolver func(repo string) string

// SumByProject returns total cost USD grouped by a project-level key
// since the given time. Each repo path is mapped through resolve() to
// produce the grouping key. Entries without a repo are placed under
// the "unattributed" key.
func (s *Service) SumByProject(ctx context.Context, since time.Time, resolve RepoNameResolver) (map[string]float64, error) {
	byRepo, err := s.SumByRepo(ctx, since)
	if err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for repo, total := range byRepo {
		key := "unattributed"
		if repo != "" {
			key = repo
			if resolve != nil {
				if name := resolve(repo); name != "" {
					key = name
				}
			}
		}
		out[key] += total
	}
	return out, nil
}

// ─── Budgets ─────────────────────────────────────────────────────────────────

// errNoBudgetStore is returned when the service was built without a
// budget store.
var errNoBudgetStore = fmt.Errorf("budget store not configured")

// GetAllBudgets returns all configured budgets, sorted by scope.
func (s *Service) GetAllBudgets(_ context.Context) ([]*Budget, error) {
	if s.budgets == nil {
		return nil, errNoBudgetStore
	}
	all, err := s.budgets.All()
	if err != nil {
		return nil, err
	}
	out := make([]*Budget, 0, len(all))
	for scope, cfg := range all {
		out = append(out, budgetFromConfig(scope, cfg))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Scope < out[j].Scope })
	return out, nil
}

// GetBudget returns the budget for a given scope, or nil when none is
// configured.
func (s *Service) GetBudget(_ context.Context, scope string) (*Budget, error) {
	if s.budgets == nil {
		return nil, errNoBudgetStore
	}
	all, err := s.budgets.All()
	if err != nil {
		return nil, err
	}
	cfg, ok := all[scope]
	if !ok {
		return nil, nil
	}
	return budgetFromConfig(scope, cfg), nil
}

// SetBudget creates or updates a budget for the given scope.
func (s *Service) SetBudget(_ context.Context, scope string, period BudgetPeriod, limitUSD, alertAt float64, hardStop bool) (*Budget, error) {
	if s.budgets == nil {
		return nil, errNoBudgetStore
	}
	cfg := BudgetConfig{
		Period:    period,
		LimitUSD:  limitUSD,
		AlertAt:   alertAt,
		HardStop:  hardStop,
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.budgets.Set(scope, cfg); err != nil {
		return nil, err
	}
	return budgetFromConfig(scope, cfg), nil
}

// DeleteBudget removes a budget for the given scope.
func (s *Service) DeleteBudget(_ context.Context, scope string) error {
	if s.budgets == nil {
		return errNoBudgetStore
	}
	return s.budgets.Delete(scope)
}

// CheckBudget returns the current computed spend against a budget.
// Returns (nil, nil) when no budget is configured for the scope.
func (s *Service) CheckBudget(ctx context.Context, scope string) (*BudgetStatus, error) {
	budget, err := s.GetBudget(ctx, scope)
	if err != nil || budget == nil {
		return nil, err
	}

	// Calculate period start time
	now := time.Now().UTC()
	var periodStart time.Time
	switch budget.Period {
	case BudgetPeriodDaily:
		periodStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case BudgetPeriodWeekly:
		// Start of week (Sunday)
		daysFromSunday := int(now.Weekday())
		periodStart = time.Date(now.Year(), now.Month(), now.Day()-daysFromSunday, 0, 0, 0, 0, time.UTC)
	default: // monthly
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	entries, err := s.Entries(ctx)
	if err != nil {
		return nil, err
	}
	var currentSpend float64
	agentScope, isAgent := budgetScopeAgent(scope)
	for _, e := range entries {
		if e.Timestamp.Before(periodStart) {
			continue
		}
		if isAgent && e.Agent != agentScope {
			continue
		}
		// team:<id> scopes never match — sources carry no team data.
		if !isAgent && scope != "workspace" {
			continue
		}
		currentSpend += e.CostUSD
	}

	status := &BudgetStatus{
		Budget:       budget,
		CurrentSpend: currentSpend,
		Remaining:    budget.LimitUSD - currentSpend,
	}
	if budget.LimitUSD > 0 {
		status.PercentUsed = currentSpend / budget.LimitUSD
		status.IsOverBudget = currentSpend >= budget.LimitUSD
		status.IsNearLimit = status.PercentUsed >= budget.AlertAt
	}
	if status.Remaining < 0 {
		status.Remaining = 0
	}
	return status, nil
}

// budgetScopeAgent parses "agent:<id>" scopes.
func budgetScopeAgent(scope string) (string, bool) {
	const prefix = "agent:"
	if len(scope) > len(prefix) && scope[:len(prefix)] == prefix {
		return scope[len(prefix):], true
	}
	return "", false
}

func budgetFromConfig(scope string, cfg BudgetConfig) *Budget {
	return &Budget{
		Scope:     scope,
		Period:    cfg.Period,
		LimitUSD:  cfg.LimitUSD,
		AlertAt:   cfg.AlertAt,
		HardStop:  cfg.HardStop,
		UpdatedAt: cfg.UpdatedAt,
	}
}

// ─── Helpers for callers ─────────────────────────────────────────────────────

// RepoLabel maps a repo path to a human-readable label: the directory
// basename, or the path itself when it has no useful base.
func RepoLabel(repo string) string {
	if repo == "" {
		return repo
	}
	if base := filepath.Base(repo); base != "." && base != string(filepath.Separator) {
		return base
	}
	return repo
}
