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

// ─── task line derived from the prompt ──────────────────────────────────────

// The task line must come from the activity stream. Before this, the only way
// an agent's task was ever set was the report_status MCP tool, so an agent that
// never called it — or forgot to after moving on — showed nothing or something
// stale while the daemon knew exactly what it had been asked to do.
func TestIngestHookEvent_UserPromptBecomesTask(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateIdle}
	svc := NewAgentService(mgr, nil, nil)

	payload := HookPayload{Event: HookUserPromptSubmit, Prompt: "fix the flaky login test"}
	if err := svc.IngestHookEvent(context.Background(), "eng-1", payload, nil); err != nil {
		t.Fatalf("IngestHookEvent: %v", err)
	}

	if got := mgr.agents["eng-1"].Task; got != "fix the flaky login test" {
		t.Errorf("task = %q, want the prompt text", got)
	}
	// The event still drives the state transition it always did.
	if got := mgr.agents["eng-1"].State; got != StateWorking {
		t.Errorf("state = %q, want %q", got, StateWorking)
	}
}

// A long prompt is cut to one line's worth of text rather than pushed whole
// into a table cell.
func TestIngestHookEvent_LongPromptIsTruncated(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateIdle}
	svc := NewAgentService(mgr, nil, nil)

	long := strings.Repeat("a", maxDerivedTaskLen*2)
	payload := HookPayload{Event: HookUserPromptSubmit, Prompt: long}
	if err := svc.IngestHookEvent(context.Background(), "eng-1", payload, nil); err != nil {
		t.Fatalf("IngestHookEvent: %v", err)
	}

	got := mgr.agents["eng-1"].Task
	if n := utf8.RuneCountInString(got); n != maxDerivedTaskLen {
		t.Errorf("task length = %d runes, want %d", n, maxDerivedTaskLen)
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("a truncated task should be marked with an ellipsis")
	}
}

// Only a user turn sets the task. Tool events fire many times per turn, and a
// lifecycle event's canned label ("Turn complete") is not a task — letting
// either through is how the task line used to get clobbered (#3259).
func TestIngestHookEvent_OnlyUserPromptSetsTask(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload HookPayload
	}{
		{"tool call", HookPayload{Event: HookPreToolUse, ToolName: "Bash", Prompt: "ignored"}},
		{"turn end with canned task", HookPayload{Event: HookStop, Task: "Turn complete"}},
		{"session start with canned task", HookPayload{Event: HookSessionStart, Task: "Session started"}},
		{"empty prompt", HookPayload{Event: HookUserPromptSubmit, Prompt: ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newTestManager(t)
			mgr.agents["eng-1"] = &Agent{
				Name: "eng-1", Role: Role("engineer"),
				State: StateWorking, Task: "the real task",
			}
			svc := NewAgentService(mgr, nil, nil)

			if err := svc.IngestHookEvent(context.Background(), "eng-1", tc.payload, nil); err != nil {
				t.Fatalf("IngestHookEvent: %v", err)
			}
			// Stop clears the task via the stopped transition; every other
			// case must leave it exactly as it was.
			if got := mgr.agents["eng-1"].Task; got != "the real task" {
				t.Errorf("task = %q, want it left untouched", got)
			}
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
		max  int
	}{
		{"under the cap is unchanged", "short", "short", 10},
		{"exactly at the cap is unchanged", "12345", "12345", 5},
		{"over the cap is elided", "123456", "1234…", 5},
		// Runes, not bytes: a multi-byte string under the rune cap must
		// survive whole rather than being cut by its byte length.
		{"multi-byte under the cap", "héllo wörld", "héllo wörld", 11},
		{"multi-byte over the cap", "héllo wörld", "héllo…", 6},
		{"cap of one truncates without an ellipsis", "abc", "a", 1},
		{"cap of zero is empty", "abc", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncateRunes(tc.in, tc.max); got != tc.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
		})
	}
}

// A real claude UserPromptSubmit body carries both the user's prompt and the
// canned label mycel's hook command merges in. The prompt must survive decoding
// — a field absent from HookPayload is silently dropped by json.Unmarshal, which
// is exactly how tool_response went missing — and it must win over the label.
func TestIngestHookEvent_DecodesPromptFromRawHookBody(t *testing.T) {
	raw := []byte(`{"session_id":"abc","prompt":"add retries to the uploader",` +
		`"event":"UserPromptSubmit","state":"working","task":"Processing prompt..."}`)
	var payload HookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Prompt != "add retries to the uploader" {
		t.Fatalf("payload.Prompt = %q, want the prompt text", payload.Prompt)
	}

	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateIdle}
	svc := NewAgentService(mgr, nil, nil)

	if err := svc.IngestHookEvent(context.Background(), "eng-1", payload, raw); err != nil {
		t.Fatalf("IngestHookEvent: %v", err)
	}
	if got := mgr.agents["eng-1"].Task; got != "add retries to the uploader" {
		t.Errorf("task = %q, want the prompt rather than the canned label", got)
	}
}

// agy names its turn-start event PreInvocation. Its agents must get a task line
// on the same terms as claude's, or removing report_status would leave them with
// none at all.
func TestIngestHookEvent_PreInvocationPromptBecomesTask(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateIdle}
	svc := NewAgentService(mgr, nil, nil)

	payload := HookPayload{Event: HookPreInvocation, Prompt: "audit the auth middleware"}
	if err := svc.IngestHookEvent(context.Background(), "eng-1", payload, nil); err != nil {
		t.Fatalf("IngestHookEvent: %v", err)
	}
	if got := mgr.agents["eng-1"].Task; got != "audit the auth middleware" {
		t.Errorf("task = %q, want the prompt text", got)
	}
}

// A payload may name its state explicitly, so a turn-start event can arrive
// carrying state=stopped. The stopped transition clears the task by design;
// deriving the prompt afterwards would refill it and leave a dead agent
// advertising work it will never do.
func TestIngestHookEvent_StoppedTurnStartLeavesTaskCleared(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{
		Name: "eng-1", Role: Role("engineer"),
		State: StateWorking, Task: "an earlier task",
	}
	svc := NewAgentService(mgr, nil, nil)

	payload := HookPayload{
		Event:  HookUserPromptSubmit,
		State:  string(StateStopped),
		Prompt: "this agent is going away",
	}
	if err := svc.IngestHookEvent(context.Background(), "eng-1", payload, nil); err != nil {
		t.Fatalf("IngestHookEvent: %v", err)
	}

	if got := mgr.agents["eng-1"].State; got != StateStopped {
		t.Fatalf("state = %q, want %q", got, StateStopped)
	}
	if got := mgr.agents["eng-1"].Task; got != "" {
		t.Errorf("task = %q, want it cleared for a stopped agent", got)
	}
}

// An agent driven through a gateway is handed the platform's whole delivery
// envelope, so its prompt is JSON and its task line read `{"raw":{"client_m…`.
// The envelope is what the transport needs; the sentence inside it is what a
// person typed and the only part worth showing (#3536).
func TestTaskFromPrompt(t *testing.T) {
	// Trimmed from a real Slack delivery, keeping the shape and field order.
	slackEnvelope := `{"raw":{"client_msg_id":"1232B1A4","type":"message",` +
		`"user":"U0AN7GW037H","text":"Update when you have something"},` +
		`"timestamp":"2026-08-03T14:36:15Z","channel":"slack:general",` +
		`"platform":"slack","sender":"[slack] Puneet Rai",` +
		`"content":"Update when you have something"}`

	tests := []struct {
		name   string
		prompt string
		want   string
		why    string
	}{
		{
			name:   "a delivery envelope yields the message",
			prompt: slackEnvelope,
			want:   "Update when you have something",
			why:    "the task line should read as the request, not as its transport",
		},
		{
			name:   "an ordinary prompt is untouched",
			prompt: "fix the login bug",
			want:   "fix the login bug",
			why:    "most prompts are not JSON and must pass through unchanged",
		},
		{
			name:   "JSON a person pasted is left alone",
			prompt: `{"content":"some config value","other":"field"}`,
			want:   `{"content":"some config value","other":"field"}`,
			why:    "without a delivery field this is data the agent was given, not an envelope",
		},
		{
			name:   "an envelope with an empty message keeps the whole prompt",
			prompt: `{"platform":"slack","sender":"someone","content":"   "}`,
			want:   `{"platform":"slack","sender":"someone","content":"   "}`,
			why:    "an empty task line is worse than an ugly one",
		},
		{
			name:   "malformed JSON keeps the whole prompt",
			prompt: `{"platform":"slack","content":"truncated`,
			want:   `{"platform":"slack","content":"truncated`,
			why:    "a parse failure must not lose the prompt",
		},
		{
			name:   "a prompt that merely starts with a brace is untouched",
			prompt: "{ this is not json at all",
			want:   "{ this is not json at all",
			why:    "the brace alone means nothing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := taskFromPrompt(tt.prompt); got != tt.want {
				t.Errorf("taskFromPrompt() = %q, want %q — %s", got, tt.want, tt.why)
			}
		})
	}
}

// The derivation runs through ingestion, so the task an agent ends up
// advertising is the readable one.
func TestIngestHookEvent_GatewayEnvelopeBecomesAReadableTask(t *testing.T) {
	mgr := newTestManager(t)
	mgr.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateIdle}
	svc := NewAgentService(mgr, nil, nil)

	payload := HookPayload{
		Event: HookUserPromptSubmit,
		Prompt: `{"timestamp":"2026-08-03T14:36:15Z","channel":"slack:general",` +
			`"platform":"slack","sender":"[slack] Puneet Rai",` +
			`"content":"review both trackers and make sure main is good"}`,
	}
	if err := svc.IngestHookEvent(context.Background(), "eng-1", payload, nil); err != nil {
		t.Fatalf("IngestHookEvent: %v", err)
	}

	if got := mgr.agents["eng-1"].Task; got != "review both trackers and make sure main is good" {
		t.Errorf("task = %q, want the message rather than its envelope", got)
	}
}
