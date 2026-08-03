package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/log"
)

// ErrUnknownHookEvent is returned by IngestHookEvent for events that are not
// part of the hook vocabulary (see IsKnownEvent).
var ErrUnknownHookEvent = errors.New("unknown event")

// HookStateSkippedError reports that a hook event's state transition could
// not be applied (e.g. the agent does not exist). The event is dropped
// entirely — nothing is persisted or broadcast — mirroring the historical
// hook endpoint behavior. Callers treat this as a soft failure.
type HookStateSkippedError struct {
	Err error
}

func (e *HookStateSkippedError) Error() string {
	return "hook state update skipped: " + e.Err.Error()
}

func (e *HookStateSkippedError) Unwrap() error { return e.Err }

// HookEventAppender persists ingested hook events. It is the Append subset
// of events.EventStore, declared here so the service depends only on what
// ingestion needs.
type HookEventAppender interface {
	Append(event events.Event) error
}

// SetHookEventStore sets the sink used to persist ingested hook events.
// nil disables persistence. Wired once at daemon startup, before serving.
func (s *AgentService) SetHookEventStore(store HookEventAppender) {
	s.hookStore = store
}

// SetOnHookEvent registers a callback invoked after every successfully
// ingested hook event with the event's SSE-shaped payload. The HTTP layer
// uses it to feed per-agent SSE subscribers (GET /api/agents/{name}/events)
// without the service knowing about SSE framing. Wired once at daemon
// startup, before serving.
func (s *AgentService) SetOnHookEvent(fn func(agentName string, ts time.Time, payload map[string]any)) {
	s.onHookEvent = fn
}

// IngestHookEvent applies a hook event to an agent: state transition, event
// persistence, and broadcast. Transport-agnostic — HTTP hooks and (future)
// transcript tailers share this one path.
//
// raw is the original JSON encoding of the payload; it is preserved verbatim
// in the event log and the hub broadcast for full observability. When raw is
// empty the payload is re-encoded.
//
// Returns ErrUnknownHookEvent for unrecognized events and
// *HookStateSkippedError when the event maps to a state transition that
// cannot be applied (in which case nothing is persisted or broadcast).
func (s *AgentService) IngestHookEvent(ctx context.Context, name string, payload HookPayload, raw []byte) error {
	if !IsKnownEvent(payload.Event) {
		return fmt.Errorf("%w: %s", ErrUnknownHookEvent, payload.Event)
	}
	if len(raw) == 0 {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode hook payload: %w", err)
		}
		raw = encoded
	}

	// Determine target state: explicit in payload > mapped from event > no change
	targetState, hasState := StateForHookEvent(payload.Event)
	if payload.State != "" && IsValidState(payload.State) {
		targetState = State(payload.State)
		hasState = true
	}

	if hasState {
		// State-only update: lifecycle descriptions baked into hook
		// commands ("Turn complete", "Session ended", "Processing
		// prompt...") must NOT become the agent's task line. They still
		// flow to the event log and SSE stream below.
		if err := s.manager.SetAgentState(ctx, name, targetState); err != nil {
			log.Debug("hook state update skipped", "agent", name, "error", err)
			return &HookStateSkippedError{Err: err}
		}
	}

	// The prompt an agent was just given is its task. Applying it here is what
	// makes the task line a property of observed activity rather than something
	// the agent has to remember to publish: it updates on every turn, for every
	// provider that reports one, and it cannot go stale while the agent works.
	//
	// A payload may name a state explicitly, so a turn-start event can arrive
	// carrying state=stopped. That transition clears the task by design; writing
	// the prompt afterwards would refill it and leave a dead agent advertising
	// work it will never do, so nothing is derived when the agent just stopped.
	if isTurnStart(payload.Event) && payload.Prompt != "" && targetState != StateStopped {
		task := truncateRunes(payload.Prompt, maxDerivedTaskLen)
		if err := s.manager.SetAgentTask(ctx, name, task); err != nil {
			// Non-fatal: the event itself is still worth logging and
			// broadcasting even if the task line could not be updated.
			log.Debug("hook task update skipped", "agent", name, "error", err)
		}
	}

	now := time.Now()
	fields := hookPayloadFields(payload)

	// Persist raw JSON body to event log — raw body in Message for full
	// observability; structured fields in Data for typed queries.
	if s.hookStore != nil {
		eventData := map[string]any{"event": string(payload.Event)}
		for k, v := range fields {
			eventData[k] = v
		}
		if payload.Message != "" {
			eventData["message"] = payload.Message
		}
		_ = s.hookStore.Append(events.Event{ //nolint:errcheck // best-effort logging
			Timestamp: now,
			Type:      events.EventType("hook." + string(payload.Event)),
			Agent:     name,
			Message:   string(raw),
			Data:      eventData,
		})
	}

	// Notify the registered per-agent event callback (SSE subscribers).
	if s.onHookEvent != nil {
		ssePayload := map[string]any{
			"event":     string(payload.Event),
			"timestamp": now.UTC().Format(time.RFC3339Nano),
			"agent":     name,
		}
		for k, v := range fields {
			ssePayload[k] = v
		}
		s.onHookEvent(name, now, ssePayload)
	}

	// Broadcast raw hook JSON on the process-wide hub for the web UI —
	// same format as the event log.
	var rawMap map[string]any
	if err := json.Unmarshal(raw, &rawMap); err == nil {
		rawMap["agent"] = name
		s.publishEvent("agent.hook", rawMap)
	}

	return nil
}

// isTurnStart reports whether an event marks the beginning of a model turn,
// i.e. the moment the agent is handed something to do. Both events that move an
// agent to "working" qualify: claude and cursor report UserPromptSubmit, while
// agy reports PreInvocation. Deriving the task from either means a provider's
// choice of name does not decide whether its agents get a task line.
func isTurnStart(ev HookEvent) bool {
	return ev == HookUserPromptSubmit || ev == HookPreInvocation
}

// maxDerivedTaskLen bounds the prompt text used as an agent's task line. The
// task line is a single row in a table and a badge subtitle, so it is cut to
// something that fits one line rather than to the prompt's own length.
const maxDerivedTaskLen = 120

// maxToolResponseBytes bounds how much of a tool's response we persist into
// the event log and broadcast to live subscribers. PostToolUse responses
// (file contents, command output, MCP results, …) can be arbitrarily large;
// without a cap a single verbose tool call could bloat the event store or a
// DB row. 16KB keeps enough of the response to be useful in the raw stream
// while staying well clear of pathological growth.
const maxToolResponseBytes = 16 * 1024

// toolResponseTruncatedSuffix is appended when a response is cut down to
// maxToolResponseBytes, so the UI and any consumer can tell the value was
// shortened rather than naturally ending there.
const toolResponseTruncatedSuffix = "…[truncated]"

// boundedToolResponse caps a tool_response value to maxToolResponseBytes
// before it is persisted or broadcast. String responses are truncated
// directly; structured responses (maps/arrays) are marshaled to measure
// their size and, if oversized, replaced with a truncated JSON string so the
// bound is enforced regardless of shape.
func boundedToolResponse(v any) any {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		if len(s) <= maxToolResponseBytes {
			return s
		}
		return truncateToRuneBoundary(s, maxToolResponseBytes) + toolResponseTruncatedSuffix
	}
	b, err := json.Marshal(v)
	if err != nil {
		// Unmarshalable value (shouldn't happen for JSON-decoded payloads) —
		// pass it through unchanged rather than dropping it.
		return v
	}
	if len(b) <= maxToolResponseBytes {
		return v
	}
	return truncateToRuneBoundary(string(b), maxToolResponseBytes) + toolResponseTruncatedSuffix
}

// truncateRunes returns s cut to at most max runes, marking a shortened result
// with an ellipsis. Runes rather than bytes because the result is shown to a
// person: a task line should be cut to a predictable visible length regardless
// of how many bytes each character happens to occupy.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// truncateToRuneBoundary returns s cut to at most maxBytes bytes without
// splitting a multi-byte UTF-8 rune: if the byte cap lands in the middle of
// a rune, it backs up to the start of that rune so the result is always
// valid UTF-8.
func truncateToRuneBoundary(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// hookPayloadFields extracts the optional structured fields shared by the
// event-log Data map and the SSE payload. Message is intentionally absent:
// it goes to the event log only, never the SSE payload.
func hookPayloadFields(payload HookPayload) map[string]any {
	fields := make(map[string]any)
	if payload.ToolName != "" {
		fields["tool_name"] = payload.ToolName
	}
	if payload.ToolInput != nil {
		fields["tool_input"] = payload.ToolInput
	}
	if payload.ToolResponse != nil {
		fields["tool_response"] = boundedToolResponse(payload.ToolResponse)
	}
	if payload.Error != "" {
		fields["error"] = payload.Error
	}
	if payload.Task != "" {
		fields["task"] = payload.Task
	}
	if payload.TaskID != "" {
		fields["task_id"] = payload.TaskID
	}
	if payload.TaskTitle != "" {
		fields["task_title"] = payload.TaskTitle
	}
	if len(payload.Metadata) > 0 {
		fields["metadata"] = payload.Metadata
	}
	return fields
}
