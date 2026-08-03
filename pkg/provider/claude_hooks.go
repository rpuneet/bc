package provider

// Claude Code hook-settings writer, moved from pkg/agent so the claude
// provider can own its ActivitySource implementation without pkg/provider
// importing pkg/agent (pkg/agent already imports pkg/provider). The event
// and state names embedded in the generated commands are the wire
// vocabulary of the daemon's /api/agents/{name}/hook endpoint (see
// pkg/agent/hooks.go for the ingestion side).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

// WriteClaudeHookSettings writes .claude/settings.json with HTTP-based hooks
// that POST to the daemon's /api/agents/{name}/hook endpoint for instant status updates.
//
// Uses curl to POST JSON payloads. Tool-aware hooks read stdin JSON via jq.
// This is idempotent: if settings.json already exists the hooks section is merged.
func WriteClaudeHookSettings(repoRoot string) error {
	// repoRoot is derived from validated agent config, but
	// reject traversal segments so hook settings can never be written
	// outside the intended directory.
	repoRoot = filepath.Clean(repoRoot)
	if strings.Contains(repoRoot, "..") {
		return fmt.Errorf("unsafe worktree root %q", repoRoot)
	}
	claudeDir := filepath.Join(repoRoot, ".claude")
	if err := os.MkdirAll(claudeDir, 0750); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}

	// Hook commands use $MYCEL_DAEMON_ADDR env var (set per-agent based on runtime).
	// Falls back to localhost for backward compat.
	daemonAddr := "${MYCEL_DAEMON_ADDR:-http://127.0.0.1:9374}"

	// hookCmd reads the full raw JSON from Claude Code's stdin, merges in
	// our event/state fields, and POSTs the complete payload to the daemon.
	// This preserves all fields Claude sends (tool_name, tool_input, session_id, etc.)
	//
	// Only event and state are added. These commands used to also send a
	// per-event task ("Processing prompt...", "Turn complete"), which the daemon
	// stored and the Live feed rendered in the same "task" field that now holds
	// the agent's real task line — a label describing the hook that fired,
	// sitting where the thing the agent was asked to do belongs. The task line is
	// derived from the prompt on a turn start; the lifecycle meaning these
	// strings carried is already in state.
	hookCmd := func(event, stateTarget string) string {
		return fmt.Sprintf(
			`bash -c 'RAW=$(cat); PAYLOAD=$(echo "$RAW" | jq -c ". + {event:\"%s\",state:\"%s\"}" 2>/dev/null || echo "{\"event\":\"%s\",\"state\":\"%s\"}"); curl -sX POST %s/api/agents/${MYCEL_AGENT_ID}/hook -H "Content-Type: application/json" -d "$PAYLOAD" 2>/dev/null || true'`,
			event, stateTarget, event, stateTarget, daemonAddr,
		)
	}

	settings := claudeSettings{
		Hooks: map[string][]claudeHookMatcher{
			"SessionStart":       {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("SessionStart", "idle")}}}},
			"SessionEnd":         {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("SessionEnd", "stopped")}}}},
			"UserPromptSubmit":   {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("UserPromptSubmit", "working")}}}},
			"PreToolUse":         {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("PreToolUse", "")}}}},
			"PostToolUse":        {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("PostToolUse", "")}}}},
			"PostToolUseFailure": {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("PostToolUseFailure", "")}}}},
			"PermissionRequest":  {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("PermissionRequest", "stuck")}}}},
			"Stop":               {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("Stop", "idle")}}}},
			"Notification":       {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("Notification", "")}}}},
			"SubagentStart":      {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("SubagentStart", "")}}}},
			"SubagentStop":       {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("SubagentStop", "")}}}},
			"TaskCompleted":      {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("TaskCompleted", "done")}}}},
			"TeammateIdle":       {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("TeammateIdle", "")}}}},
			"InstructionsLoaded": {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("InstructionsLoaded", "")}}}},
			"ConfigChange":       {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("ConfigChange", "")}}}},
			"WorktreeCreate":     {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("WorktreeCreate", "")}}}},
			"WorktreeRemove":     {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("WorktreeRemove", "")}}}},
			"PreCompact":         {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("PreCompact", "")}}}},
			"PostCompact":        {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("PostCompact", "")}}}},
			"Elicitation":        {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("Elicitation", "stuck")}}}},
			"ElicitationResult":  {{Hooks: []claudeHook{{Type: "command", Command: hookCmd("ElicitationResult", "working")}}}},
		},
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Merge if the file already exists — round-tripping every top-level
	// key (permissions, env, model, …) untouched so only the hooks
	// section is rewritten. Modeling just `hooks` in a struct would drop
	// everything else on re-marshal.
	if raw, err := os.ReadFile(settingsPath); err == nil { //nolint:gosec // worktree-relative
		var full map[string]json.RawMessage
		if err := json.Unmarshal(raw, &full); err == nil {
			existing := &claudeSettings{}
			if h, ok := full["hooks"]; ok {
				// Best-effort: an unparseable hooks section is rebuilt.
				_ = json.Unmarshal(h, &existing.Hooks) //nolint:errcheck
			}
			mergeHooks(existing, settings.Hooks)
			hooksJSON, marshalErr := json.Marshal(existing.Hooks)
			if marshalErr != nil {
				return fmt.Errorf("marshal hook settings: %w", marshalErr)
			}
			full["hooks"] = hooksJSON
			data, marshalErr := json.MarshalIndent(full, "", "  ")
			if marshalErr != nil {
				return fmt.Errorf("marshal hook settings: %w", marshalErr)
			}
			return os.WriteFile(settingsPath, data, 0600)
		}
		// Unparseable file: fall through and rewrite fresh (previous
		// behavior — a corrupt settings.json would break Claude anyway).
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal hook settings: %w", err)
	}
	return os.WriteFile(settingsPath, data, 0600)
}

// invalidHookKeys are Claude Code hook event names that mycel has generated in the
// past but are not actually valid. They must be actively removed from existing
// settings files to prevent Claude from rejecting the entire settings file.
var invalidHookKeys = []string{"StopFailure"}

// isMycelManagedHook returns true if the hook matchers were generated by mycel
// (contain the daemon hook endpoint URL).
func isMycelManagedHook(matchers []claudeHookMatcher) bool {
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
	// Remove known-invalid keys that may exist from prior mycel versions.
	for _, bad := range invalidHookKeys {
		delete(dst.Hooks, bad)
	}
	// Overwrite mycel-managed hooks so URL/env changes propagate.
	// Preserve user-customized hooks (those not generated by mycel).
	for event, matchers := range src {
		existing, exists := dst.Hooks[event]
		if !exists || isMycelManagedHook(existing) {
			dst.Hooks[event] = matchers
		}
	}
}
