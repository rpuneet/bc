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
