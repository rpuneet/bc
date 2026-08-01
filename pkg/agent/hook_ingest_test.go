package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/rpuneet/mycel/pkg/events"
)

// fakeHookAppender records events passed to Append, standing in for the
// real event store so ingestion tests can assert on persisted Data without
// touching disk.
type fakeHookAppender struct {
	events []events.Event
}

func (f *fakeHookAppender) Append(event events.Event) error {
	f.events = append(f.events, event)
	return nil
}

// TestIngestHookEvent_PersistsToolResponse verifies the fix for the "no
// output in the raw stream" bug: a PostToolUse hook payload carrying
// tool_response must land in the persisted event's Data map (consumed by
// /api/agents/{name}/activity) and in the live SSE payload (onHookEvent),
// not just the WebSocket hub broadcast.
func TestIngestHookEvent_PersistsToolResponse(t *testing.T) {
	mgr := newTestManager(t)
	svc := NewAgentService(mgr, nil, nil)

	appender := &fakeHookAppender{}
	svc.SetHookEventStore(appender)

	var ssePayloads []map[string]any
	svc.SetOnHookEvent(func(_ string, _ time.Time, payload map[string]any) {
		ssePayloads = append(ssePayloads, payload)
	})

	payload := HookPayload{
		Event:        HookPostToolUse,
		ToolName:     "Bash",
		ToolInput:    map[string]any{"command": "echo hi"},
		ToolResponse: map[string]any{"stdout": "hi\n", "stderr": ""},
	}

	if err := svc.IngestHookEvent(context.Background(), "does-not-exist", payload, nil); err != nil {
		t.Fatalf("IngestHookEvent: %v", err)
	}

	if len(appender.events) != 1 {
		t.Fatalf("expected 1 persisted event, got %d", len(appender.events))
	}
	got, ok := appender.events[0].Data["tool_response"]
	if !ok {
		t.Fatal("persisted event Data missing tool_response")
	}
	gotMap, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("tool_response type = %T, want map[string]any", got)
	}
	if gotMap["stdout"] != "hi\n" {
		t.Errorf("tool_response.stdout = %v, want %q", gotMap["stdout"], "hi\n")
	}

	if len(ssePayloads) != 1 {
		t.Fatalf("expected 1 SSE payload, got %d", len(ssePayloads))
	}
	if _, ok := ssePayloads[0]["tool_response"]; !ok {
		t.Error("live SSE payload missing tool_response")
	}
}

// TestIngestHookEvent_NoToolResponse verifies events without a
// tool_response (e.g. PreToolUse) don't gain a spurious key.
func TestIngestHookEvent_NoToolResponse(t *testing.T) {
	mgr := newTestManager(t)
	svc := NewAgentService(mgr, nil, nil)

	appender := &fakeHookAppender{}
	svc.SetHookEventStore(appender)

	payload := HookPayload{
		Event:     HookPreToolUse,
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "echo hi"},
	}

	if err := svc.IngestHookEvent(context.Background(), "does-not-exist", payload, nil); err != nil {
		t.Fatalf("IngestHookEvent: %v", err)
	}
	if len(appender.events) != 1 {
		t.Fatalf("expected 1 persisted event, got %d", len(appender.events))
	}
	if _, ok := appender.events[0].Data["tool_response"]; ok {
		t.Error("Data should not contain tool_response when payload has none")
	}
}

func TestBoundedToolResponse(t *testing.T) {
	t.Run("nil passthrough", func(t *testing.T) {
		if got := boundedToolResponse(nil); got != nil {
			t.Errorf("boundedToolResponse(nil) = %v, want nil", got)
		}
	})

	t.Run("small string unchanged", func(t *testing.T) {
		s := "hello world"
		if got := boundedToolResponse(s); got != s {
			t.Errorf("got %v, want %q", got, s)
		}
	})

	t.Run("oversized string truncated", func(t *testing.T) {
		big := strings.Repeat("a", maxToolResponseBytes+500)
		got, ok := boundedToolResponse(big).(string)
		if !ok {
			t.Fatalf("expected string result, got %T", got)
		}
		if !strings.HasSuffix(got, toolResponseTruncatedSuffix) {
			t.Errorf("truncated string missing suffix marker: %q", got[len(got)-30:])
		}
		if len(got) != maxToolResponseBytes+len(toolResponseTruncatedSuffix) {
			t.Errorf("truncated length = %d, want %d", len(got), maxToolResponseBytes+len(toolResponseTruncatedSuffix))
		}
	})

	t.Run("small structured value unchanged", func(t *testing.T) {
		v := map[string]any{"stdout": "ok", "stderr": ""}
		got := boundedToolResponse(v)
		gotMap, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("expected map passthrough, got %T", got)
		}
		if gotMap["stdout"] != "ok" {
			t.Errorf("stdout = %v, want ok", gotMap["stdout"])
		}
	})

	t.Run("oversized structured value truncated to string", func(t *testing.T) {
		v := map[string]any{"stdout": strings.Repeat("b", maxToolResponseBytes+500)}
		got := boundedToolResponse(v)
		s, ok := got.(string)
		if !ok {
			t.Fatalf("expected string fallback for oversized structured value, got %T", got)
		}
		if !strings.HasSuffix(s, toolResponseTruncatedSuffix) {
			t.Errorf("missing truncation suffix")
		}
		if len(s) != maxToolResponseBytes+len(toolResponseTruncatedSuffix) {
			t.Errorf("truncated length = %d, want %d", len(s), maxToolResponseBytes+len(toolResponseTruncatedSuffix))
		}
	})

	t.Run("oversized multi-byte string stays valid UTF-8", func(t *testing.T) {
		// "тест" is 4 Cyrillic runes = 8 bytes; repeating it past the cap
		// guarantees the byte cap lands mid-rune, so a naive byte slice
		// would emit invalid UTF-8.
		big := strings.Repeat("тест", maxToolResponseBytes/8+100)
		got, ok := boundedToolResponse(big).(string)
		if !ok {
			t.Fatalf("expected string result, got %T", got)
		}
		if !utf8.ValidString(got) {
			t.Error("truncated multi-byte string is not valid UTF-8")
		}
		if !strings.HasSuffix(got, toolResponseTruncatedSuffix) {
			t.Errorf("truncated string missing suffix marker: %q", got[len(got)-30:])
		}
		// The body (minus the multi-byte suffix marker) must not exceed the
		// byte cap — the rune-boundary backup only ever shrinks it.
		body := strings.TrimSuffix(got, toolResponseTruncatedSuffix)
		if len(body) > maxToolResponseBytes {
			t.Errorf("truncated body length = %d, exceeds cap %d", len(body), maxToolResponseBytes)
		}
	})
}

// TestHookPayloadFields_ToolResponseRoundtrip verifies HookPayload correctly
// decodes tool_response from raw hook JSON — this is the field that was
// previously silently dropped by json.Unmarshal because HookPayload had no
// matching struct field.
func TestHookPayloadFields_ToolResponseRoundtrip(t *testing.T) {
	raw := []byte(`{"event":"PostToolUse","tool_name":"Bash","tool_response":{"stdout":"ok"}}`)
	var payload HookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.ToolResponse == nil {
		t.Fatal("payload.ToolResponse is nil after unmarshal")
	}

	fields := hookPayloadFields(payload)
	if _, ok := fields["tool_response"]; !ok {
		t.Error("hookPayloadFields did not include tool_response")
	}
}
