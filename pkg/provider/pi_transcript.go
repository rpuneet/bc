package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// pi transcript capture.
//
// pi writes one JSONL session file per (cwd, session) under
// <sessionsRoot>/<encoded-cwd>/<timestamp>_<uuid>.jsonl, where sessionsRoot
// defaults to ~/.pi/agent/sessions and the cwd is encoded by stripping the
// leading "/", replacing every remaining "/" with "-", and wrapping the
// result in "--" on both sides (so "/Users/x/wt" becomes "--Users-x-wt--").
// Each line is a JSON object; the
// interesting ones are:
//
//	{"type":"session","cwd":"…"}
//	{"type":"message","message":{"role":"user","content":[{"type":"text","text":"…"}]}}
//	{"type":"message","message":{"role":"assistant","content":[…,{"type":"toolCall","name":"bash","arguments":{…}}]}}
//	{"type":"message","message":{"role":"toolResult","toolName":"bash","toolCallId":"…","isError":false,"content":[{"type":"text","text":"…"}]}}
//
// The parser maps these onto the daemon hook vocabulary so pi agents feed the
// same Live feed as hook-based providers (Claude, agy). This format was
// verified against real session files under ~/.pi/agent/sessions on the dev
// machine.

// piSessionsRoot is overridable in tests. It resolves pi's default session
// directory. pi honors PI_CODING_AGENT_SESSION_DIR, but mycel never sets it,
// so the default under the user home is authoritative for mycel-spawned tmux
// agents.
var piSessionsRoot = func() string {
	if dir := os.Getenv("PI_CODING_AGENT_SESSION_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent", "sessions")
}

// encodePiCWD reproduces pi's cwd-to-directory encoding: strip the leading
// "/", replace every remaining "/" with "-", and wrap the result in "--" on
// both sides (so "/Users/x/wt" becomes "--Users-x-wt--"). Verified against
// real directories under ~/.pi/agent/sessions.
func encodePiCWD(cwd string) string {
	return "--" + strings.ReplaceAll(strings.TrimPrefix(cwd, "/"), "/", "-") + "--"
}

// ActivityMode reports that pi exposes activity via an on-disk session
// transcript the daemon tails, not via hook commands.
func (p *PiProvider) ActivityMode() string { return ActivityModeTranscript }

// WriteHookConfig is a no-op: pi has no hook mechanism, activity is sourced by
// tailing its transcript (ActivityModeTranscript).
func (p *PiProvider) WriteHookConfig(_, _, _ string) error { return nil }

// TranscriptGlobs returns the glob matching pi's session transcripts for an
// agent working in cwd. Returns nil when the sessions root or cwd is unknown.
func (p *PiProvider) TranscriptGlobs(cwd string) []string {
	root := piSessionsRoot()
	if root == "" || cwd == "" {
		return nil
	}
	return []string{filepath.Join(root, encodePiCWD(cwd), "*.jsonl")}
}

// piTranscriptLine is one JSONL entry in a pi session file.
type piTranscriptLine struct {
	Message   *piTranscriptMessage `json:"message,omitempty"`
	Type      string               `json:"type"`
	Timestamp string               `json:"timestamp"`
}

// piTranscriptMessage is the message payload of a pi "message" line.
type piTranscriptMessage struct {
	Role     string             `json:"role"`
	ToolName string             `json:"toolName,omitempty"`
	Content  []piTranscriptItem `json:"content"`
	IsError  bool               `json:"isError,omitempty"`
}

// piTranscriptItem is a single content block (text or toolCall).
type piTranscriptItem struct {
	Arguments any    `json:"arguments,omitempty"`
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Name      string `json:"name,omitempty"`
}

// ParseTranscriptLine turns one pi JSONL line into zero or more activity
// events. Unrecognized or malformed lines yield (nil, nil).
func (p *PiProvider) ParseTranscriptLine(line []byte) ([]TranscriptActivity, error) {
	line = trimSpaceBytes(line)
	if len(line) == 0 {
		return nil, nil
	}
	var entry piTranscriptLine
	if err := json.Unmarshal(line, &entry); err != nil {
		return nil, nil //nolint:nilerr // tolerate malformed lines mid-stream
	}

	ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp) //nolint:errcheck // zero time is an acceptable fallback

	switch entry.Type {
	case "session":
		return []TranscriptActivity{{Event: "SessionStart", Timestamp: ts}}, nil
	case "message":
		if entry.Message == nil {
			return nil, nil
		}
		return piMessageActivities(entry.Message, ts), nil
	default:
		return nil, nil
	}
}

// piMessageActivities maps one pi message onto activity events.
func piMessageActivities(m *piTranscriptMessage, ts time.Time) []TranscriptActivity {
	switch m.Role {
	case "user":
		text := piConcatText(m.Content)
		if text == "" {
			return nil
		}
		return []TranscriptActivity{{Event: "UserPromptSubmit", Prompt: text, Timestamp: ts}}
	case "assistant":
		var out []TranscriptActivity
		for i := range m.Content {
			c := &m.Content[i]
			if c.Type == "toolCall" && c.Name != "" {
				out = append(out, TranscriptActivity{
					Event:     "PreToolUse",
					ToolName:  c.Name,
					ToolInput: c.Arguments,
					Timestamp: ts,
				})
			}
		}
		return out
	case "toolResult":
		if m.ToolName == "" {
			return nil
		}
		resp := piConcatText(m.Content)
		if m.IsError {
			return []TranscriptActivity{{
				Event:        "PostToolUseFailure",
				ToolName:     m.ToolName,
				ToolResponse: resp,
				Error:        resp,
				Timestamp:    ts,
			}}
		}
		return []TranscriptActivity{{
			Event:        "PostToolUse",
			ToolName:     m.ToolName,
			ToolResponse: resp,
			Timestamp:    ts,
		}}
	default:
		return nil
	}
}

// piConcatText joins the text of all text content items.
func piConcatText(items []piTranscriptItem) string {
	var b strings.Builder
	for i := range items {
		if items[i].Type == "text" && items[i].Text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(items[i].Text)
		}
	}
	return b.String()
}

// trimSpaceBytes trims surrounding ASCII whitespace without allocating.
func trimSpaceBytes(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

// Ensure PiProvider satisfies the activity/transcript capabilities.
var _ ActivitySource = (*PiProvider)(nil)
var _ TranscriptParser = (*PiProvider)(nil)
