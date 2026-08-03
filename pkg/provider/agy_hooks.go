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
	"strconv"
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

// agyHookCommand builds the shell command an agy hook runs. It merges mycel's
// event/state fields into the payload agy pipes in on stdin, POSTs the result to
// the daemon, and prints the JSON result agy expects on stdout. The agent ID is
// resolved at runtime from MYCEL_AGENT_ID and the daemon address through
// DaemonAddrShell, matching the claude provider.
//
// It previously discarded stdin (`cat >/dev/null`) and POSTed only the three
// fields known at generation time. Everything agy reports about a turn — the
// prompt, the tool being called, its input and its result — was thrown away, so
// agy agents got a Live feed of bare event names and, once the task line began
// to be derived from the prompt, no task at all. Forwarding the payload is what
// gives agy the same detail claude and cursor have.
//
// If jq is missing or the payload is unparseable, the bare event is still POSTed
// so state remains correct; only the detail is lost. The stdout contract is
// honored unconditionally — a reporting failure must never stall a turn.
//
// Only event and state are added. These commands used to also send a per-event
// task ("Thinking...", "Turn complete"), which the daemon stored and the Live
// feed rendered in the same "task" field that now holds the agent's real task
// line — a label naming the hook that fired, sitting where the thing the agent
// was asked to do belongs. The task line is derived from the prompt on a turn
// start; the lifecycle meaning those strings carried is already in state.
func agyHookCommand(event, state, stdout string) string {
	const daemonAddr = DaemonAddrShell
	fallback := fmt.Sprintf(`{\"event\":\"%s\",\"state\":\"%s\"}`, event, state)
	return fmt.Sprintf(
		`bash -c 'RAW=$(cat); PAYLOAD=$(echo "$RAW" | jq -c ". + {event:\"%s\",state:\"%s\"}" 2>/dev/null || echo "%s"); `+
			`curl -sX POST "%s/api/agents/${MYCEL_AGENT_ID}/hook" -H "Content-Type: application/json" -d "$PAYLOAD" >/dev/null 2>&1; `+
			`printf "%%s" %s'`,
		event, state, fallback, daemonAddr, strconv.Quote(stdout),
	)
}

func agyHandler(event, state, stdout string) agyHookHandler {
	return agyHookHandler{Type: "command", Command: agyHookCommand(event, state, stdout), Timeout: agyHookTimeoutSecs}
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
		PreInvocation:  []agyHookHandler{agyHandler("PreInvocation", "working", empty)},
		PostInvocation: []agyHookHandler{agyHandler("PostInvocation", "", empty)},
		Stop:           []agyHookHandler{agyHandler("Stop", "idle", empty)},
		PreToolUse:     []agyHookGroup{{Matcher: "*", Hooks: []agyHookHandler{agyHandler("PreToolUse", "", allow)}}},
		PostToolUse:    []agyHookGroup{{Matcher: "*", Hooks: []agyHookHandler{agyHandler("PostToolUse", "", empty)}}},
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
