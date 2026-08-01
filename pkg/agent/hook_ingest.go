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
		// prompt...") must NOT overwrite the agent's reported task.
		// They still flow to the event log and SSE stream below.
		if err := s.manager.SetAgentState(ctx, name, targetState); err != nil {
			log.Debug("hook state update skipped", "agent", name, "error", err)
			return &HookStateSkippedError{Err: err}
		}
	}

	now := time.Now()
	fields := hookPayloadFields(payload)

	// Persist raw JSON body to event log — raw body in Message for full
	// observability; structured fields in Data for typed queries. Track
	// whether this succeeded so the publishEvent fallback below can persist
	// as a backup on failure without double-writing on success.
	persisted := false
	if s.hookStore != nil {
		eventData := map[string]any{"event": string(payload.Event)}
		for k, v := range fields {
			eventData[k] = v
		}
		if payload.Message != "" {
			eventData["message"] = payload.Message
		}
		if err := s.hookStore.Append(events.Event{
			Timestamp: now,
			Type:      events.EventType("hook." + string(payload.Event)),
			Agent:     name,
			Message:   string(raw),
			Data:      eventData,
		}); err != nil {
			log.Debug("hook event append failed", "agent", name, "event", payload.Event, "error", err)
		} else {
			persisted = true
		}
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
		// alreadyPersisted skips the store append in the success case (we
		// persisted directly above); on a direct-append failure it lets the
		// fallback persist so the event is never silently lost.
		s.publishEventPersisted("agent.hook", rawMap, persisted)
	}

	return nil
}

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
