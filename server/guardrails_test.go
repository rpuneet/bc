package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
	dbpkg "github.com/rpuneet/mycel/pkg/db"
	eventspkg "github.com/rpuneet/mycel/pkg/events"
	templatepkg "github.com/rpuneet/mycel/pkg/template"
)

// fakeCostQuerier returns a fixed spend for every agent, keyed by name so a
// single test can give different agents different totals.
type fakeCostQuerier struct {
	byAgent map[string]float64
}

func (f *fakeCostQuerier) AgentCostSummary(agentID string) (*agentpkg.CostSummary, error) {
	return &agentpkg.CostSummary{AgentID: agentID, TotalCostUSD: f.byAgent[agentID]}, nil
}

// fakeEventStore is a minimal eventspkg.EventStore stub. Only
// ReadByAgentPage is meaningful; it returns the configured events for the
// requested agent (newest first, matching the real store's contract).
type fakeEventStore struct {
	byAgent map[string][]eventspkg.Event
}

func (f *fakeEventStore) Append(eventspkg.Event) error            { return nil }
func (f *fakeEventStore) Read() ([]eventspkg.Event, error)        { return nil, nil }
func (f *fakeEventStore) ReadLast(int) ([]eventspkg.Event, error) { return nil, nil }
func (f *fakeEventStore) ReadByAgent(name string) ([]eventspkg.Event, error) {
	return f.byAgent[name], nil
}
func (f *fakeEventStore) ReadByAgentPage(name string, limit int, _ int64) ([]eventspkg.Event, error) {
	evs := f.byAgent[name]
	if len(evs) > limit {
		evs = evs[:limit]
	}
	return evs, nil
}
func (f *fakeEventStore) Close() error { return nil }

// guardrailTestSetup wires a real Manager (SQLite-backed via MYCEL_HOME, tmux
// runtime — never actually started) + AgentService, and seeds one agent
// through the exported Save/LoadState path so the test only exercises
// guardrails.go, not agent spawning.
func guardrailTestSetup(t *testing.T, a *agentpkg.Agent, cost *fakeCostQuerier) (*agentpkg.AgentService, *templatepkg.Store) {
	t.Helper()
	t.Setenv("MYCEL_HOME", t.TempDir())

	repo := t.TempDir()
	gitInitDir(t, repo)

	dbPath, err := dbpkg.GlobalDBPath()
	if err != nil {
		t.Fatalf("GlobalDBPath: %v", err)
	}
	store, err := agentpkg.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	if err := store.Save(context.Background(), a); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	mgr := agentpkg.NewManagerWithRepo(filepath.Join(t.TempDir(), "agents"), repo)
	if err := mgr.LoadState(); err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	var cq agentpkg.CostQuerier
	if cost != nil {
		cq = cost
	}
	svc := agentpkg.NewAgentService(mgr, nil, cq)

	tmplStore := templatepkg.NewStore(t.TempDir())
	return svc, tmplStore
}

func mustCreateTemplate(t *testing.T, store *templatepkg.Store, tmpl templatepkg.Template) {
	t.Helper()
	if err := store.Create(tmpl, "", templatepkg.ScopeGlobal); err != nil {
		t.Fatalf("create template %q: %v", tmpl.Name, err)
	}
}

// TestGuardrail_CostOverLimit_StopsAgent verifies that an agent whose
// session spend has reached its template's MaxCostUSD is stopped, with a
// task/reason recorded explaining why.
func TestGuardrail_CostOverLimit_StopsAgent(t *testing.T) {
	agent := &agentpkg.Agent{
		Name: "spendy", State: agentpkg.StateWorking, Template: "costly",
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	svc, tmplStore := guardrailTestSetup(t, agent, &fakeCostQuerier{byAgent: map[string]float64{"spendy": 5.00}})
	mustCreateTemplate(t, tmplStore, templatepkg.Template{Name: "costly", MaxCostUSD: 2.50})

	checkAllGuardrails(context.Background(), svc, tmplStore, nil)

	got, err := svc.Get(context.Background(), "spendy")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != agentpkg.StateStopped {
		t.Fatalf("state = %s, want stopped", got.State)
	}
	if got.Task == "" {
		t.Fatal("expected task/reason to be recorded explaining the stop")
	}
}

// TestGuardrail_UnderCostLimit_NotStopped verifies an agent whose spend has
// not yet reached MaxCostUSD is left running.
func TestGuardrail_UnderCostLimit_NotStopped(t *testing.T) {
	agent := &agentpkg.Agent{
		Name: "frugal", State: agentpkg.StateWorking, Template: "costly",
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	svc, tmplStore := guardrailTestSetup(t, agent, &fakeCostQuerier{byAgent: map[string]float64{"frugal": 0.10}})
	mustCreateTemplate(t, tmplStore, templatepkg.Template{Name: "costly", MaxCostUSD: 2.50})

	checkAllGuardrails(context.Background(), svc, tmplStore, nil)

	got, err := svc.Get(context.Background(), "frugal")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != agentpkg.StateWorking {
		t.Fatalf("state = %s, want working (under budget must not be touched)", got.State)
	}
}

// TestGuardrail_StuckTimeout_FlagsAgent verifies a StateWorking agent whose
// last persisted event is older than StuckTimeoutMin is flagged StateStuck
// (not stopped).
func TestGuardrail_StuckTimeout_FlagsAgent(t *testing.T) {
	staleTime := time.Now().Add(-10 * time.Minute)
	agent := &agentpkg.Agent{
		Name: "hung", State: agentpkg.StateWorking, Template: "patient",
		StartedAt: staleTime, UpdatedAt: staleTime,
	}
	svc, tmplStore := guardrailTestSetup(t, agent, nil)
	mustCreateTemplate(t, tmplStore, templatepkg.Template{Name: "patient", StuckTimeoutMin: 5})

	events := &fakeEventStore{byAgent: map[string][]eventspkg.Event{
		"hung": {{Agent: "hung", Type: "hook.PostToolUse", Timestamp: staleTime}},
	}}

	checkAllGuardrails(context.Background(), svc, tmplStore, events)

	got, err := svc.Get(context.Background(), "hung")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != agentpkg.StateStuck {
		t.Fatalf("state = %s, want stuck", got.State)
	}
	if got.Task == "" {
		t.Fatal("expected task/reason to be recorded explaining the stuck flag")
	}
}

// TestGuardrail_StuckTimeout_FallsBackToUpdatedAt verifies that when no
// event log is available (e.g. degraded mode), the stuck check falls back
// to the agent's own UpdatedAt instead of silently never firing. This
// drives checkStuckGuardrail directly (rather than through
// checkAllGuardrails/List) because Save() always stamps updated_at with
// "now", so a stale UpdatedAt can't survive a seed-and-reload round trip —
// the fallback value has to come from the Agent snapshot the caller holds,
// exactly as production's checkAllGuardrails passes the List() snapshot.
func TestGuardrail_StuckTimeout_FallsBackToUpdatedAt(t *testing.T) {
	seed := &agentpkg.Agent{
		Name: "hung2", State: agentpkg.StateWorking, Template: "patient",
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	svc, _ := guardrailTestSetup(t, seed, nil)
	tmpl := &templatepkg.Template{Name: "patient", StuckTimeoutMin: 5}

	stale := &agentpkg.Agent{
		Name: "hung2", State: agentpkg.StateWorking,
		UpdatedAt: time.Now().Add(-10 * time.Minute),
	}
	checkStuckGuardrail(context.Background(), svc, nil, stale, tmpl)

	got, err := svc.Get(context.Background(), "hung2")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != agentpkg.StateStuck {
		t.Fatalf("state = %s, want stuck (fallback to UpdatedAt when no event log)", got.State)
	}
}

// TestGuardrail_ActivelyProgressing_NotFlaggedStuck verifies that a recent
// event in the event log overrides a stale Agent.UpdatedAt — an agent that
// is actively producing tool events must never be flagged stuck just
// because its lifecycle state hasn't transitioned recently.
func TestGuardrail_ActivelyProgressing_NotFlaggedStuck(t *testing.T) {
	staleTime := time.Now().Add(-10 * time.Minute)
	agent := &agentpkg.Agent{
		Name: "busy", State: agentpkg.StateWorking, Template: "patient",
		StartedAt: staleTime, UpdatedAt: staleTime,
	}
	svc, tmplStore := guardrailTestSetup(t, agent, nil)
	mustCreateTemplate(t, tmplStore, templatepkg.Template{Name: "patient", StuckTimeoutMin: 5})

	events := &fakeEventStore{byAgent: map[string][]eventspkg.Event{
		"busy": {{Agent: "busy", Type: "hook.PostToolUse", Timestamp: time.Now()}},
	}}

	checkAllGuardrails(context.Background(), svc, tmplStore, events)

	got, err := svc.Get(context.Background(), "busy")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != agentpkg.StateWorking {
		t.Fatalf("state = %s, want working (recent tool event must prevent a stuck flag)", got.State)
	}
}

// TestGuardrail_ZeroLimits_Untouched verifies that a template with both
// guardrails disabled (the Template zero value) never touches the agent,
// no matter how much it has spent or how long it has been idle.
func TestGuardrail_ZeroLimits_Untouched(t *testing.T) {
	staleTime := time.Now().Add(-24 * time.Hour)
	agent := &agentpkg.Agent{
		Name: "unlimited", State: agentpkg.StateWorking, Template: "no-limits",
		StartedAt: staleTime, UpdatedAt: staleTime,
	}
	svc, tmplStore := guardrailTestSetup(t, agent, &fakeCostQuerier{byAgent: map[string]float64{"unlimited": 999.99}})
	mustCreateTemplate(t, tmplStore, templatepkg.Template{Name: "no-limits"})

	checkAllGuardrails(context.Background(), svc, tmplStore, nil)

	got, err := svc.Get(context.Background(), "unlimited")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != agentpkg.StateWorking {
		t.Fatalf("state = %s, want working (zero limits must disable both guardrails)", got.State)
	}
}

// TestGuardrail_NoTemplate_Untouched verifies agents spawned without a
// template (Agent.Template == "") are never evaluated at all.
func TestGuardrail_NoTemplate_Untouched(t *testing.T) {
	staleTime := time.Now().Add(-24 * time.Hour)
	agent := &agentpkg.Agent{
		Name: "templateless", State: agentpkg.StateWorking,
		StartedAt: staleTime, UpdatedAt: staleTime,
	}
	svc, tmplStore := guardrailTestSetup(t, agent, &fakeCostQuerier{byAgent: map[string]float64{"templateless": 999.99}})

	checkAllGuardrails(context.Background(), svc, tmplStore, nil)

	got, err := svc.Get(context.Background(), "templateless")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != agentpkg.StateWorking {
		t.Fatalf("state = %s, want working (no template means no guardrails)", got.State)
	}
}
