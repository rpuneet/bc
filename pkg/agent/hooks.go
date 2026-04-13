package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HookEvent is a lifecycle event type — either a Claude Code hook or a bc-internal event.
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

// ── bc-internal events (POSTed by bcd Go code, not Claude Code hooks) ──

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
	ToolInput    any       `json:"tool_input,omitempty"`
	SubagentID   string    `json:"subagent_id,omitempty"`
	Channel      string    `json:"channel,omitempty"`
	State        string    `json:"state,omitempty"`
	Task         string    `json:"task,omitempty"`
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

// ── Settings.json writer (generates HTTP-based hooks) ──

type claudeSettings struct {
	Hooks map[string][]claudeHookMatcher `json:"hooks,omitempty"`
}

type claudeHookMatcher struct {
	Matcher string       `json:"matcher,omitempty"`
	Hooks   []claudeHook `json:"hooks"`
}

type claudeHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// WriteWorkspaceHookSettings writes .claude/settings.json with HTTP-based hooks
// that POST to bcd's /api/agents/{name}/hook endpoint for instant status updates.
//
// Uses curl to POST JSON payloads. Tool-aware hooks read stdin JSON via jq.
// This is idempotent: if settings.json already exists the hooks section is merged.
func WriteWorkspaceHookSettings(workspaceRoot string) error {
	claudeDir := filepath.Join(workspaceRoot, ".claude")
	if err := os.MkdirAll(claudeDir, 0750); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}

	// Hook commands use $BC_BCD_ADDR env var (set per-agent based on runtime).
	// Falls back to localhost for backward compat.
	bcdAddr := "${BC_BCD_ADDR:-http://127.0.0.1:9374}"

	// hookCmd reads the full raw JSON from Claude Code's stdin, merges in
	// our event/state fields, and POSTs the complete payload to bcd.
	// This preserves all fields Claude sends (tool_name, tool_input, session_id, etc.)
	hookCmd := func(event HookEvent, stateTarget State, taskDesc string) string {
		return fmt.Sprintf(
			`bash -c 'RAW=$(cat); PAYLOAD=$(echo "$RAW" | jq -c ". + {event:\"%s\",state:\"%s\",task:\"%s\"}" 2>/dev/null || echo "{\"event\":\"%s\",\"state\":\"%s\",\"task\":\"%s\"}"); curl -sX POST %s/api/agents/${BC_AGENT_ID}/hook -H "Content-Type: application/json" -d "$PAYLOAD" 2>/dev/null || true'`,
			event, stateTarget, taskDesc, event, stateTarget, taskDesc, bcdAddr,
		)
	}

	settings := claudeSettings{
		Hooks: map[string][]claudeHookMatcher{
			"SessionStart":       {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookSessionStart, StateIdle, "Session started")}}}},
			"SessionEnd":         {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookSessionEnd, StateStopped, "Session ended")}}}},
			"UserPromptSubmit":   {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookUserPromptSubmit, StateWorking, "Processing prompt...")}}}},
			"PreToolUse":         {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookPreToolUse, "", "Running tool")}}}},
			"PostToolUse":        {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookPostToolUse, "", "Tool completed")}}}},
			"PostToolUseFailure": {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookPostToolUseFailure, "", "Tool failed")}}}},
			"PermissionRequest":  {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookPermissionRequest, StateStuck, "Waiting for permission")}}}},
			"Stop":               {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookStop, StateIdle, "Turn complete")}}}},
			"Notification":       {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("Notification", "", "")}}}},
			"SubagentStart":      {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookSubagentStart, "", "Subagent started")}}}},
			"SubagentStop":       {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookSubagentStop, "", "Subagent completed")}}}},
			"TaskCompleted":      {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookTaskCompleted, StateDone, "Task completed")}}}},
			"TeammateIdle":       {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("TeammateIdle", "", "")}}}},
			"InstructionsLoaded": {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("InstructionsLoaded", "", "")}}}},
			"ConfigChange":       {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("ConfigChange", "", "")}}}},
			"WorktreeCreate":     {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookWorktreeCreate, "", "Creating worktree")}}}},
			"WorktreeRemove":     {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("WorktreeRemove", "", "")}}}},
			"PreCompact":         {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookPreCompact, "", "Compacting context...")}}}},
			"PostCompact":        {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookPostCompact, "", "Context compacted")}}}},
			"Elicitation":        {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookElicitation, StateStuck, "MCP input needed")}}}},
			"ElicitationResult":  {{Hooks: []claudeHook{{Type: "command", Command: hookCmd(HookElicitationResult, StateWorking, "MCP input received")}}}},
		},
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Merge if file already exists so we don't clobber user customizations.
	if existing, err := loadClaudeSettings(settingsPath); err == nil {
		mergeHooks(existing, settings.Hooks)
		data, marshalErr := json.MarshalIndent(existing, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("marshal hook settings: %w", marshalErr)
		}
		return os.WriteFile(settingsPath, data, 0600)
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal hook settings: %w", err)
	}
	return os.WriteFile(settingsPath, data, 0600)
}

func loadClaudeSettings(path string) (*claudeSettings, error) {
	data, err := os.ReadFile(path) //nolint:gosec // workspace-relative
	if err != nil {
		return nil, err
	}
	var s claudeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// invalidHookKeys are Claude Code hook event names that bc has generated in the
// past but are not actually valid. They must be actively removed from existing
// settings files to prevent Claude from rejecting the entire settings file.
var invalidHookKeys = []string{"StopFailure"}

// isBcManagedHook returns true if the hook matchers were generated by bc
// (contain the bcd hook endpoint URL).
func isBcManagedHook(matchers []claudeHookMatcher) bool {
	for _, m := range matchers {
		for _, h := range m.Hooks {
			if strings.Contains(h.Command, "/api/agents/") {
				return true
			}
		}
	}
	return false
}

func mergeHooks(dst *claudeSettings, src map[string][]claudeHookMatcher) {
	if dst.Hooks == nil {
		dst.Hooks = make(map[string][]claudeHookMatcher)
	}
	// Remove known-invalid keys that may exist from prior bc versions.
	for _, bad := range invalidHookKeys {
		delete(dst.Hooks, bad)
	}
	// Overwrite bc-managed hooks so URL/env changes propagate.
	// Preserve user-customized hooks (those not generated by bc).
	for event, matchers := range src {
		existing, exists := dst.Hooks[event]
		if !exists || isBcManagedHook(existing) {
			dst.Hooks[event] = matchers
		}
	}
}
