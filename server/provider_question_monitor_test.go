package server

import (
	"context"
	"testing"
	"time"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
)

// fakeQuestionDeps stands in for the agent service so a sweep can be driven
// without a tmux session.
type fakeQuestionDeps struct {
	panes    map[string]string
	agents   []*agentpkg.Agent
	peeked   []string
	ingested []agentpkg.HookPayload
}

func (f *fakeQuestionDeps) List(context.Context, agentpkg.ListOptions) ([]*agentpkg.Agent, error) {
	return f.agents, nil
}

func (f *fakeQuestionDeps) Peek(_ context.Context, name string, _ int) (string, error) {
	f.peeked = append(f.peeked, name)
	return f.panes[name], nil
}

func (f *fakeQuestionDeps) IngestHookEvent(_ context.Context, _ string, p agentpkg.HookPayload, _ []byte) error {
	f.ingested = append(f.ingested, p)
	return nil
}

// The bottom of a cursor screen whose input box is waiting for an answer.
const cursorAskingPane = ` ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄
  → Answer questions (Enter to select/next, Esc to skip)
 ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
  ~/.mycel/agents/eager-fox/worktree · main
`

// The same screen once someone has answered.
const cursorResumedPane = ` ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄
  → Add a follow-up                        ctrl+c to stop
 ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
  ~/.mycel/agents/eager-fox/worktree · main
`

func quietCursorAgent(name string, state agentpkg.State, quietFor time.Duration, now time.Time) *agentpkg.Agent {
	return &agentpkg.Agent{
		Name:      name,
		Tool:      "cursor",
		State:     state,
		UpdatedAt: now.Add(-quietFor),
	}
}

func TestSweepFlagsAnAgentWaitingForAnAnswer(t *testing.T) {
	now := time.Now()
	deps := &fakeQuestionDeps{
		agents: []*agentpkg.Agent{quietCursorAgent("fox", agentpkg.StateWorking, 5*time.Minute, now)},
		panes:  map[string]string{"fox": cursorAskingPane},
	}

	sweepForOpenQuestions(context.Background(), deps, map[string]string{}, now)

	if len(deps.ingested) != 1 {
		t.Fatalf("ingested %d events, want 1", len(deps.ingested))
	}
	got := deps.ingested[0]
	if got.Event != agentpkg.HookAwaitingInput {
		t.Errorf("event = %q, want %q", got.Event, agentpkg.HookAwaitingInput)
	}
	if got.Message == "" {
		t.Error("the event must carry the question — knowing what is asked is the point")
	}
	if state, ok := agentpkg.StateForHookEvent(got.Event); !ok || state != agentpkg.StateStuck {
		t.Errorf("AwaitingInput maps to state %q (ok=%v), want %q", state, ok, agentpkg.StateStuck)
	}
}

// An agent that gets flagged and never unflagged is worse than one that is
// never flagged: the state stops meaning anything.
func TestSweepReleasesAnAgentOnceItsQuestionIsAnswered(t *testing.T) {
	now := time.Now()
	agent := quietCursorAgent("fox", agentpkg.StateWorking, 5*time.Minute, now)
	deps := &fakeQuestionDeps{
		agents: []*agentpkg.Agent{agent},
		panes:  map[string]string{"fox": cursorAskingPane},
	}
	asked := map[string]string{}

	sweepForOpenQuestions(context.Background(), deps, asked, now)
	if len(asked) != 1 {
		t.Fatalf("the agent was not flagged: %v", asked)
	}

	// Someone answers. Flagging moved the agent's own clock, so this is also
	// the case where the quiet gate would hide the recovery if it applied.
	agent.State = agentpkg.StateStuck
	agent.UpdatedAt = now
	deps.panes["fox"] = cursorResumedPane

	sweepForOpenQuestions(context.Background(), deps, asked, now)

	if len(asked) != 0 {
		t.Errorf("the agent is still flagged as waiting: %v", asked)
	}
	if len(deps.ingested) != 2 {
		t.Fatalf("ingested %d events, want 2", len(deps.ingested))
	}
	release := deps.ingested[1]
	if release.Event != agentpkg.HookInputProvided {
		t.Errorf("event = %q, want %q", release.Event, agentpkg.HookInputProvided)
	}
	if state, ok := agentpkg.StateForHookEvent(release.Event); !ok || state != agentpkg.StateWorking {
		t.Errorf("InputProvided maps to state %q (ok=%v), want %q", state, ok, agentpkg.StateWorking)
	}
}

// Silence is the cheap gate and the pane is the expensive, low-confidence one.
// An agent that is still reporting activity is never read at all, which is most
// of what keeps a busy agent from being flagged over something in its output.
func TestSweepLeavesActiveAgentsAlone(t *testing.T) {
	now := time.Now()
	deps := &fakeQuestionDeps{
		agents: []*agentpkg.Agent{quietCursorAgent("fox", agentpkg.StateWorking, 5*time.Second, now)},
		panes:  map[string]string{"fox": cursorAskingPane},
	}

	sweepForOpenQuestions(context.Background(), deps, map[string]string{}, now)

	if len(deps.peeked) != 0 {
		t.Errorf("peeked %v, want an agent reporting activity to be left alone", deps.peeked)
	}
	if len(deps.ingested) != 0 {
		t.Errorf("ingested %d events, want none", len(deps.ingested))
	}
}

// One open prompt is one event, however long it stays open.
func TestSweepReportsTheSameQuestionOnce(t *testing.T) {
	now := time.Now()
	agent := quietCursorAgent("fox", agentpkg.StateWorking, 5*time.Minute, now)
	deps := &fakeQuestionDeps{
		agents: []*agentpkg.Agent{agent},
		panes:  map[string]string{"fox": cursorAskingPane},
	}
	asked := map[string]string{}

	sweepForOpenQuestions(context.Background(), deps, asked, now)
	agent.State = agentpkg.StateStuck
	sweepForOpenQuestions(context.Background(), deps, asked, now)
	sweepForOpenQuestions(context.Background(), deps, asked, now)

	if len(deps.ingested) != 1 {
		t.Errorf("ingested %d events, want 1", len(deps.ingested))
	}
}

// The state machine has no starting → stuck edge, and an agent that has not
// finished booting has not asked anything.
func TestSweepSkipsStatesThatCannotBeWaiting(t *testing.T) {
	now := time.Now()
	for _, state := range []agentpkg.State{
		agentpkg.StateStarting, agentpkg.StateStopped, agentpkg.StateDone, agentpkg.StateError,
	} {
		t.Run(string(state), func(t *testing.T) {
			deps := &fakeQuestionDeps{
				agents: []*agentpkg.Agent{quietCursorAgent("fox", state, time.Hour, now)},
				panes:  map[string]string{"fox": cursorAskingPane},
			}
			sweepForOpenQuestions(context.Background(), deps, map[string]string{}, now)
			if len(deps.ingested) != 0 {
				t.Errorf("ingested %d events for a %s agent, want none", len(deps.ingested), state)
			}
		})
	}
}

// Providers that report waiting through a hook must never have their screens
// read: the hook is the better signal and reading as well would double-report.
func TestSweepIgnoresProvidersWithoutAQuestionDetector(t *testing.T) {
	now := time.Now()
	deps := &fakeQuestionDeps{
		agents: []*agentpkg.Agent{{
			Name: "cheetah", Tool: "claude",
			State: agentpkg.StateWorking, UpdatedAt: now.Add(-time.Hour),
		}},
		panes: map[string]string{"cheetah": cursorAskingPane},
	}

	sweepForOpenQuestions(context.Background(), deps, map[string]string{}, now)

	if len(deps.peeked) != 0 {
		t.Errorf("peeked %v, want claude left to its hooks", deps.peeked)
	}
}
