package agent

// HookEvent is a lifecycle event type — either a Claude Code hook or a mycel-internal event.
type HookEvent string

// ── Claude Code hook events (configured in .claude/settings.json) ──

const (
	HookSessionStart       HookEvent = "SessionStart"
	HookSessionEnd         HookEvent = "SessionEnd"
	HookUserPromptSubmit   HookEvent = "UserPromptSubmit"
	HookPreToolUse         HookEvent = "PreToolUse"
	HookPostToolUse        HookEvent = "PostToolUse"
	HookPostToolUseFailure HookEvent = "PostToolUseFailure"
	HookPermissionRequest  HookEvent = "PermissionRequest"
	HookStop               HookEvent = "Stop"
	HookNotification       HookEvent = "Notification"
	HookSubagentStart      HookEvent = "SubagentStart"
	HookSubagentStop       HookEvent = "SubagentStop"
	HookTaskCompleted      HookEvent = "TaskCompleted"
	HookTeammateIdle       HookEvent = "TeammateIdle"
	HookInstructionsLoaded HookEvent = "InstructionsLoaded"
	HookConfigChange       HookEvent = "ConfigChange"
	HookWorktreeCreate     HookEvent = "WorktreeCreate"
	HookWorktreeRemove     HookEvent = "WorktreeRemove"
	HookPreCompact         HookEvent = "PreCompact"
	HookPostCompact        HookEvent = "PostCompact"
	HookElicitation        HookEvent = "Elicitation"
	HookElicitationResult  HookEvent = "ElicitationResult"
)

// ── Antigravity CLI (agy) lifecycle hook events (configured in .agents/hooks.json) ──
//
// agy's tool events (PreToolUse, PostToolUse) and loop-termination event (Stop)
// reuse the Claude event names above. These two are agy-specific: they wrap the
// model invocation, so PreInvocation marks the agent as working and
// PostInvocation is informational.
const (
	HookPreInvocation  HookEvent = "PreInvocation"
	HookPostInvocation HookEvent = "PostInvocation"
)

// ── mycel-internal events (POSTed by the daemon Go code, not Claude Code hooks) ──

const (
	HookChannelMessage HookEvent = "ChannelMessage"
	HookChannelSent    HookEvent = "ChannelSent"
	HookAgentMessage   HookEvent = "AgentMessage"
	HookCostUpdate     HookEvent = "CostUpdate"
)

// hookEventStateMap maps hook events to the target agent state.
// Only events that represent genuine state transitions are included.
// Tool-level events (PreToolUse, PostToolUse, etc.) are informational
// and do NOT change agent state — the agent stays "working" from
// UserPromptSubmit until Stop/SessionEnd.
var hookEventStateMap = map[HookEvent]State{
	HookSessionStart:      StateIdle,
	HookSessionEnd:        StateStopped,
	HookUserPromptSubmit:  StateWorking,
	HookPreInvocation:     StateWorking,
	HookPermissionRequest: StateStuck,
	HookElicitation:       StateStuck,
	HookElicitationResult: StateWorking,
	HookStop:              StateIdle,
	HookTaskCompleted:     StateDone,
}

// StateForHookEvent returns the target agent State for a hook event.
// Returns false if the event doesn't trigger a state change (informational events).
func StateForHookEvent(ev HookEvent) (State, bool) {
	s, ok := hookEventStateMap[ev]
	return s, ok
}

// IsKnownEvent returns true if the event type is recognized (even if informational).
func IsKnownEvent(ev HookEvent) bool {
	if _, ok := hookEventStateMap[ev]; ok {
		return true
	}
	// Events that are known but don't change agent state (logged for activity tracking)
	switch ev {
	case HookPreToolUse, HookPostToolUse, HookPostToolUseFailure,
		HookPostInvocation,
		HookSubagentStart, HookSubagentStop,
		HookWorktreeCreate, HookPreCompact, HookPostCompact,
		HookNotification, HookTeammateIdle, HookInstructionsLoaded,
		HookConfigChange, HookWorktreeRemove,
		HookChannelMessage, HookChannelSent, HookAgentMessage, HookCostUpdate:
		return true
	}
	return false
}

// HookPayload is the JSON payload received by the /hook endpoint.
// Different events populate different fields.
type HookPayload struct {
	ToolInput    any            `json:"tool_input,omitempty"`
	ToolResponse any            `json:"tool_response,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	SubagentID   string         `json:"subagent_id,omitempty"`
	Channel      string         `json:"channel,omitempty"`
	State        string         `json:"state,omitempty"`
	// Prompt is the text the user sent on a UserPromptSubmit event. It is the
	// source of the agent's task line: what an agent was asked to do is a
	// better answer to "what is this agent working on" than a summary it has to
	// remember to publish itself. Providers that forward their hook payload
	// (claude, cursor) carry it through unchanged; the transcript tailer fills
	// it from the parsed user turn.
	Prompt       string    `json:"prompt,omitempty"`
	Task         string    `json:"task,omitempty"`
	TaskID       string    `json:"task_id,omitempty"`
	TaskTitle    string    `json:"task_title,omitempty"`
	ToolName     string    `json:"tool_name,omitempty"`
	Command      string    `json:"command,omitempty"`
	Error        string    `json:"error,omitempty"`
	Model        string    `json:"model,omitempty"`
	Event        HookEvent `json:"event"`
	Sender       string    `json:"sender,omitempty"`
	SubagentType string    `json:"subagent_type,omitempty"`
	Message      string    `json:"message,omitempty"`
	File         string    `json:"file,omitempty"`
	Mentions     []string  `json:"mentions,omitempty"`
	CostUSD      float64   `json:"cost_usd,omitempty"`
	InputTokens  int64     `json:"input_tokens,omitempty"`
	OutputTokens int64     `json:"output_tokens,omitempty"`
}

// The Claude Code hook-settings writer that generates .claude/settings.json
// (the push side of this contract) lives in pkg/provider (claude_hooks.go)
// as part of the claude provider's ActivitySource implementation.
