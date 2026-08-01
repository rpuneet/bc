package provider

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// codex transcript capture.
//
// The codex CLI writes one JSONL "rollout" file per session under
// <codexHome>/sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl (codexHome defaults
// to ~/.codex). Unlike pi the path is date-keyed, not cwd-keyed: the working
// directory lives inside the file, in the first `session_meta` record. So codex
// implements TranscriptFileSelector (match the agent's cwd against session_meta
// and pick the newest matching rollout) rather than a cwd-encoded path glob, and
// TranscriptSessionParser (its tool-result lines carry only a call_id, no tool
// name, so a per-file session resolves each result against the earlier call).
//
// Each line is {"timestamp":…,"type":…,"payload":{…}}. The records that map onto
// the daemon hook vocabulary are:
//
//	{"type":"session_meta","payload":{"cwd":"…", …}}                      → SessionStart
//	{"type":"event_msg","payload":{"type":"user_message","message":"…"}}  → UserPromptSubmit
//	{"type":"event_msg","payload":{"type":"task_complete", …}}            → Stop
//	{"type":"response_item","payload":{"type":"function_call","name":"…","arguments":"{…}","call_id":"…"}}       → PreToolUse
//	{"type":"response_item","payload":{"type":"custom_tool_call","name":"…","input":"…","call_id":"…"}}          → PreToolUse
//	{"type":"response_item","payload":{"type":"function_call_output","call_id":"…","output":"Exit code: N\n…"}}  → PostToolUse / …Failure
//	{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"…","output":"{\"metadata\":{\"exit_code\":N}}"}} → PostToolUse / …Failure
//
// Assistant text and reasoning have no hook-event equivalent and are skipped, as
// in the pi parser. Verified against real rollout files under ~/.codex/sessions
// on the dev machine (including mycel-spawned codex agents).

// codexSessionsRoot resolves codex's sessions directory. codex honors CODEX_HOME
// (sessions live under <CODEX_HOME>/sessions); otherwise the default under the
// user home is authoritative for mycel-spawned tmux agents. Overridable in tests.
var codexSessionsRoot = func() string {
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		return filepath.Join(dir, "sessions")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "sessions")
}

// ActivityMode reports that codex exposes activity via an on-disk session
// transcript the daemon tails, not via hook commands.
func (p *CodexProvider) ActivityMode() string { return ActivityModeTranscript }

// WriteHookConfig is a no-op: codex has no hook mechanism, activity is sourced
// by tailing its rollout transcript (ActivityModeTranscript).
func (p *CodexProvider) WriteHookConfig(_, _, _ string) error { return nil }

// TranscriptGlobs returns nil: codex's active file is located by SelectTranscript
// (session_meta cwd match), not a cwd-encoded path glob.
func (p *CodexProvider) TranscriptGlobs(_ string) []string { return nil }

// NewTranscriptSession returns a fresh per-file session for the tailer.
func (p *CodexProvider) NewTranscriptSession() TranscriptSession {
	return &codexSession{calls: make(map[string]string)}
}

// SelectTranscript returns the newest rollout file whose session_meta records
// the given cwd, or "" when none matches. Candidate rollouts are ranked
// newest-first by mtime and their session_meta read lazily, so an active agent
// resolves after reading only the first (most recent) file's header.
func (p *CodexProvider) SelectTranscript(cwd string) string {
	root := codexSessionsRoot()
	if root == "" || cwd == "" {
		return ""
	}
	matches, err := filepath.Glob(filepath.Join(root, "*", "*", "*", "rollout-*.jsonl"))
	if err != nil || len(matches) == 0 {
		return ""
	}

	type candidate struct {
		mod  time.Time
		path string
	}
	cands := make([]candidate, 0, len(matches))
	for _, m := range matches {
		fi, statErr := os.Stat(m)
		if statErr != nil {
			continue
		}
		cands = append(cands, candidate{mod: fi.ModTime(), path: m})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mod.After(cands[j].mod) })

	// Bound header reads so an unusually large sessions dir can't stall a tick;
	// the active agent's file is near the top by mtime.
	const maxProbe = 256
	for i := range cands {
		if i >= maxProbe {
			break
		}
		if codexSessionCWD(cands[i].path) == cwd {
			return cands[i].path
		}
	}
	return ""
}

// codexSessionCWD reads a rollout file's first line and returns its session_meta
// cwd, or "" when the file is unreadable or its first line is not a session_meta
// record. The first line embeds base_instructions and can be several KB, so the
// read is bounded to 1 MiB.
func codexSessionCWD(path string) string {
	f, err := os.Open(path) //nolint:gosec // path comes from a glob of the user's own codex sessions dir
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // read-only handle

	line, err := bufio.NewReaderSize(io.LimitReader(f, 1<<20), 64*1024).ReadBytes('\n')
	if len(line) == 0 && err != nil {
		return ""
	}
	var top codexLine
	if json.Unmarshal(line, &top) != nil || top.Type != "session_meta" {
		return ""
	}
	var pl codexPayload
	if json.Unmarshal(top.Payload, &pl) != nil {
		return ""
	}
	return pl.CWD
}

// codexLine is the envelope of one rollout JSONL entry.
type codexLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// codexPayload is the union of the payload fields across the record types codex
// emits; each record populates only the subset relevant to it.
type codexPayload struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	CallID    string `json:"call_id"`
	Arguments string `json:"arguments"`
	Input     string `json:"input"`
	Output    string `json:"output"`
	Message   string `json:"message"`
	CWD       string `json:"cwd"`
}

// codexSession holds the per-file state for parsing one rollout: the map from a
// tool call's id to its tool name (codex result lines carry only the id), and
// whether SessionStart has been emitted so a forked session's second
// session_meta record does not double-fire it.
type codexSession struct {
	calls        map[string]string
	startEmitted bool
}

// ParseLine turns one codex rollout line into zero or more activity events.
// Unrecognized or malformed lines yield (nil, nil).
func (s *codexSession) ParseLine(line []byte) ([]TranscriptActivity, error) {
	line = trimSpaceBytes(line)
	if len(line) == 0 {
		return nil, nil
	}
	var top codexLine
	if err := json.Unmarshal(line, &top); err != nil {
		return nil, nil //nolint:nilerr // tolerate malformed lines mid-stream
	}
	ts, _ := time.Parse(time.RFC3339Nano, top.Timestamp) //nolint:errcheck // zero time is an acceptable fallback

	var pl codexPayload
	if len(top.Payload) > 0 {
		_ = json.Unmarshal(top.Payload, &pl) //nolint:errcheck // tolerate; unknown payloads yield no events
	}

	switch top.Type {
	case "session_meta":
		if s.startEmitted {
			return nil, nil
		}
		s.startEmitted = true
		return []TranscriptActivity{{Event: "SessionStart", Timestamp: ts}}, nil
	case "event_msg":
		return s.eventMsg(&pl, ts), nil
	case "response_item":
		return s.responseItem(&pl, ts), nil
	default:
		return nil, nil
	}
}

// eventMsg maps codex UI events onto activity. Only genuine user turns and
// turn-completion map to hook events; reasoning, agent messages, token counts
// and the like are informational and skipped.
func (s *codexSession) eventMsg(pl *codexPayload, ts time.Time) []TranscriptActivity {
	switch pl.Type {
	case "user_message":
		if pl.Message == "" {
			return nil
		}
		return []TranscriptActivity{{Event: "UserPromptSubmit", Prompt: pl.Message, Timestamp: ts}}
	case "task_complete":
		return []TranscriptActivity{{Event: "Stop", Timestamp: ts}}
	default:
		return nil
	}
}

// responseItem maps codex model items onto activity: tool calls to PreToolUse
// (recording call_id→name), tool outputs to the paired PostToolUse/…Failure.
func (s *codexSession) responseItem(pl *codexPayload, ts time.Time) []TranscriptActivity {
	switch pl.Type {
	case "function_call":
		if pl.Name == "" {
			return nil
		}
		if pl.CallID != "" {
			s.calls[pl.CallID] = pl.Name
		}
		return []TranscriptActivity{{Event: "PreToolUse", ToolName: pl.Name, ToolInput: decodeCodexArgs(pl.Arguments), Timestamp: ts}}
	case "custom_tool_call":
		if pl.Name == "" {
			return nil
		}
		if pl.CallID != "" {
			s.calls[pl.CallID] = pl.Name
		}
		return []TranscriptActivity{{Event: "PreToolUse", ToolName: pl.Name, ToolInput: pl.Input, Timestamp: ts}}
	case "function_call_output":
		name := s.takeCall(pl.CallID)
		if name == "" {
			return nil
		}
		code, ok := codexShellExitCode(pl.Output)
		return codexToolResult(name, pl.Output, ok && code != 0, ts)
	case "custom_tool_call_output":
		name := s.takeCall(pl.CallID)
		if name == "" {
			return nil
		}
		out, failed := codexCustomOutput(pl.Output)
		return codexToolResult(name, out, failed, ts)
	default:
		return nil
	}
}

// takeCall resolves and consumes the tool name recorded for a call id. Consuming
// keeps the map bounded (each call has exactly one output). Returns "" when the
// originating call was not seen — e.g. it predates the tailer's seed at EOF — so
// the orphan output is skipped rather than emitted as an unpaired node.
func (s *codexSession) takeCall(callID string) string {
	if callID == "" {
		return ""
	}
	name := s.calls[callID]
	if name != "" {
		delete(s.calls, callID)
	}
	return name
}

// codexToolResult builds the PostToolUse (or PostToolUseFailure) activity for a
// completed tool call.
func codexToolResult(name, resp string, failed bool, ts time.Time) []TranscriptActivity {
	if failed {
		return []TranscriptActivity{{
			Event:        "PostToolUseFailure",
			ToolName:     name,
			ToolResponse: resp,
			Error:        resp,
			Timestamp:    ts,
		}}
	}
	return []TranscriptActivity{{
		Event:        "PostToolUse",
		ToolName:     name,
		ToolResponse: resp,
		Timestamp:    ts,
	}}
}

// decodeCodexArgs decodes a function_call's arguments (a JSON-encoded string)
// into a structured value for the tool_input field, falling back to the raw
// string when it is not valid JSON.
func decodeCodexArgs(s string) any {
	if s == "" {
		return nil
	}
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		return v
	}
	return s
}

// codexShellExitCode parses the leading "Exit code: N" of a shell tool output.
// Returns the code and true when present, (0,false) when the output does not
// carry one (treated as success by the caller).
func codexShellExitCode(output string) (int, bool) {
	const prefix = "Exit code: "
	if !strings.HasPrefix(output, prefix) {
		return 0, false
	}
	rest := output[len(prefix):]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	code, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil {
		return 0, false
	}
	return code, true
}

// codexCustomOutput decodes a custom-tool output (a JSON string with an inner
// "output" and "metadata.exit_code"), returning the inner output text and
// whether the tool failed. Falls back to the raw output on a decode failure.
func codexCustomOutput(output string) (string, bool) {
	if output == "" {
		return "", false
	}
	var wrapped struct {
		Output   string `json:"output"`
		Metadata struct {
			ExitCode int `json:"exit_code"`
		} `json:"metadata"`
	}
	if json.Unmarshal([]byte(output), &wrapped) != nil {
		return output, false
	}
	text := wrapped.Output
	if text == "" {
		text = output
	}
	return text, wrapped.Metadata.ExitCode != 0
}

// Ensure CodexProvider satisfies the activity/transcript capabilities.
var (
	_ ActivitySource          = (*CodexProvider)(nil)
	_ TranscriptSessionParser = (*CodexProvider)(nil)
	_ TranscriptFileSelector  = (*CodexProvider)(nil)
)
