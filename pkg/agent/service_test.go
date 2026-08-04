package agent

import (
	"context"
	"strings"
	"testing"
	"time"
)

// mockEventPublisher records published events.
type mockEventPublisher struct {
	events []publishedEvent
}

type publishedEvent struct {
	data      map[string]any
	eventType string
}

func (m *mockEventPublisher) Publish(eventType string, data map[string]any) {
	m.events = append(m.events, publishedEvent{eventType: eventType, data: data})
}

// mockCostQuerier returns fixed cost data.
type mockCostQuerier struct {
	summary *CostSummary
}

func (m *mockCostQuerier) AgentCostSummary(agentID string) (*CostSummary, error) {
	if m.summary != nil {
		return m.summary, nil
	}
	return &CostSummary{AgentID: agentID}, nil
}

func TestAgentService_ListEmpty(t *testing.T) {
	mgr := newTestManager(t)
	svc := NewAgentService(mgr, nil, nil)

	agents, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestAgentService_ListWithFilters(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateIdle, Children: []string{}}
	mgr.agents["eng-2"] = &Agent{Name: "eng-2", Role: Role("engineer"), State: StateStopped, Children: []string{}}
	mgr.agents["qa-1"] = &Agent{Name: "qa-1", Role: Role("qa"), State: StateWorking, Children: []string{}}

	svc := NewAgentService(mgr, nil, nil)

	t.Run("filter by role", func(t *testing.T) {
		agents, err := svc.List(context.Background(), ListOptions{Role: "engineer"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(agents) != 2 {
			t.Errorf("expected 2 engineers, got %d", len(agents))
		}
	})

	t.Run("filter by status running", func(t *testing.T) {
		agents, err := svc.List(context.Background(), ListOptions{Status: "running"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(agents) != 2 {
			t.Errorf("expected 2 running agents, got %d", len(agents))
		}
	})

	t.Run("filter by status stopped", func(t *testing.T) {
		agents, err := svc.List(context.Background(), ListOptions{Status: "stopped"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(agents) != 1 {
			t.Errorf("expected 1 stopped agent, got %d", len(agents))
		}
	})

	t.Run("filter by role and status", func(t *testing.T) {
		agents, err := svc.List(context.Background(), ListOptions{Role: "engineer", Status: "running"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(agents) != 1 {
			t.Errorf("expected 1 running engineer, got %d", len(agents))
		}
	})
}

func TestAgentService_StopNonexistent(t *testing.T) {
	mgr := newTestManager(t)
	svc := NewAgentService(mgr, nil, nil)

	err := svc.Stop(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

// TestAgentService_StopForGuardrail verifies the guardrail-specific stop
// path: the agent is stopped, the reason is recorded as its Task (visible
// in the UI even after the agent is gone), and the published event tags
// the reason as "guardrail" rather than "user_request" so the two are
// distinguishable in the activity timeline.
func TestAgentService_StopForGuardrail(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateWorking, Children: []string{}}

	pub := &mockEventPublisher{}
	svc := NewAgentService(mgr, pub, nil)

	const reason = "guardrail: cost limit $2.50 reached ($5.00 spent), stopped"
	if err := svc.StopForGuardrail(context.Background(), "eng-1", reason); err != nil {
		t.Fatalf("StopForGuardrail: %v", err)
	}

	if mgr.agents["eng-1"].State != StateStopped {
		t.Fatalf("state = %s, want stopped", mgr.agents["eng-1"].State)
	}
	if mgr.agents["eng-1"].Task != reason {
		t.Fatalf("task = %q, want %q", mgr.agents["eng-1"].Task, reason)
	}

	var found bool
	for _, e := range pub.events {
		if e.eventType != "agent.stopped" {
			continue
		}
		found = true
		if e.data["reason"] != "guardrail" {
			t.Errorf("published reason = %v, want %q", e.data["reason"], "guardrail")
		}
		if e.data["detail"] != reason {
			t.Errorf("published detail = %v, want %q", e.data["detail"], reason)
		}
	}
	if !found {
		t.Fatal("expected an agent.stopped event to be published")
	}
}

func TestAgentService_DeleteReconcilesDead(t *testing.T) {
	// An agent with StateIdle but no actual session should be reconciled
	// to StateStopped and deleted successfully (not stuck in catch-22).
	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateIdle, Children: []string{}}

	pub := &mockEventPublisher{}
	svc := NewAgentService(mgr, pub, nil)

	// Delete should succeed because no session exists (container dead)
	err := svc.Delete(context.Background(), "eng-1", false)
	if err != nil {
		t.Fatalf("Delete of dead agent should succeed after reconciliation: %v", err)
	}
}

func TestAgentService_DeleteStopped(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateStopped, Children: []string{}}

	pub := &mockEventPublisher{}
	svc := NewAgentService(mgr, pub, nil)

	err := svc.Delete(context.Background(), "eng-1", false)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify event published
	if len(pub.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(pub.events))
	}
	if pub.events[0].eventType != "agent.deleted" {
		t.Errorf("event type = %q, want agent.deleted", pub.events[0].eventType)
	}
}

func TestAgentService_SendToStopped(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateStopped}

	svc := NewAgentService(mgr, nil, nil)

	err := svc.Send(context.Background(), "eng-1", "hello")
	if err == nil {
		t.Error("expected error when sending to stopped agent")
	}
}

func TestAgentService_SendToNonexistent(t *testing.T) {
	mgr := newTestManager(t)
	svc := NewAgentService(mgr, nil, nil)

	err := svc.Send(context.Background(), "nonexistent", "hello")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestAgentService_PeekNonexistent(t *testing.T) {
	mgr := newTestManager(t)
	svc := NewAgentService(mgr, nil, nil)

	_, err := svc.Peek(context.Background(), "nonexistent", 50)
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestAgentService_CostNil(t *testing.T) {
	mgr := newTestManager(t)
	svc := NewAgentService(mgr, nil, nil) // no cost querier

	_, err := svc.Cost(context.Background(), "eng-1")
	if err == nil {
		t.Error("expected error when cost tracking not configured")
	}
}

func TestAgentService_CostQuerier(t *testing.T) {
	mgr := newTestManager(t)
	cq := &mockCostQuerier{summary: &CostSummary{
		AgentID:      "eng-1",
		TotalCostUSD: 1.50,
		RequestCount: 10,
	}}
	svc := NewAgentService(mgr, nil, cq)

	summary, err := svc.Cost(context.Background(), "eng-1")
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if summary.TotalCostUSD != 1.50 {
		t.Errorf("TotalCostUSD = %f, want 1.50", summary.TotalCostUSD)
	}
}

func TestAgentService_Broadcast(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateIdle}
	mgr.agents["eng-2"] = &Agent{Name: "eng-2", Role: Role("engineer"), State: StateStopped}
	mgr.agents["qa-1"] = &Agent{Name: "qa-1", Role: Role("qa"), State: StateWorking}

	svc := NewAgentService(mgr, nil, nil)

	// Broadcast will try to send to eng-1 and qa-1 (skip stopped eng-2)
	// SendToAgent will fail because there are no real tmux sessions, but
	// we're testing the filtering logic
	sent, err := svc.Broadcast(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	// Both will fail since no tmux sessions, so sent should be 0
	_ = sent
}

func TestMatchesStatus(t *testing.T) {
	tests := []struct {
		state  State
		status string
		want   bool
	}{
		{StateIdle, "running", true},
		{StateWorking, "running", true},
		{StateStarting, "running", true},
		{StateStopped, "running", false},
		{StateError, "running", false},
		{StateStopped, "stopped", true},
		{StateIdle, "stopped", false},
		{StateError, "error", true},
		{StateIdle, "error", false},
		{StateStarting, "starting", true},
		{StateIdle, "idle", true},       // exact match
		{StateWorking, "working", true}, // exact match
	}

	for _, tt := range tests {
		t.Run(string(tt.state)+"_"+tt.status, func(t *testing.T) {
			if got := matchesStatus(tt.state, tt.status); got != tt.want {
				t.Errorf("matchesStatus(%q, %q) = %v, want %v", tt.state, tt.status, got, tt.want)
			}
		})
	}
}

func TestAgentService_Get(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateIdle, Children: []string{}}

	svc := NewAgentService(mgr, nil, nil)

	t.Run("found", func(t *testing.T) {
		a, err := svc.Get(context.Background(), "eng-1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if a.Name != "eng-1" {
			t.Errorf("Name = %q, want eng-1", a.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := svc.Get(context.Background(), "nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent agent")
		}
	})
}

func TestAgentService_Manager(t *testing.T) {
	mgr := newTestManager(t)
	svc := NewAgentService(mgr, nil, nil)

	if svc.Manager() != mgr {
		t.Error("Manager() should return the underlying manager")
	}
}

// TestAgentService_ArchiveRoundtrip confirms Archive/Unarchive flip the
// ArchivedAt field and interact correctly with List's default filter.
func TestAgentService_ArchiveRoundtrip(t *testing.T) {
	mgr := newTestManager(t)
	// Archive requires a non-running state — the service refuses to
	// archive agents that are idle/starting/working.
	mgr.agents["keep"] = &Agent{Name: "keep", Role: Role("engineer"), State: StateStopped, Children: []string{}}
	mgr.agents["away"] = &Agent{Name: "away", Role: Role("engineer"), State: StateStopped, Children: []string{}}
	svc := NewAgentService(mgr, nil, nil)
	ctx := context.Background()

	if err := svc.Archive(ctx, "away"); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	// Default list hides archived
	def, _ := svc.List(ctx, ListOptions{})
	if len(def) != 1 || def[0].Name != "keep" {
		t.Errorf("default list: expected [keep], got %+v", def)
	}

	// IncludeArchived sees both
	both, _ := svc.List(ctx, ListOptions{IncludeArchived: true})
	if len(both) != 2 {
		t.Errorf("IncludeArchived list: expected 2, got %d", len(both))
	}

	// OnlyArchived returns just away
	only, _ := svc.List(ctx, ListOptions{OnlyArchived: true})
	if len(only) != 1 || only[0].Name != "away" {
		t.Errorf("OnlyArchived list: expected [away], got %+v", only)
	}

	// Idempotent double-archive
	if err := svc.Archive(ctx, "away"); err != nil {
		t.Errorf("second archive should be idempotent, got %v", err)
	}

	// Unarchive flips back
	if err := svc.Unarchive(ctx, "away"); err != nil {
		t.Fatalf("Unarchive: %v", err)
	}
	def2, _ := svc.List(ctx, ListOptions{})
	if len(def2) != 2 {
		t.Errorf("after unarchive: expected 2, got %d", len(def2))
	}

	// Archiving a missing agent errors
	if err := svc.Archive(ctx, "ghost"); err == nil {
		t.Error("expected error archiving unknown agent")
	}
}

// TestAgentService_ArchiveRefusesRunning confirms the service rejects
// attempts to archive an agent that is still running — callers must
// stop it first.
func TestAgentService_ArchiveRefusesRunning(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["busy-idle"] = &Agent{Name: "busy-idle", Role: Role("engineer"), State: StateIdle, Children: []string{}}
	mgr.agents["busy-working"] = &Agent{Name: "busy-working", Role: Role("engineer"), State: StateWorking, Children: []string{}}
	mgr.agents["booting"] = &Agent{Name: "booting", Role: Role("engineer"), State: StateStarting, Children: []string{}}
	mgr.agents["stopped"] = &Agent{Name: "stopped", Role: Role("engineer"), State: StateStopped, Children: []string{}}
	svc := NewAgentService(mgr, nil, nil)
	ctx := context.Background()

	for _, name := range []string{"busy-idle", "busy-working", "booting"} {
		err := svc.Archive(ctx, name)
		if err == nil {
			t.Errorf("expected error archiving running agent %q, got nil", name)
			continue
		}
		if !strings.Contains(err.Error(), "while running") {
			t.Errorf("agent %q: expected 'while running' error, got %v", name, err)
		}
		// Ensure the failed Archive did NOT set ArchivedAt.
		if mgr.agents[name].ArchivedAt != nil {
			t.Errorf("agent %q: ArchivedAt was set despite failed Archive", name)
		}
	}

	// Stopped agent archives cleanly.
	if err := svc.Archive(ctx, "stopped"); err != nil {
		t.Errorf("expected Archive on stopped agent to succeed, got %v", err)
	}
	if mgr.agents["stopped"].ArchivedAt == nil {
		t.Error("stopped agent should have ArchivedAt set after Archive")
	}
}

// SyncSessions is what stops an agent being reported as working when nothing is
// running behind it. The test manager's tmux prefix is unique per run, so no
// agent here has a session — which is exactly the condition being reconciled.
func TestSyncSessionsStopsAnAgentWithNoSession(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["working"] = &Agent{Name: "working", State: StateWorking, Task: "refactoring", Children: []string{}}
	mgr.agents["idle"] = &Agent{Name: "idle", State: StateIdle, Children: []string{}}
	mgr.agents["stuck"] = &Agent{Name: "stuck", State: StateStuck, Children: []string{}}

	pub := &mockEventPublisher{}
	svc := NewAgentService(mgr, pub, nil)

	synced, stopped := svc.SyncSessions(context.Background())
	if synced != 3 || stopped != 3 {
		t.Fatalf("synced=%d stopped=%d, want 3 and 3", synced, stopped)
	}
	for _, name := range []string{"working", "idle", "stuck"} {
		if got := mgr.GetAgent(name).State; got != StateStopped {
			t.Errorf("%s state = %q, want stopped", name, got)
		}
	}
	// The reason has to be somewhere the user looks, not only in an event.
	if task := mgr.GetAgent("working").Task; !strings.Contains(task, "session ended") {
		t.Errorf("task = %q, want it to explain that the session ended", task)
	}
}

// Terminal states are the agent's own account of how it finished. Stopped and
// error have nothing to reconcile, and overwriting done with stopped would
// replace "it completed" with "its session is gone", which says less.
func TestSyncSessionsLeavesTerminalStatesAlone(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["done"] = &Agent{Name: "done", State: StateDone, Children: []string{}}
	mgr.agents["failed"] = &Agent{Name: "failed", State: StateError, Children: []string{}}
	mgr.agents["halted"] = &Agent{Name: "halted", State: StateStopped, Children: []string{}}

	svc := NewAgentService(mgr, nil, nil)

	synced, stopped := svc.SyncSessions(context.Background())
	if synced != 0 || stopped != 0 {
		t.Errorf("synced=%d stopped=%d, want 0 and 0", synced, stopped)
	}
	if got := mgr.GetAgent("done").State; got != StateDone {
		t.Errorf("done agent became %q", got)
	}
}

// An agent is registered before its session exists, so a sweep landing in that
// window must not stop an agent that is still starting.
func TestSyncSessionsSparesAnAgentThatJustStarted(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["starting"] = &Agent{
		Name:      "starting",
		State:     StateWorking,
		StartedAt: time.Now(),
		Children:  []string{},
	}

	svc := NewAgentService(mgr, nil, nil)

	if synced, stopped := svc.SyncSessions(context.Background()); synced != 0 || stopped != 0 {
		t.Errorf("synced=%d stopped=%d, want the starting agent skipped", synced, stopped)
	}
	if got := mgr.GetAgent("starting").State; got != StateWorking {
		t.Errorf("state = %q, want working", got)
	}
}
