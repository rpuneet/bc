package server

import (
	"context"
	"errors"
	"testing"
	"time"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
)

// fakeFailureDeps stands in for the agent service so a sweep can be driven
// without a tmux session.
type fakeFailureDeps struct {
	agents    []*agentpkg.Agent
	panes     map[string]string
	peekErr   map[string]error
	peeked    []string
	ingested  []agentpkg.HookPayload
	ingestErr error
	listErr   error
}

func (f *fakeFailureDeps) List(context.Context, agentpkg.ListOptions) ([]*agentpkg.Agent, error) {
	return f.agents, f.listErr
}

func (f *fakeFailureDeps) Peek(_ context.Context, name string, _ int) (string, error) {
	f.peeked = append(f.peeked, name)
	if err, ok := f.peekErr[name]; ok {
		return "", err
	}
	return f.panes[name], nil
}

func (f *fakeFailureDeps) IngestHookEvent(_ context.Context, _ string, p agentpkg.HookPayload, _ []byte) error {
	if f.ingestErr != nil {
		return f.ingestErr
	}
	f.ingested = append(f.ingested, p)
	return nil
}

// A pi agent that cannot reach its model, verbatim from #3512.
const brokenPiPane = ` Error: 404 The model ` + "`qwen/qwen3-32b`" + ` does not exist or you do not have access to it.
~/.mycel/agents/fierce-osprey/worktree (detached)`

func quietPiAgent(name string, state agentpkg.State, quietFor time.Duration, now time.Time) *agentpkg.Agent {
	return &agentpkg.Agent{
		Name:      name,
		Tool:      "pi",
		State:     state,
		UpdatedAt: now.Add(-quietFor),
	}
}

func TestSweepReportsAQuietAgentWhoseProviderCannotWork(t *testing.T) {
	now := time.Now()
	deps := &fakeFailureDeps{
		agents: []*agentpkg.Agent{quietPiAgent("osprey", agentpkg.StateWorking, time.Hour, now)},
		panes:  map[string]string{"osprey": brokenPiPane},
	}

	sweepForFailedProviders(context.Background(), deps, map[string]string{}, now)

	if len(deps.ingested) != 1 {
		t.Fatalf("ingested %d events, want 1", len(deps.ingested))
	}
	got := deps.ingested[0]
	if got.Event != agentpkg.HookProviderFailure {
		t.Errorf("event = %q, want %q", got.Event, agentpkg.HookProviderFailure)
	}
	if got.Error == "" {
		t.Error("the event must carry the reason — it is the only place the user can read it")
	}
	// The event has to move the agent out of a state that claims it is fine.
	if state, ok := agentpkg.StateForHookEvent(got.Event); !ok || state != agentpkg.StateError {
		t.Errorf("ProviderFailure maps to state %q (ok=%v), want %q", state, ok, agentpkg.StateError)
	}
}

func TestSweepLeavesBusyAgentsAlone(t *testing.T) {
	// An agent that reported something a moment ago is working. Reading its
	// terminal risks calling a healthy agent broken, and costs a capture per
	// agent per sweep for nothing.
	now := time.Now()
	deps := &fakeFailureDeps{
		agents: []*agentpkg.Agent{quietPiAgent("busy", agentpkg.StateWorking, 5*time.Second, now)},
		panes:  map[string]string{"busy": brokenPiPane},
	}

	sweepForFailedProviders(context.Background(), deps, map[string]string{}, now)

	if len(deps.peeked) != 0 {
		t.Errorf("peeked %v, want no terminal read for a recently active agent", deps.peeked)
	}
	if len(deps.ingested) != 0 {
		t.Errorf("ingested %d events, want none", len(deps.ingested))
	}
}

func TestSweepSkipsAgentsWithNothingToDiagnose(t *testing.T) {
	now := time.Now()
	for _, state := range []agentpkg.State{
		agentpkg.StateStopped,
		agentpkg.StateDone,
		agentpkg.StateError,
	} {
		t.Run(string(state), func(t *testing.T) {
			deps := &fakeFailureDeps{
				agents: []*agentpkg.Agent{quietPiAgent("a", state, time.Hour, now)},
				panes:  map[string]string{"a": brokenPiPane},
			}
			sweepForFailedProviders(context.Background(), deps, map[string]string{}, now)
			if len(deps.peeked) != 0 {
				t.Errorf("peeked a %s agent", state)
			}
		})
	}
}

func TestSweepReportsOneEventPerFailure(t *testing.T) {
	// A broken agent stays broken. Re-reporting it every 30 seconds would bury
	// the feed it was meant to explain.
	now := time.Now()
	deps := &fakeFailureDeps{
		agents: []*agentpkg.Agent{quietPiAgent("osprey", agentpkg.StateIdle, time.Hour, now)},
		panes:  map[string]string{"osprey": brokenPiPane},
	}
	reported := map[string]string{}

	sweepForFailedProviders(context.Background(), deps, reported, now)
	sweepForFailedProviders(context.Background(), deps, reported, now)
	sweepForFailedProviders(context.Background(), deps, reported, now)

	if len(deps.ingested) != 1 {
		t.Errorf("ingested %d events across three sweeps, want 1", len(deps.ingested))
	}
}

func TestSweepReportsAgainAfterRecovery(t *testing.T) {
	now := time.Now()
	deps := &fakeFailureDeps{
		agents: []*agentpkg.Agent{quietPiAgent("osprey", agentpkg.StateIdle, time.Hour, now)},
		panes:  map[string]string{"osprey": brokenPiPane},
	}
	reported := map[string]string{}

	sweepForFailedProviders(context.Background(), deps, reported, now)
	// The user fixes the model and the agent goes quiet again for other reasons.
	deps.panes["osprey"] = "● Read main.go\n> "
	sweepForFailedProviders(context.Background(), deps, reported, now)
	// It breaks a second time; the user has to hear about it again.
	deps.panes["osprey"] = brokenPiPane
	sweepForFailedProviders(context.Background(), deps, reported, now)

	if len(deps.ingested) != 2 {
		t.Errorf("ingested %d events, want 2 — one per failure episode", len(deps.ingested))
	}
}

func TestSweepRetriesWhenReportingFailed(t *testing.T) {
	// If the report itself did not land, the agent is still broken and still
	// unexplained, so the next sweep has to try again.
	now := time.Now()
	deps := &fakeFailureDeps{
		agents:    []*agentpkg.Agent{quietPiAgent("osprey", agentpkg.StateIdle, time.Hour, now)},
		panes:     map[string]string{"osprey": brokenPiPane},
		ingestErr: errors.New("store unavailable"),
	}
	reported := map[string]string{}

	sweepForFailedProviders(context.Background(), deps, reported, now)
	if len(reported) != 0 {
		t.Fatalf("remembered a report that failed to land: %v", reported)
	}

	deps.ingestErr = nil
	sweepForFailedProviders(context.Background(), deps, reported, now)
	if len(deps.ingested) != 1 {
		t.Errorf("ingested %d events, want the retry to succeed", len(deps.ingested))
	}
}

func TestSweepIgnoresAgentsWhoseTerminalCannotBeRead(t *testing.T) {
	// A container agent, or one whose session has gone: not evidence of a
	// provider failure.
	now := time.Now()
	deps := &fakeFailureDeps{
		agents:  []*agentpkg.Agent{quietPiAgent("gone", agentpkg.StateIdle, time.Hour, now)},
		peekErr: map[string]error{"gone": errors.New("no such session")},
	}

	sweepForFailedProviders(context.Background(), deps, map[string]string{}, now)

	if len(deps.ingested) != 0 {
		t.Errorf("ingested %d events for an unreadable terminal, want none", len(deps.ingested))
	}
}

func TestSweepIgnoresProvidersWithoutADetector(t *testing.T) {
	now := time.Now()
	a := quietPiAgent("claude-agent", agentpkg.StateIdle, time.Hour, now)
	a.Tool = "claude"
	deps := &fakeFailureDeps{
		agents: []*agentpkg.Agent{a},
		panes:  map[string]string{"claude-agent": brokenPiPane},
	}

	sweepForFailedProviders(context.Background(), deps, map[string]string{}, now)

	if len(deps.peeked) != 0 {
		t.Errorf("peeked %v, want no read for a provider that declares no patterns", deps.peeked)
	}
}

func TestSweepForgetsAgentsThatNoLongerExist(t *testing.T) {
	now := time.Now()
	deps := &fakeFailureDeps{}
	reported := map[string]string{"deleted": "some old reason"}

	sweepForFailedProviders(context.Background(), deps, reported, now)

	if _, ok := reported["deleted"]; ok {
		t.Error("bookkeeping for a deleted agent must not be kept forever")
	}
}
