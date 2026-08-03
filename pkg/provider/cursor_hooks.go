package provider

// Cursor Agent lifecycle-hook writer. This is the cursor analog of
// claude_hooks.go and agy_hooks.go: it writes .cursor/hooks.json plus a small
// reporter script into the agent worktree so cursor-agent reports its lifecycle
// to the daemon's /api/agents/{name}/hook endpoint. The event and state names
// passed to that script are the wire vocabulary of the endpoint (see
// pkg/agent/hooks.go for the ingestion side).
//
// cursor's hooks.json schema (https://cursor.com/docs/agent/hooks):
//
//	{"version":1,"hooks":{"<event>":[{"command":"…","timeout":5}]}}
//
// Every entry registered for an event runs; each receives the event payload as
// JSON on stdin and may print a JSON object on stdout. Cursor fails open when a
// hook exits non-zero unless the entry sets failClosed, which mycel never does:
// activity reporting must never be able to block an agent.
//
// Unlike the claude and agy writers, which inline the whole shell pipeline into
// the config, cursor's hooks point at a script mycel writes next to the config.
// That is not a style preference. cursor-agent silently declines to run a
// sufficiently long inlined command — verified against 2026.07.23, where a
// ~530-character `bash -c '…'` entry never executed while the same command run
// by hand POSTed correctly, and short inline commands ran fine. A script path is
// also the form cursor's own documentation uses, and it keeps the reporter
// readable and independently testable.
//
// Cursor names its events in camelCase and uses its own field names, so the
// reporter translates on the way out:
//
//	sessionStart       → SessionStart        (idle)
//	beforeSubmitPrompt → UserPromptSubmit    (working)   carries .prompt
//	preToolUse         → PreToolUse          (no change) carries .tool_name/.tool_input
//	postToolUse        → PostToolUse         (no change) .tool_output   → tool_response
//	postToolUseFailure → PostToolUseFailure  (no change) .error_message → error
//	subagentStart      → SubagentStart       (no change)
//	subagentStop       → SubagentStop        (no change)
//	preCompact         → PreCompact          (no change)
//	stop               → Stop                (idle)
//	sessionEnd         → SessionEnd          (stopped)
//
// Deliberately unregistered: afterAgentThought fires several times per model
// turn and has no daemon equivalent, so forwarding it would flood the event log
// with events the UI cannot render. beforeShellExecution/afterShellExecution,
// beforeReadFile and afterFileEdit are narrower restatements of
// preToolUse/postToolUse and would double-report every tool call.
//
// This mapping was verified against cursor-agent 2026.07.23 by registering every
// documented event and observing which fire, in both one-shot (-p) and
// interactive (tmux) runs. Interactive is the mode mycel uses, and
// beforeSubmitPrompt and stop fire only there — which is what makes an agent's
// turn boundaries (working → idle) visible for a mycel-managed session.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cursorHooksSchemaVersion is the schema version cursor expects in hooks.json.
const cursorHooksSchemaVersion = 1

// cursorHookTimeoutSecs bounds each hook command so a slow or unreachable
// daemon never stalls the agent's turn.
const cursorHookTimeoutSecs = 5

// cursorReporterRelPath is where the reporter script lives, relative to the
// worktree root. Cursor runs project hooks from the project root, so this
// doubles as the command written into hooks.json.
const cursorReporterRelPath = ".cursor/hooks/mycel-activity.sh"

// cursorNoState is the sentinel passed to the reporter for events that must not
// move the agent's state. A word is used rather than an empty argument so every
// command in hooks.json is whitespace-free and needs no shell quoting.
const cursorNoState = "none"

// cursorReporterScript is the hook reporter mycel writes into the worktree. It
// takes the daemon event name and target state as arguments, reads cursor's
// payload on stdin, and POSTs a merged payload to the daemon.
//
// jq is used with --arg so no value is ever spliced into a program string, and
// the two field renames are applied only when cursor actually sent the field.
// Without jq the reporter still POSTs the bare event and state, so an agent's
// state stays correct on a machine that lacks it; only the tool detail is lost.
//
// It always exits 0 and always prints {}: a reporter that failed to reach the
// daemon must never turn into a stalled or blocked agent.
//
// Only event and state are added. The reporter used to also derive a per-event
// task ("Processing prompt...", "Turn complete"), which the daemon stored and
// the Live feed rendered in the same "task" field that now holds the agent's
// real task line — a label naming the hook that fired, sitting where the thing
// the agent was asked to do belongs. The task line is derived from the prompt on
// a turn start; the lifecycle meaning those strings carried is already in state.
const cursorReporterScript = `#!/usr/bin/env bash
# Managed by mycel. Regenerated whenever an agent starts — edit the generator
# (pkg/provider/cursor_hooks.go), not this file.
#
# Reports one cursor-agent lifecycle event to the mycel daemon.
# Usage: mycel-activity.sh <DaemonEvent> <state|none>

event="$1"
state="$2"
[ "$state" = "` + cursorNoState + `" ] && state=""

raw=$(cat)
addr="` + DaemonAddrShell + `"

payload=$(printf '%s' "$raw" | jq -c \
  --arg event "$event" --arg state "$state" \
  '. + {event: $event, state: $state}
     + (if .tool_output   then {tool_response: .tool_output}  else {} end)
     + (if .error_message then {error: .error_message}        else {} end)' 2>/dev/null)

if [ -z "$payload" ]; then
  payload=$(printf '{"event":"%s","state":"%s"}' "$event" "$state")
fi

curl -sS -m 3 -X POST "$addr/api/agents/${MYCEL_AGENT_ID}/hook" \
  -H 'Content-Type: application/json' \
  -d "$payload" >/dev/null 2>&1 || true

printf '{}'
exit 0
`

// cursorHook is a single command entry under a cursor hook event.
type cursorHook struct {
	Command string `json:"command"`
	Type    string `json:"type,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// cursorHookEntry builds the hooks.json entry that invokes the reporter for one
// daemon event. state is cursorNoState for informational events.
func cursorHookEntry(event, state string) cursorHook {
	return cursorHook{
		Type:    "command",
		Command: fmt.Sprintf("%s %s %s", cursorReporterRelPath, event, state),
		Timeout: cursorHookTimeoutSecs,
	}
}

// cursorActivityHooks is the mycel-managed hook set, keyed by cursor's event
// name and mapped to the daemon event plus the state that event implies.
func cursorActivityHooks() map[string][]cursorHook {
	return map[string][]cursorHook{
		"sessionStart":       {cursorHookEntry("SessionStart", "idle")},
		"beforeSubmitPrompt": {cursorHookEntry("UserPromptSubmit", "working")},
		"preToolUse":         {cursorHookEntry("PreToolUse", cursorNoState)},
		"postToolUse":        {cursorHookEntry("PostToolUse", cursorNoState)},
		"postToolUseFailure": {cursorHookEntry("PostToolUseFailure", cursorNoState)},
		"subagentStart":      {cursorHookEntry("SubagentStart", cursorNoState)},
		"subagentStop":       {cursorHookEntry("SubagentStop", cursorNoState)},
		"preCompact":         {cursorHookEntry("PreCompact", cursorNoState)},
		"stop":               {cursorHookEntry("Stop", "idle")},
		"sessionEnd":         {cursorHookEntry("SessionEnd", "stopped")},
	}
}

// ActivityMode reports that cursor emits activity via lifecycle hooks that POST
// to the daemon, not via a tailed transcript.
func (p *CursorProvider) ActivityMode() string { return ActivityModeHooks }

// WriteHookConfig writes .cursor/hooks.json and the reporter script into the
// agent worktree so cursor-agent reports lifecycle events to the daemon.
// daemonAddr and agentID are ignored: the reporter resolves both at runtime from
// MYCEL_DAEMON_ADDR / MYCEL_AGENT_ID, so one config stays correct across daemon
// restarts and port changes.
//
// It is idempotent and preserves hooks mycel did not write: for each event the
// mycel entry is replaced while any user-defined entry is kept, which is safe
// because cursor runs every entry registered for an event.
func (p *CursorProvider) WriteHookConfig(worktreeDir, _, _ string) error {
	return WriteCursorHookSettings(worktreeDir)
}

// TranscriptGlobs returns nil: cursor is a hooks-mode provider and its hook
// payloads already carry a transcript_path, so mycel never has to locate the
// session file by globbing a path-encoded directory name.
func (p *CursorProvider) TranscriptGlobs(_ string) []string { return nil }

// WriteCursorHookSettings writes the mycel reporter script and merges the mycel
// activity hooks into .cursor/hooks.json under worktreeRoot.
func WriteCursorHookSettings(worktreeRoot string) error {
	// worktreeRoot is derived from validated agent config, but reject traversal
	// segments so hook settings can never be written outside the intended
	// directory.
	worktreeRoot = filepath.Clean(worktreeRoot)
	if hasParentSegment(worktreeRoot) {
		return fmt.Errorf("unsafe worktree root %q", worktreeRoot)
	}
	cursorDir := filepath.Join(worktreeRoot, ".cursor")
	if err := os.MkdirAll(filepath.Join(cursorDir, "hooks"), 0750); err != nil {
		return fmt.Errorf("create .cursor/hooks dir: %w", err)
	}

	// 0700 rather than the 0600 used for configs: cursor invokes the reporter as
	// a command, so it has to carry the execute bit. Owner-only keeps it as tight
	// as an executable can be.
	reporterPath := filepath.Join(worktreeRoot, filepath.FromSlash(cursorReporterRelPath))
	if err := os.WriteFile(reporterPath, []byte(cursorReporterScript), 0700); err != nil { //nolint:gosec // must be executable
		return fmt.Errorf("write cursor hook reporter: %w", err)
	}
	// WriteFile applies its mode only when it creates the file, so a reporter
	// left non-executable by an earlier mycel version — or by a user — would stay
	// that way and silently never run. Chmod every time makes regeneration
	// self-healing.
	if err := os.Chmod(reporterPath, 0700); err != nil { //nolint:gosec // must be executable
		return fmt.Errorf("make cursor hook reporter executable: %w", err)
	}

	return writeCursorHooksFile(filepath.Join(cursorDir, "hooks.json"))
}

// hasParentSegment reports whether a cleaned path still walks upward, i.e. has
// a ".." path segment. It compares whole segments rather than substrings so a
// legitimate directory name that merely contains dots — "notes..archive" — is
// not mistaken for traversal. Clean collapses interior "..", so a survivor can
// only be a leading one on a relative path.
func hasParentSegment(path string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(path), "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// writeCursorHooksFile merges the mycel hook set into hooks.json at path.
func writeCursorHooksFile(path string) error {
	// Round-trip every top-level key so a future schema addition (or a user's
	// own key) survives a rewrite; only "hooks" and "version" are touched.
	file := map[string]json.RawMessage{}
	hooks := map[string][]cursorHook{}
	if raw, err := os.ReadFile(path); err == nil { //nolint:gosec // worktree-relative
		if err := json.Unmarshal(raw, &file); err != nil {
			// Unparseable file: rewrite fresh rather than fail the agent. A
			// corrupt hooks.json stops cursor loading any hooks anyway.
			file = map[string]json.RawMessage{}
		} else if h, ok := file["hooks"]; ok {
			// Best-effort: an unparseable hooks section is rebuilt.
			_ = json.Unmarshal(h, &hooks) //nolint:errcheck
		}
	}

	mergeCursorHooks(hooks, cursorActivityHooks())

	hooksJSON, err := json.Marshal(hooks)
	if err != nil {
		return fmt.Errorf("marshal cursor hooks: %w", err)
	}
	file["hooks"] = hooksJSON

	// Set the version only when the file does not already declare a newer one.
	// Stamping our schema version over a higher one would tell cursor to read a
	// newer config with older rules, breaking hooks mycel does not own.
	if !declaresAtLeastCursorSchema(file["version"]) {
		version, marshalErr := json.Marshal(cursorHooksSchemaVersion)
		if marshalErr != nil {
			return fmt.Errorf("marshal cursor hooks version: %w", marshalErr)
		}
		file["version"] = version
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cursor hooks file: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

// declaresAtLeastCursorSchema reports whether raw is a version number at or
// above the schema mycel writes. A missing or unreadable value is false, so the
// caller stamps its own version.
func declaresAtLeastCursorSchema(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var existing int
	if err := json.Unmarshal(raw, &existing); err != nil {
		return false
	}
	return existing >= cursorHooksSchemaVersion
}

// isMycelCursorHook reports whether an entry was generated by mycel, identified
// by the reporter script path in its command.
func isMycelCursorHook(h cursorHook) bool {
	return strings.Contains(h.Command, cursorReporterRelPath)
}

// mergeCursorHooks installs src into dst, dropping previously-generated mycel
// entries (so a changed command shape propagates) and keeping every user entry.
func mergeCursorHooks(dst map[string][]cursorHook, src map[string][]cursorHook) {
	for event, ours := range src {
		kept := make([]cursorHook, 0, len(dst[event])+len(ours))
		for _, h := range dst[event] {
			if !isMycelCursorHook(h) {
				kept = append(kept, h)
			}
		}
		dst[event] = append(kept, ours...)
	}
}

// Ensure CursorProvider satisfies the activity capability.
var _ ActivitySource = (*CursorProvider)(nil)
