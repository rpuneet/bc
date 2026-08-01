package provider

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mtime returns a deterministic, strictly increasing timestamp for ordering
// test fixture files by modification time.
func mtime(n int) time.Time {
	return time.Date(2026, 5, 1, 0, 0, n, 0, time.UTC)
}

func codexProvider(t *testing.T) *CodexProvider {
	t.Helper()
	p, ok := DefaultRegistry.Get("codex")
	if !ok {
		t.Fatal("codex provider not registered")
	}
	cp, ok := p.(*CodexProvider)
	if !ok {
		t.Fatalf("registered codex provider is %T, want *CodexProvider", p)
	}
	return cp
}

func TestCodexActivityMode(t *testing.T) {
	p := codexProvider(t)
	if p.ActivityMode() != ActivityModeTranscript {
		t.Errorf("ActivityMode = %q, want %q", p.ActivityMode(), ActivityModeTranscript)
	}
	if globs := p.TranscriptGlobs("/x"); globs != nil {
		t.Errorf("TranscriptGlobs = %v, want nil (codex uses SelectTranscript)", globs)
	}
	if err := p.WriteHookConfig("/wt", "http://d", "a"); err != nil {
		t.Errorf("WriteHookConfig = %v, want nil", err)
	}
}

// parse is a helper that runs a session over lines and returns all activities.
func parseCodex(t *testing.T, lines ...string) []TranscriptActivity {
	t.Helper()
	sess := codexProvider(t).NewTranscriptSession()
	out := make([]TranscriptActivity, 0, len(lines))
	for _, l := range lines {
		acts, err := sess.ParseLine([]byte(l))
		if err != nil {
			t.Fatalf("ParseLine(%q) error: %v", l, err)
		}
		out = append(out, acts...)
	}
	return out
}

// TestCodexParsePairing feeds real-shape codex rollout lines (a shell tool call
// and its output, then a failing one) and asserts correctly-paired Pre/Post with
// the tool name carried across from the call to the output line.
func TestCodexParsePairing(t *testing.T) {
	lines := []string{
		`{"timestamp":"2026-05-02T08:29:51.690Z","type":"session_meta","payload":{"cwd":"/wt/eng","originator":"codex-tui"}}`,
		`{"timestamp":"2026-05-02T08:30:00.000Z","type":"event_msg","payload":{"type":"user_message","message":"list files"}}`,
		`{"timestamp":"2026-05-02T08:30:01.000Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"ls\",\"workdir\":\"/wt/eng\"}","call_id":"call_A"}}`,
		`{"timestamp":"2026-05-02T08:30:02.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_A","output":"Exit code: 0\nWall time: 0.1 seconds\nOutput:\nREADME.md\n"}}`,
		`{"timestamp":"2026-05-02T08:30:03.000Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"go test\"}","call_id":"call_B"}}`,
		`{"timestamp":"2026-05-02T08:30:09.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_B","output":"Exit code: 1\nWall time: 6s\nOutput:\nFAIL\n"}}`,
		`{"timestamp":"2026-05-02T08:30:10.000Z","type":"event_msg","payload":{"type":"task_complete"}}`,
	}
	acts := parseCodex(t, lines...)

	wantEvents := []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PreToolUse", "PostToolUseFailure", "Stop"}
	if len(acts) != len(wantEvents) {
		t.Fatalf("got %d activities %v, want %d", len(acts), eventList(acts), len(wantEvents))
	}
	for i, want := range wantEvents {
		if acts[i].Event != want {
			t.Errorf("activity[%d].Event = %q, want %q", i, acts[i].Event, want)
		}
	}

	if acts[1].Prompt != "list files" {
		t.Errorf("UserPromptSubmit prompt = %q", acts[1].Prompt)
	}
	// Pre carries decoded arguments as a structured object.
	if acts[2].ToolName != "shell_command" {
		t.Errorf("PreToolUse tool = %q, want shell_command", acts[2].ToolName)
	}
	if m, ok := acts[2].ToolInput.(map[string]any); !ok || m["command"] != "ls" {
		t.Errorf("PreToolUse input = %#v, want decoded map with command=ls", acts[2].ToolInput)
	}
	// Post pairs with its call by id and carries the tool name from the call.
	if acts[3].ToolName != "shell_command" {
		t.Errorf("PostToolUse tool = %q, want shell_command (paired via call_id)", acts[3].ToolName)
	}
	// Non-zero exit → failure.
	if acts[5].ToolName != "shell_command" || acts[5].Error == "" {
		t.Errorf("PostToolUseFailure = %#v, want shell_command with error", acts[5])
	}
}

// TestCodexCustomToolExit covers the custom-tool (apply_patch) output shape whose
// exit code lives in a JSON metadata object.
func TestCodexCustomToolExit(t *testing.T) {
	ok := parseCodex(t,
		`{"type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","input":"*** Begin Patch","call_id":"c1"}}`,
		`{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"c1","output":"{\"output\":\"Success\",\"metadata\":{\"exit_code\":0}}"}}`,
	)
	if len(ok) != 2 || ok[0].Event != "PreToolUse" || ok[1].Event != "PostToolUse" {
		t.Fatalf("ok case = %v, want Pre+PostToolUse", eventList(ok))
	}
	if ok[1].ToolResponse != "Success" {
		t.Errorf("PostToolUse response = %q, want inner output 'Success'", ok[1].ToolResponse)
	}

	bad := parseCodex(t,
		`{"type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","input":"*** Begin Patch","call_id":"c2"}}`,
		`{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"c2","output":"{\"output\":\"boom\",\"metadata\":{\"exit_code\":1}}"}}`,
	)
	if len(bad) != 2 || bad[1].Event != "PostToolUseFailure" {
		t.Fatalf("bad case = %v, want Pre+PostToolUseFailure", eventList(bad))
	}
}

// TestCodexOrphanOutputSkipped verifies that a tool output whose originating
// call was never seen (e.g. it predates the tailer's seed at EOF) is skipped
// rather than emitted as an unpaired node.
func TestCodexOrphanOutputSkipped(t *testing.T) {
	acts := parseCodex(t,
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"unknown","output":"Exit code: 0\n"}}`,
	)
	if len(acts) != 0 {
		t.Fatalf("orphan output produced %v, want none", eventList(acts))
	}
}

// TestCodexSessionStartOnce ensures a forked session's second session_meta record
// does not double-fire SessionStart.
func TestCodexSessionStartOnce(t *testing.T) {
	acts := parseCodex(t,
		`{"type":"session_meta","payload":{"cwd":"/wt"}}`,
		`{"type":"session_meta","payload":{"cwd":"/wt"}}`,
	)
	if len(acts) != 1 || acts[0].Event != "SessionStart" {
		t.Fatalf("got %v, want single SessionStart", eventList(acts))
	}
}

// TestCodexSelectTranscript writes real-shape rollout files under a temp
// CODEX_HOME and asserts SelectTranscript matches on the session_meta cwd and
// returns the newest matching rollout.
func TestCodexSelectTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)

	mkRollout := func(day, id, cwd string) string {
		dir := filepath.Join(home, "sessions", "2026", "05", day)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "rollout-2026-05-"+day+"T00-00-00-"+id+".jsonl")
		body := `{"timestamp":"2026-05-` + day + `T00:00:00Z","type":"session_meta","payload":{"cwd":"` + cwd + `","originator":"codex-tui"}}` + "\n" +
			`{"type":"event_msg","payload":{"type":"user_message","message":"hi"}}` + "\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	other := mkRollout("01", "aaaa", "/wt/other")
	oldMatch := mkRollout("02", "bbbb", "/wt/eng")
	newMatch := mkRollout("03", "cccc", "/wt/eng")

	// Order mtimes: other (oldest) < oldMatch < newMatch (newest).
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.Chtimes(other, mtime(1), mtime(1)))
	must(os.Chtimes(oldMatch, mtime(2), mtime(2)))
	must(os.Chtimes(newMatch, mtime(3), mtime(3)))

	p := codexProvider(t)
	if got := p.SelectTranscript("/wt/eng"); got != newMatch {
		t.Errorf("SelectTranscript(/wt/eng) = %q, want newest match %q", got, newMatch)
	}
	if got := p.SelectTranscript("/wt/nope"); got != "" {
		t.Errorf("SelectTranscript(/wt/nope) = %q, want empty (no cwd match)", got)
	}
	if got := p.SelectTranscript(""); got != "" {
		t.Errorf("SelectTranscript(\"\") = %q, want empty", got)
	}
}

func eventList(acts []TranscriptActivity) []string {
	out := make([]string, len(acts))
	for i := range acts {
		out[i] = acts[i].Event
	}
	return out
}
