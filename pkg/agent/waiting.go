// waiting.go — telling an agent that is working apart from one that has
// stopped to ask its user something.
//
// A provider holding a permission prompt or a choice menu open is neither
// working nor idle: it will do nothing at all until a person answers it, which
// is what stuck already means. mycel used to read such an agent as working —
// silently spending its template's StuckTimeoutMin until the guardrail loop
// noticed the silence half an hour later and blamed a timer for it (#3582).
//
// The signal is there for the asking. claude classifies its own Notification
// events, so which ones block on a person is its statement rather than mycel's
// guess; the events raised from an agent's terminal (see
// provider.QuestionDetector) say so by construction. Both arrive here.
package agent

import "strings"

// claude's notification_type values, as declared by the Notification hook
// input schema in claude-code 2.1.205:
//
//	{hook_event_name, message, title?, notification_type}
//
// Only the two that mean "this session is blocked until you answer" are named
// here, plus the two that mean the answer arrived. The rest are deliberately
// absent and stay informational — most importantly idle_prompt, which claude
// fires a minute after a turn ends. That agent is idle, has already reported
// Stop, and is waiting for work rather than for an answer; treating it as
// stuck would flag every quiet agent in the fleet and make the state
// meaningless.
const (
	notifyPermissionPrompt    = "permission_prompt"
	notifyElicitationDialog   = "elicitation_dialog"
	notifyElicitationComplete = "elicitation_complete"
	notifyElicitationResponse = "elicitation_response"
)

// humanWait is what a hook event says about whether its agent is blocked on a
// person.
type humanWait int

const (
	// humanWaitUnchanged means the event says nothing either way.
	humanWaitUnchanged humanWait = iota
	// humanWaitBlocked means the provider is holding a prompt open.
	humanWaitBlocked
	// humanWaitAnswered means the prompt it was holding is gone.
	humanWaitAnswered
)

// waitingReasonPrefix labels the agent's task line while it is blocked, so a
// question sitting where the task normally goes reads as a question rather
// than as work the agent claims to be doing.
const waitingReasonPrefix = "waiting for an answer"

// classifyHumanWait reports whether a hook event means its agent has stopped
// to ask its user something, and what it asked.
//
// The reason is returned only for humanWaitBlocked, where it becomes the
// agent's task line: knowing an agent needs you is worth much less than
// knowing what it wants, since the second tells you whether to answer now or
// after lunch.
func classifyHumanWait(payload HookPayload) (humanWait, string) {
	switch payload.Event {
	case HookAwaitingInput, HookPermissionRequest, HookElicitation:
		return humanWaitBlocked, waitingReason(payload.Message)
	case HookInputProvided, HookElicitationResult:
		return humanWaitAnswered, ""
	case HookNotification:
		switch payload.NotificationType {
		case notifyPermissionPrompt, notifyElicitationDialog:
			return humanWaitBlocked, waitingReason(payload.Message)
		case notifyElicitationComplete, notifyElicitationResponse:
			return humanWaitAnswered, ""
		}
	}
	return humanWaitUnchanged, ""
}

// waitingReason renders a provider's question as an agent task line, falling
// back to the bare label for a provider that reports being blocked without
// saying on what.
func waitingReason(question string) string {
	question = strings.TrimSpace(question)
	if question == "" {
		return waitingReasonPrefix
	}
	return truncateRunes(waitingReasonPrefix+": "+question, maxDerivedTaskLen)
}

// provesTurnInProgress reports whether an event could only have been emitted
// by a provider that is actively running a turn.
//
// This is the ordinary way out of stuck. Every event that follows an answered
// prompt — the tool the user just approved, the subagent it spawned — is one
// the state map treats as informational, so an agent flagged stuck used to
// stay stuck for the rest of its turn no matter how much work it did, and the
// guardrail loop's promise that "any further hook event flips it back to
// StateWorking" was not kept by anything. A CLI cannot run a tool while it is
// holding a prompt open, so these events are proof the question is settled.
func provesTurnInProgress(ev HookEvent) bool {
	switch ev {
	case HookPreToolUse, HookPostToolUse, HookPostToolUseFailure,
		HookSubagentStart, HookSubagentStop, HookPostInvocation:
		return true
	}
	return false
}
