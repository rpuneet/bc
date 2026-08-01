package server

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/provider"
)

// recordingPublisher captures agent.hook broadcasts — the exact events the web
// Live feed consumes over the WebSocket hub.
type recordingPublisher struct {
	events []map[string]any
	mu     sync.Mutex
}

func (r *recordingPublisher) Publish(eventType string, data map[string]any) {
	if eventType != "agent.hook" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[string]any, len(data))
	for k, v := range data {
		cp[k] = v
	}
	r.events = append(r.events, cp)
}

func (r *recordingPublisher) snapshot() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]map[string]any, len(r.events))
	copy(out, r.events)
	return out
}

func TestReadNewLines_PartialTrailingLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.jsonl")
	// A complete line followed by a partial (no trailing newline) line.
	if err := os.WriteFile(path, []byte("{\"a\":1}\n{\"partial\""), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	lines, consumed := readNewLines(path, 0, fi.Size())
	if len(lines) != 1 || string(lines[0]) != `{"a":1}` {
		t.Fatalf("lines = %q, want one complete line", lines)
	}
	if consumed != int64(len("{\"a\":1}\n")) {
		t.Fatalf("consumed = %d, want %d (partial line left for next tick)", consumed, len("{\"a\":1}\n"))
	}
}

// TestTranscriptTailer_LiveCapture is the in-process live-capture check: it
// feeds a real pi transcript (seed at EOF, then append activity) and asserts
// the parsed tool events publish onto the agent.hook feed — the same path the
// web Live tab renders.
func TestTranscriptTailer_LiveCapture(t *testing.T) {
	// Isolate the agent service under a throwaway home; no runtime backend.
	repo := t.TempDir()
	t.Setenv("MYCEL_HOME", t.TempDir())
	mgr := agentpkg.NewManagerWithRepo(filepath.Join(t.TempDir(), "agents"), repo)
	pub := &recordingPublisher{}
	svc := agentpkg.NewAgentService(mgr, pub, nil)

	pi, ok := provider.DefaultRegistry.Get("pi")
	if !ok {
		t.Fatal("pi provider not registered")
	}
	if _, isParser := pi.(provider.TranscriptParser); !isParser {
		t.Fatal("pi provider does not implement TranscriptParser")
	}

	sessDir := t.TempDir()
	transcript := filepath.Join(sessDir, "session.jsonl")

	// Seed with a session line already present — the tailer must NOT replay it
	// (it starts at EOF on first sighting).
	seed := `{"type":"session","cwd":"/x","timestamp":"2026-07-06T13:36:41.507Z"}` + "\n"
	if err := os.WriteFile(transcript, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	cursors := map[string]*tailCursor{}

	// First tick: seeds the cursor at EOF, captures nothing.
	tailAgentFile(ctx, svc, cursors, "pilot", transcript, pi)
	if got := pub.snapshot(); len(got) != 0 {
		t.Fatalf("first tick published %d events, want 0 (seed-at-EOF)", len(got))
	}

	// pi appends a user turn, an assistant tool call, and its result.
	appended := `{"type":"message","timestamp":"2026-07-06T13:36:42Z","message":{"role":"user","content":[{"type":"text","text":"list files"}]}}` + "\n" +
		`{"type":"message","timestamp":"2026-07-06T13:36:43Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"functions.bash:0","name":"bash","arguments":{"command":"ls -la"}}]}}` + "\n" +
		`{"type":"message","timestamp":"2026-07-06T13:36:44Z","message":{"role":"toolResult","toolCallId":"functions.bash:0","toolName":"bash","isError":false,"content":[{"type":"text","text":"a.txt\n"}]}}` + "\n"
	f, err := os.OpenFile(transcript, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // test-controlled temp path
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(appended); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	// Second tick: captures the appended activity.
	tailAgentFile(ctx, svc, cursors, "pilot", transcript, pi)

	got := pub.snapshot()
	byEvent := map[string]map[string]any{}
	for _, e := range got {
		if ev, _ := e["event"].(string); ev != "" {
			byEvent[ev] = e
		}
		if a, _ := e["agent"].(string); a != "pilot" {
			t.Errorf("event has agent=%q, want pilot", a)
		}
	}

	// PreToolUse/PostToolUse are informational and always publish. The
	// UserPromptSubmit event additionally drives a state transition, which is
	// dropped in this lightweight harness (no DB-backed state store wired), so
	// it is not asserted here — the full daemon path is exercised by the
	// live-server run documented in the PR.
	pre, ok := byEvent["PreToolUse"]
	if !ok {
		t.Fatalf("missing PreToolUse; got events %v", eventNames(got))
	}
	if pre["tool_name"] != "bash" {
		t.Errorf("PreToolUse tool_name = %v, want bash", pre["tool_name"])
	}
	post, ok := byEvent["PostToolUse"]
	if !ok {
		t.Fatalf("missing PostToolUse; got events %v", eventNames(got))
	}
	if post["tool_name"] != "bash" {
		t.Errorf("PostToolUse tool_name = %v, want bash", post["tool_name"])
	}
}

// TestTranscriptTailer_CodexLiveCapture is the codex analogue of the pi live
// check: it seeds a real-shape codex rollout at EOF, appends a user turn plus a
// paired shell tool call/output (whose result line carries only the call_id),
// and asserts the session-based path publishes PreToolUse/PostToolUse with the
// tool name correctly carried across onto the agent.hook feed.
func TestTranscriptTailer_CodexLiveCapture(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("MYCEL_HOME", t.TempDir())
	mgr := agentpkg.NewManagerWithRepo(filepath.Join(t.TempDir(), "agents"), repo)
	pub := &recordingPublisher{}
	svc := agentpkg.NewAgentService(mgr, pub, nil)

	codex, ok := provider.DefaultRegistry.Get("codex")
	if !ok {
		t.Fatal("codex provider not registered")
	}
	if _, isSession := codex.(provider.TranscriptSessionParser); !isSession {
		t.Fatal("codex provider does not implement TranscriptSessionParser")
	}

	transcript := filepath.Join(t.TempDir(), "rollout.jsonl")
	// Seed with the session_meta header already present — the tailer must start
	// at EOF and not replay it.
	seed := `{"timestamp":"2026-05-02T08:29:51.690Z","type":"session_meta","payload":{"cwd":"/wt/eng","originator":"codex-tui"}}` + "\n"
	if err := os.WriteFile(transcript, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	cursors := map[string]*tailCursor{}

	tailAgentFile(ctx, svc, cursors, "pilot", transcript, codex)
	if got := pub.snapshot(); len(got) != 0 {
		t.Fatalf("first tick published %d events, want 0 (seed-at-EOF)", len(got))
	}

	appended := `{"timestamp":"2026-05-02T08:30:00Z","type":"event_msg","payload":{"type":"user_message","message":"list files"}}` + "\n" +
		`{"timestamp":"2026-05-02T08:30:01Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"ls\"}","call_id":"call_A"}}` + "\n" +
		`{"timestamp":"2026-05-02T08:30:02Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_A","output":"Exit code: 0\nWall time: 0.1 seconds\nOutput:\nREADME.md\n"}}` + "\n"
	f, err := os.OpenFile(transcript, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // test-controlled temp path
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(appended); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	tailAgentFile(ctx, svc, cursors, "pilot", transcript, codex)

	byEvent := map[string]map[string]any{}
	for _, e := range pub.snapshot() {
		if ev, _ := e["event"].(string); ev != "" {
			byEvent[ev] = e
		}
	}
	pre, ok := byEvent["PreToolUse"]
	if !ok {
		t.Fatalf("missing PreToolUse; got %v", eventNames(pub.snapshot()))
	}
	if pre["tool_name"] != "shell_command" {
		t.Errorf("PreToolUse tool_name = %v, want shell_command", pre["tool_name"])
	}
	post, ok := byEvent["PostToolUse"]
	if !ok {
		t.Fatalf("missing PostToolUse; got %v", eventNames(pub.snapshot()))
	}
	if post["tool_name"] != "shell_command" {
		t.Errorf("PostToolUse tool_name = %v, want shell_command (paired via call_id)", post["tool_name"])
	}
}

func eventNames(events []map[string]any) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		if ev, _ := e["event"].(string); ev != "" {
			out = append(out, ev)
		}
	}
	return out
}
