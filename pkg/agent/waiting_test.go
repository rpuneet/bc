package agent

import (
	"context"
	"strings"
	"testing"
)

// serviceWithAgent builds a service around a single agent in a known state,
// which is all the ingestion path needs to be driven.
func serviceWithAgent(state State, task string) (*AgentService, *Manager) {
	mgr := &Manager{agents: map[string]*Agent{
		"eng-1": {Name: "eng-1", Role: Role("engineer"), State: state, Task: task},
	}}
	return NewAgentService(mgr, nil, nil), mgr
}

// The bug: claude reports "I cannot continue without you" through its
// Notification hook, mycel registered that hook, ingested it, and threw it
// away. The agent kept reading as working while it sat there.
func TestIngestHookEvent_PermissionNotificationMarksAgentStuck(t *testing.T) {
	svc, mgr := serviceWithAgent(StateWorking, "implementing issue #3582")

	payload := HookPayload{
		Event:            HookNotification,
		NotificationType: notifyPermissionPrompt,
		Message:          "Claude needs your permission to use Bash",
	}
	if err := svc.IngestHookEvent(context.Background(), "eng-1", payload, nil); err != nil {
		t.Fatalf("IngestHookEvent: %v", err)
	}

	got := mgr.agents["eng-1"]
	if got.State != StateStuck {
		t.Errorf("state = %q, want %q — an agent waiting on permission is not working", got.State, StateStuck)
	}
	if !strings.Contains(got.Task, "permission to use Bash") {
		t.Errorf("task = %q, want the question the agent is blocked on", got.Task)
	}
}

// A notification that is not a question must stay informational. idle_prompt is
// the one that matters: claude fires it a minute after every turn ends, so
// treating it as stuck would flag the whole fleet.
func TestIngestHookEvent_InformationalNotificationLeavesStateAlone(t *testing.T) {
	for _, notificationType := range []string{"idle_prompt", "auth_success", "agent_completed", ""} {
		t.Run(notificationType, func(t *testing.T) {
			svc, mgr := serviceWithAgent(StateIdle, "implementing issue #3582")

			payload := HookPayload{
				Event:            HookNotification,
				NotificationType: notificationType,
				Message:          "Claude is waiting for your input",
			}
			if err := svc.IngestHookEvent(context.Background(), "eng-1", payload, nil); err != nil {
				t.Fatalf("IngestHookEvent: %v", err)
			}

			got := mgr.agents["eng-1"]
			if got.State != StateIdle {
				t.Errorf("state = %q, want %q — this notification asks nothing of anyone", got.State, StateIdle)
			}
			if got.Task != "implementing issue #3582" {
				t.Errorf("task = %q, want the real task untouched", got.Task)
			}
		})
	}
}

// An agent that gets stuck and never recovers is worse than one that never
// showed stuck: the answer arrives, the CLI runs the tool it asked about, and
// nothing in the events that follow used to move it back.
func TestIngestHookEvent_AnsweredQuestionReturnsAgentToWorking(t *testing.T) {
	svc, mgr := serviceWithAgent(StateWorking, "implementing issue #3582")
	ctx := context.Background()

	ask := HookPayload{
		Event:            HookNotification,
		NotificationType: notifyPermissionPrompt,
		Message:          "Claude needs your permission to use Bash",
	}
	if err := svc.IngestHookEvent(ctx, "eng-1", ask, nil); err != nil {
		t.Fatalf("IngestHookEvent(ask): %v", err)
	}
	if mgr.agents["eng-1"].State != StateStuck {
		t.Fatalf("state = %q, want the agent stuck before it is answered", mgr.agents["eng-1"].State)
	}

	// Permission granted: claude runs the tool it was asking about. That event
	// carries no state of its own, and it is the only thing the agent sends.
	if err := svc.IngestHookEvent(ctx, "eng-1", HookPayload{Event: HookPreToolUse, ToolName: "Bash"}, nil); err != nil {
		t.Fatalf("IngestHookEvent(resume): %v", err)
	}
	if got := mgr.agents["eng-1"].State; got != StateWorking {
		t.Errorf("state = %q, want %q — the tool it asked about is running", got, StateWorking)
	}
}

// The pane monitor's own way out, for an answer that leads to something quiet
// enough that no tool event follows.
func TestIngestHookEvent_InputProvidedReleasesStuckAgent(t *testing.T) {
	svc, mgr := serviceWithAgent(StateStuck, "waiting for an answer: pick a branch")

	if err := svc.IngestHookEvent(context.Background(), "eng-1", HookPayload{Event: HookInputProvided}, nil); err != nil {
		t.Fatalf("IngestHookEvent: %v", err)
	}
	if got := mgr.agents["eng-1"].State; got != StateWorking {
		t.Errorf("state = %q, want %q", got, StateWorking)
	}
}

// A tool event must not drag an idle or done agent into working — it is only
// evidence about an agent that was flagged stuck.
func TestIngestHookEvent_ToolEventDoesNotDisturbUnflaggedAgents(t *testing.T) {
	for _, state := range []State{StateIdle, StateDone} {
		t.Run(string(state), func(t *testing.T) {
			svc, mgr := serviceWithAgent(state, "implementing issue #3582")

			if err := svc.IngestHookEvent(context.Background(), "eng-1", HookPayload{Event: HookPostToolUse}, nil); err != nil {
				t.Fatalf("IngestHookEvent: %v", err)
			}
			if got := mgr.agents["eng-1"].State; got != state {
				t.Errorf("state = %q, want %q unchanged", got, state)
			}
		})
	}
}

func TestClassifyHumanWait(t *testing.T) {
	tests := []struct {
		name    string
		payload HookPayload
		want    humanWait
	}{
		{"permission prompt", HookPayload{Event: HookNotification, NotificationType: notifyPermissionPrompt}, humanWaitBlocked},
		{"elicitation dialog", HookPayload{Event: HookNotification, NotificationType: notifyElicitationDialog}, humanWaitBlocked},
		{"elicitation answered", HookPayload{Event: HookNotification, NotificationType: notifyElicitationComplete}, humanWaitAnswered},
		{"idle prompt", HookPayload{Event: HookNotification, NotificationType: "idle_prompt"}, humanWaitUnchanged},
		{"teammate needs input", HookPayload{Event: HookNotification, NotificationType: "agent_needs_input"}, humanWaitUnchanged},
		{"permission request hook", HookPayload{Event: HookPermissionRequest}, humanWaitBlocked},
		{"pane detector", HookPayload{Event: HookAwaitingInput, Message: "pick a branch"}, humanWaitBlocked},
		{"tool call", HookPayload{Event: HookPreToolUse}, humanWaitUnchanged},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := classifyHumanWait(tt.payload); got != tt.want {
				t.Errorf("classifyHumanWait = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWaitingReason(t *testing.T) {
	if got := waitingReason("  Claude needs your permission to use Bash  "); got != "waiting for an answer: Claude needs your permission to use Bash" {
		t.Errorf("waitingReason = %q", got)
	}
	if got := waitingReason(""); got != waitingReasonPrefix {
		t.Errorf("waitingReason(empty) = %q, want %q", got, waitingReasonPrefix)
	}
	// The reason lands in a task line, which is one row of a table.
	long := waitingReason(strings.Repeat("why", 200))
	if len([]rune(long)) > maxDerivedTaskLen {
		t.Errorf("reason is %d runes, want at most %d", len([]rune(long)), maxDerivedTaskLen)
	}
}
