// Package provider — TranscriptParser capability.
//
// Providers driven in ActivityModeTranscript (they write a readable session
// log rather than invoking hook commands) implement TranscriptParser so the
// daemon's transcript tailer can turn appended session lines into Live
// activity events. The parsed events reuse the daemon's hook vocabulary
// (see pkg/agent HookEvent constants) so hook-based and transcript-based
// providers feed the exact same Live feed with no parallel UI.
package provider

import "time"

// TranscriptActivity is a single lifecycle/tool event derived from one line
// (or one message) of a provider transcript, mapped onto the daemon hook
// vocabulary. A single transcript line may yield zero, one, or several of
// these (e.g. an assistant turn that issues two tool calls).
type TranscriptActivity struct {
	// Timestamp is when the underlying transcript entry was recorded. Zero
	// when the transcript does not carry one; the tailer falls back to the
	// ingestion time in that case.
	Timestamp time.Time
	// Event is the hook event name this activity maps to, e.g.
	// "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure",
	// "Stop", or "SessionStart". It must be a value IsKnownEvent accepts.
	Event string
	// ToolName is the tool being invoked/completed for PreToolUse/PostToolUse
	// events, empty otherwise.
	ToolName string
	// ToolInput is the decoded tool arguments for PreToolUse, nil otherwise.
	ToolInput any
	// ToolResponse is the decoded tool result for PostToolUse events, nil
	// otherwise.
	ToolResponse any
	// Prompt is the user's message text for UserPromptSubmit events.
	Prompt string
	// Error is the failure message for PostToolUseFailure events.
	Error string
}

// TranscriptParser is optionally implemented by providers whose on-disk
// session transcript can be parsed into Live activity events. Providers in
// ActivityModeTranscript should implement it; providers whose transcript is
// not parseable (or nonexistent) simply don't, and the Live feed shows an
// honest empty state for their agents.
type TranscriptParser interface {
	// ParseTranscriptLine parses a single transcript line into zero or more
	// activity events. It must be stateless and tolerant: unrecognized or
	// malformed lines return (nil, nil) rather than an error so the tailer
	// can skip them without aborting the stream.
	ParseTranscriptLine(line []byte) ([]TranscriptActivity, error)
}

// TranscriptSession parses one provider transcript file with per-file state.
// Unlike TranscriptParser (stateless, each line independent), the tailer
// creates one session per file it follows and feeds that file's lines to it in
// order, so the session can correlate lines that reference each other. codex
// needs this: its tool-result lines carry only the originating call's id and no
// tool name, so the session resolves each result against the call it recorded
// earlier to emit a correctly-paired PostToolUse.
type TranscriptSession interface {
	// ParseLine parses one transcript line into zero or more activity events.
	// Like TranscriptParser it must tolerate malformed or unrecognized lines by
	// returning (nil, nil) rather than an error.
	ParseLine(line []byte) ([]TranscriptActivity, error)
}

// TranscriptSessionParser is implemented by transcript providers that need
// per-file state to parse correctly (e.g. codex). Providers whose lines are
// independently parseable implement the simpler stateless TranscriptParser
// instead; a provider implements at most one of the two.
type TranscriptSessionParser interface {
	// NewTranscriptSession returns a fresh session bound to a single transcript
	// file. The tailer calls it once when it starts following a file and again
	// when the file rotates, never sharing a session across files.
	NewTranscriptSession() TranscriptSession
}

// TranscriptFileSelector is implemented by transcript providers whose active
// session file cannot be located by a cwd-encoded path glob because the working
// directory is recorded inside the file rather than in its path. codex keys its
// files by date (sessions/YYYY/MM/DD/rollout-*.jsonl) and records the cwd in a
// session_meta line, so the tailer calls SelectTranscript to resolve the file
// instead of globbing TranscriptGlobs.
type TranscriptFileSelector interface {
	// SelectTranscript returns the path to the newest transcript belonging to
	// the agent working in cwd, or "" when none matches.
	SelectTranscript(cwd string) string
}
