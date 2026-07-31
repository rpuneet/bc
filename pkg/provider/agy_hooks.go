package provider

// Antigravity CLI (`agy`) lifecycle-hook writer. This is the agy analog of
// claude_hooks.go: it writes .agents/hooks.json so an agy agent reports its
// lifecycle to the daemon's /api/agents/{name}/hook endpoint. The event and state
// names embedded in the generated commands are the wire vocabulary of that
// endpoint (see pkg/agent/hooks.go for the ingestion side).
//
// agy's hooks.json schema (https://antigravity.google/docs/hooks):
//   - Top-level keys are hook *names*; each maps to an event configuration.
//   - Tool events (PreToolUse, PostToolUse) are "grouped": a matcher regex
//     plus a list of handlers.
//   - Lifecycle events (PreInvocation, PostInvocation, Stop) are "flat": a
//     list of handlers directly.
//   - Each handler runs via `sh -c` with the working directory set to the
//     directory containing hooks.json. It receives a JSON payload on stdin
//     and must print a JSON object on stdout (PreToolUse expects a decision;
//     the others expect `{}`).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// agyHookHandler is a single command handler in agy's hooks.json.
type agyHookHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// agyHookGroup wraps handlers for a tool event (PreToolUse/PostToolUse) with a
// matcher regex.
type agyHookGroup struct {
	Matcher string           `json:"matcher"`
	Hooks   []agyHookHandler `json:"hooks"`
}

// agyHookSpec is one named hook entry. Tool events use the grouped form; the
// lifecycle events use the flat handler-list form.
type agyHookSpec struct {
	PreToolUse     []agyHookGroup   `json:"PreToolUse,omitempty"`
	PostToolUse    []agyHookGroup   `json:"PostToolUse,omitempty"`
	PreInvocation  []agyHookHandler `json:"PreInvocation,omitempty"`
	PostInvocation []agyHookHandler `json:"PostInvocation,omitempty"`
	Stop           []agyHookHandler `json:"Stop,omitempty"`
}

// agyHookTimeoutSecs bounds each hook command so a slow curl never stalls the
// agent's execution loop.
const agyHookTimeoutSecs = 5

// agyHookCommand builds the shell command an agy hook runs. It drains stdin
// (agy pipes the payload in), POSTs a mycel hook event to the daemon, and prints
// the JSON result agy expects on stdout. daemonAddr and agentID are resolved
// at runtime from the MYCEL_DAEMON_ADDR / MYCEL_AGENT_ID environment variables
// set on the agent session, matching the claude provider.
func agyHookCommand(event, state, task, stdout string) string {
	const daemonAddr = "${MYCEL_DAEMON_ADDR:-http://127.0.0.1:9374}"
	payload := fmt.Sprintf(`{"event":"%s","state":"%s","task":"%s"}`, event, state, task)
	return fmt.Sprintf(
		`cat >/dev/null 2>&1; curl -sX POST "%s/api/agents/${MYCEL_AGENT_ID}/hook" -H "Content-Type: application/json" -d '%s' >/dev/null 2>&1; printf '%%s' '%s'`,
		daemonAddr, payload, stdout,
	)
}

func agyHandler(event, state, task, stdout string) agyHookHandler {
	return agyHookHandler{Type: "command", Command: agyHookCommand(event, state, task, stdout), Timeout: agyHookTimeoutSecs}
}

// WriteAgyHookSettings writes .agents/hooks.json into the agent worktree so the
// agy CLI reports lifecycle transitions to the daemon. It is idempotent: the mycel hook
// entry is overwritten while any other user-defined hooks are preserved.
func WriteAgyHookSettings(worktreeRoot string) error {
	// worktreeRoot is derived from validated agent config, but reject
	// traversal segments so hook settings can never be written outside the
	// intended directory.
	worktreeRoot = filepath.Clean(worktreeRoot)
	if strings.Contains(worktreeRoot, "..") {
		return fmt.Errorf("unsafe worktree root %q", worktreeRoot)
	}
	agentsDir := filepath.Join(worktreeRoot, ".agents")
	if err := os.MkdirAll(agentsDir, 0750); err != nil {
		return fmt.Errorf("create .agents dir: %w", err)
	}

	// PreToolUse must return a decision; "allow" keeps agy autonomous
	// (redundant with --dangerously-skip-permissions, but the contract asks
	// for a decision). The remaining events expect an empty object.
	const allow = `{"decision":"allow"}`
	const empty = `{}`

	bcHook := agyHookSpec{
		PreInvocation:  []agyHookHandler{agyHandler("PreInvocation", "working", "Thinking...", empty)},
		PostInvocation: []agyHookHandler{agyHandler("PostInvocation", "", "Response received", empty)},
		Stop:           []agyHookHandler{agyHandler("Stop", "idle", "Turn complete", empty)},
		PreToolUse:     []agyHookGroup{{Matcher: "*", Hooks: []agyHookHandler{agyHandler("PreToolUse", "", "Running tool", allow)}}},
		PostToolUse:    []agyHookGroup{{Matcher: "*", Hooks: []agyHookHandler{agyHandler("PostToolUse", "", "Tool completed", empty)}}},
	}

	const hookName = "mycel-activity"
	hooksPath := filepath.Join(agentsDir, "hooks.json")

	// Merge into an existing hooks.json if present, replacing only the
	// mycel-managed entry and round-tripping every other named hook untouched.
	hooks := map[string]json.RawMessage{}
	if raw, err := os.ReadFile(hooksPath); err == nil { //nolint:gosec // worktree-relative
		if err := json.Unmarshal(raw, &hooks); err != nil {
			// Unparseable file: rewrite fresh rather than fail the agent.
			hooks = map[string]json.RawMessage{}
		}
	}
	hookJSON, err := json.Marshal(bcHook)
	if err != nil {
		return fmt.Errorf("marshal agy hook: %w", err)
	}
	hooks[hookName] = hookJSON

	data, err := json.MarshalIndent(hooks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agy hooks: %w", err)
	}
	return os.WriteFile(hooksPath, append(data, '\n'), 0600)
}
