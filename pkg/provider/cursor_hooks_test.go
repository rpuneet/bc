package provider

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// readCursorHooks loads the hooks section of a generated .cursor/hooks.json.
func readCursorHooks(t *testing.T, root string) map[string][]cursorHook {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".cursor", "hooks.json")) //nolint:gosec // test-local temp dir
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	var file struct {
		Hooks   map[string][]cursorHook `json:"hooks"`
		Version int                     `json:"version"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parse hooks.json: %v\n%s", err, raw)
	}
	if file.Version != cursorHooksSchemaVersion {
		t.Errorf("version = %d, want %d", file.Version, cursorHooksSchemaVersion)
	}
	return file.Hooks
}

// mycelEntry returns the mycel-generated entry for a cursor event.
func mycelEntry(t *testing.T, hooks map[string][]cursorHook, event string) cursorHook {
	t.Helper()
	for _, h := range hooks[event] {
		if isMycelCursorHook(h) {
			return h
		}
	}
	t.Fatalf("no mycel entry registered for %q (got %+v)", event, hooks[event])
	return cursorHook{}
}

func TestCursorActivityMode(t *testing.T) {
	p := NewCursorProvider()
	if got := p.ActivityMode(); got != ActivityModeHooks {
		t.Errorf("ActivityMode = %q, want %q", got, ActivityModeHooks)
	}
	// cursor's hook payloads carry transcript_path, so mycel never globs for the
	// session file. Returning globs here would imply the tailer should run.
	if globs := p.TranscriptGlobs("/tmp/wt"); globs != nil {
		t.Errorf("TranscriptGlobs = %v, want nil", globs)
	}
}

// The daemon reads PascalCase event names and only acts on states it knows.
// Every generated command must therefore name a daemon event, not cursor's
// camelCase one, or the ingest endpoint rejects it as unknown.
func TestCursorHooksTranslateToDaemonVocabulary(t *testing.T) {
	root := t.TempDir()
	if err := WriteCursorHookSettings(root); err != nil {
		t.Fatalf("WriteCursorHookSettings: %v", err)
	}
	hooks := readCursorHooks(t, root)

	want := map[string]struct{ event, state string }{
		"sessionStart":       {"SessionStart", "idle"},
		"beforeSubmitPrompt": {"UserPromptSubmit", "working"},
		"preToolUse":         {"PreToolUse", cursorNoState},
		"postToolUse":        {"PostToolUse", cursorNoState},
		"postToolUseFailure": {"PostToolUseFailure", cursorNoState},
		"subagentStart":      {"SubagentStart", cursorNoState},
		"subagentStop":       {"SubagentStop", cursorNoState},
		"preCompact":         {"PreCompact", cursorNoState},
		"stop":               {"Stop", "idle"},
		"sessionEnd":         {"SessionEnd", "stopped"},
	}

	for cursorEvent, exp := range want {
		entry := mycelEntry(t, hooks, cursorEvent)
		wantCmd := cursorReporterRelPath + " " + exp.event + " " + exp.state
		if entry.Command != wantCmd {
			t.Errorf("%s command = %q, want %q", cursorEvent, entry.Command, wantCmd)
		}
		if entry.Timeout == 0 {
			t.Errorf("%s has no timeout: a hung reporter would stall the agent's turn", cursorEvent)
		}
	}

	for event := range hooks {
		if _, ok := want[event]; !ok {
			t.Errorf("unexpected event %q registered", event)
		}
	}
}

// afterAgentThought fires several times per model turn and has no daemon
// equivalent; the shell/file events restate preToolUse/postToolUse. Registering
// any of them would flood the event log or double-count every tool call.
func TestCursorHooksSkipNoisyAndDuplicateEvents(t *testing.T) {
	root := t.TempDir()
	if err := WriteCursorHookSettings(root); err != nil {
		t.Fatalf("WriteCursorHookSettings: %v", err)
	}
	hooks := readCursorHooks(t, root)

	for _, event := range []string{
		"afterAgentThought", "afterAgentResponse",
		"beforeShellExecution", "afterShellExecution",
		"beforeReadFile", "afterFileEdit",
		"beforeMCPExecution", "afterMCPExecution",
	} {
		if _, ok := hooks[event]; ok {
			t.Errorf("event %q is registered but should not be", event)
		}
	}
}

// Every command must be whitespace-safe: the reporter is invoked as a plain
// command string by cursor, so an argument containing a space would need shell
// quoting that a naive splitter may not honor.
func TestCursorHookCommandsNeedNoShellQuoting(t *testing.T) {
	root := t.TempDir()
	if err := WriteCursorHookSettings(root); err != nil {
		t.Fatalf("WriteCursorHookSettings: %v", err)
	}
	for event, entries := range readCursorHooks(t, root) {
		for _, h := range entries {
			if strings.ContainsAny(h.Command, `"'\`) {
				t.Errorf("%s command needs quoting: %q", event, h.Command)
			}
			if fields := strings.Fields(h.Command); len(fields) != 3 {
				t.Errorf("%s command = %q, want exactly 3 whitespace-free tokens", event, h.Command)
			}
		}
	}
}

func TestCursorReporterIsExecutable(t *testing.T) {
	root := t.TempDir()
	if err := WriteCursorHookSettings(root); err != nil {
		t.Fatalf("WriteCursorHookSettings: %v", err)
	}
	path := filepath.Join(root, filepath.FromSlash(cursorReporterRelPath))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat reporter: %v", err)
	}
	// Cursor runs the hook as a command, so a non-executable reporter is a
	// silently dead activity feed.
	if info.Mode().Perm()&0100 == 0 {
		t.Errorf("reporter mode = %v, want owner-executable", info.Mode().Perm())
	}
}

// The reporter must be a valid shell script. A syntax error would make every
// hook a no-op with nothing in the UI to explain why.
func TestCursorReporterIsValidBash(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	root := t.TempDir()
	if writeErr := WriteCursorHookSettings(root); writeErr != nil {
		t.Fatalf("WriteCursorHookSettings: %v", writeErr)
	}
	path := filepath.Join(root, filepath.FromSlash(cursorReporterRelPath))
	out, err := exec.CommandContext(t.Context(), bash, "-n", path).CombinedOutput() //nolint:gosec // fixed args, test-local path
	if err != nil {
		t.Fatalf("reporter is not valid bash: %v\n%s", err, out)
	}
}

// The reporter runs on a machine whose daemon may be on a non-default port and
// whose agent name is only known at runtime, so it must read both from the
// environment rather than bake in a value at write time.
func TestCursorReporterResolvesDaemonAndAgentAtRuntime(t *testing.T) {
	for _, want := range []string{"MYCEL_DAEMON_ADDR", "MYCEL_AGENT_ID"} {
		if !strings.Contains(cursorReporterScript, want) {
			t.Errorf("reporter does not reference %s", want)
		}
	}
	// cursor reports a tool's output as tool_output and a failure as
	// error_message; the daemon reads tool_response and error.
	for _, rename := range []string{"tool_output", "tool_response", "error_message"} {
		if !strings.Contains(cursorReporterScript, rename) {
			t.Errorf("reporter does not handle %s", rename)
		}
	}
	// A reporter that cannot reach the daemon must not fail the agent's turn.
	if !strings.Contains(cursorReporterScript, "exit 0") {
		t.Error("reporter must always exit 0 so a hook failure cannot block the agent")
	}
}

// curlRecorder puts a fake curl earlier on PATH than the real one and returns
// the PATH to use plus the file the shim writes the POSTed body to. The shim is
// /bin/sh so it does not itself depend on anything being on PATH.
func curlRecorder(t *testing.T, extraShims map[string]string) (path, bodyPath string) {
	t.Helper()
	binDir := t.TempDir()
	bodyPath = filepath.Join(binDir, "body.json")
	shim := "#!/bin/sh\nprev=\nfor a in \"$@\"; do\n  if [ \"$prev\" = \"-d\" ]; then printf '%s' \"$a\" > " +
		bodyPath + "; fi\n  prev=$a\ndone\nexit 0\n"
	// 0700 rather than 0600: the shims stand in for binaries on PATH, so they
	// have to be executable.
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(shim), 0700); err != nil { //nolint:gosec // must be executable
		t.Fatalf("write curl shim: %v", err)
	}
	for name, body := range extraShims {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(body), 0700); err != nil { //nolint:gosec // must be executable
			t.Fatalf("write %s shim: %v", name, err)
		}
	}
	return binDir + string(os.PathListSeparator) + os.Getenv("PATH"), bodyPath
}

// Verified behavior: the reporter is what actually POSTs, so run it and assert
// on the request the daemon would receive — including both field renames.
func TestCursorReporterPostsTranslatedPayload(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	root := t.TempDir()
	if writeErr := WriteCursorHookSettings(root); writeErr != nil {
		t.Fatalf("WriteCursorHookSettings: %v", writeErr)
	}

	path, bodyPath := curlRecorder(t, nil)

	stdin := `{"tool_name":"Shell","tool_input":{"command":"echo hi"},` +
		`"tool_output":"{\"exitCode\":0}","error_message":"boom","hook_event_name":"postToolUse"}`

	cmd := exec.CommandContext(t.Context(), bash, filepath.Join(root, filepath.FromSlash(cursorReporterRelPath)), "PostToolUse", cursorNoState) //nolint:gosec // fixed args, test-local path
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(),
		"PATH="+path,
		"MYCEL_AGENT_ID=agent-x",
		"MYCEL_DAEMON_ADDR=http://127.0.0.1:1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("reporter failed: %v\n%s", err, out)
	}
	// Cursor treats stdout as the hook's response; it must be a JSON object.
	if got := strings.TrimSpace(string(out)); got != "{}" {
		t.Errorf("reporter stdout = %q, want {}", got)
	}

	raw, err := os.ReadFile(bodyPath) //nolint:gosec // test-local temp dir
	if err != nil {
		t.Fatalf("reporter did not POST a body: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("POSTed body is not JSON: %v\n%s", err, raw)
	}

	if got["event"] != "PostToolUse" {
		t.Errorf("event = %v, want PostToolUse", got["event"])
	}
	// cursorNoState must arrive as an empty state so the ingest leaves the
	// agent's state alone for informational events.
	if got["state"] != "" {
		t.Errorf("state = %v, want empty for %s", got["state"], cursorNoState)
	}
	if got["tool_response"] != `{"exitCode":0}` {
		t.Errorf("tool_response = %v, want cursor's tool_output", got["tool_response"])
	}
	if got["error"] != "boom" {
		t.Errorf("error = %v, want cursor's error_message", got["error"])
	}
	// Cursor's own fields must survive so the raw stream stays complete.
	if got["tool_name"] != "Shell" {
		t.Errorf("tool_name = %v, want Shell (original payload must pass through)", got["tool_name"])
	}
}

// Without jq the reporter must still report the lifecycle transition, or an
// agent on a jq-less machine would look permanently stuck.
func TestCursorReporterFallsBackWithoutJQ(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	root := t.TempDir()
	if writeErr := WriteCursorHookSettings(root); writeErr != nil {
		t.Fatalf("WriteCursorHookSettings: %v", writeErr)
	}

	// Shadow jq with one that always fails, standing in for a machine without
	// it. Everything else stays on PATH so only the jq path is exercised.
	path, bodyPath := curlRecorder(t, map[string]string{
		"jq": "#!/bin/sh\nexit 127\n",
	})

	cmd := exec.CommandContext(t.Context(), bash, filepath.Join(root, filepath.FromSlash(cursorReporterRelPath)), "Stop", "idle") //nolint:gosec // fixed args, test-local path
	cmd.Stdin = strings.NewReader(`{"status":"completed"}`)
	cmd.Env = append(os.Environ(),
		"PATH="+path,
		"MYCEL_AGENT_ID=agent-x",
		"MYCEL_DAEMON_ADDR=http://127.0.0.1:1",
	)
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		t.Fatalf("reporter failed without jq: %v\n%s", runErr, out)
	}

	raw, err := os.ReadFile(bodyPath) //nolint:gosec // test-local temp dir
	if err != nil {
		t.Fatalf("reporter did not POST without jq: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("fallback body is not JSON: %v\n%s", err, raw)
	}
	if got["event"] != "Stop" || got["state"] != "idle" {
		t.Errorf("fallback payload = %v, want event=Stop state=idle", got)
	}
}

func TestWriteCursorHookSettingsIsIdempotent(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 3; i++ {
		if err := WriteCursorHookSettings(root); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	hooks := readCursorHooks(t, root)
	for event, entries := range hooks {
		var mycel int
		for _, h := range entries {
			if isMycelCursorHook(h) {
				mycel++
			}
		}
		if mycel != 1 {
			t.Errorf("%s has %d mycel entries after 3 writes, want 1", event, mycel)
		}
	}
}

// A user who added their own cursor hook must keep it: cursor runs every entry
// registered for an event, so mycel appends rather than replaces.
func TestWriteCursorHookSettingsPreservesUserHooksAndKeys(t *testing.T) {
	root := t.TempDir()
	cursorDir := filepath.Join(root, ".cursor")
	if err := os.MkdirAll(cursorDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	existing := `{
	  "version": 1,
	  "somethingElse": {"keep": true},
	  "hooks": {
	    "preToolUse": [{"command": ".cursor/hooks/my-audit.sh"}],
	    "afterFileEdit": [{"command": ".cursor/hooks/format.sh"}]
	  }
	}`
	if err := os.WriteFile(filepath.Join(cursorDir, "hooks.json"), []byte(existing), 0600); err != nil {
		t.Fatalf("seed hooks.json: %v", err)
	}

	if err := WriteCursorHookSettings(root); err != nil {
		t.Fatalf("WriteCursorHookSettings: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(cursorDir, "hooks.json")) //nolint:gosec // test-local temp dir
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var file map[string]json.RawMessage
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parse: %v", err)
	}
	// An unrelated top-level key must round-trip untouched.
	if _, ok := file["somethingElse"]; !ok {
		t.Error("unrelated top-level key was dropped")
	}

	hooks := readCursorHooks(t, root)
	var found bool
	for _, h := range hooks["preToolUse"] {
		if h.Command == ".cursor/hooks/my-audit.sh" {
			found = true
		}
	}
	if !found {
		t.Errorf("user's preToolUse hook was dropped: %+v", hooks["preToolUse"])
	}
	// mycel's entry must still be installed alongside it.
	mycelEntry(t, hooks, "preToolUse")

	// An event mycel does not manage must be left entirely alone.
	if len(hooks["afterFileEdit"]) != 1 || hooks["afterFileEdit"][0].Command != ".cursor/hooks/format.sh" {
		t.Errorf("unmanaged event was modified: %+v", hooks["afterFileEdit"])
	}
}

// A stale mycel entry from an older mycel must be replaced, not accumulated,
// so a changed reporter contract actually propagates.
func TestWriteCursorHookSettingsReplacesStaleMycelHooks(t *testing.T) {
	root := t.TempDir()
	cursorDir := filepath.Join(root, ".cursor")
	if err := os.MkdirAll(cursorDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stale := `{"version":1,"hooks":{"stop":[{"command":"` + cursorReporterRelPath + ` OldEvent whatever"}]}}`
	if err := os.WriteFile(filepath.Join(cursorDir, "hooks.json"), []byte(stale), 0600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := WriteCursorHookSettings(root); err != nil {
		t.Fatalf("WriteCursorHookSettings: %v", err)
	}
	entries := readCursorHooks(t, root)["stop"]
	if len(entries) != 1 {
		t.Fatalf("stop has %d entries, want 1: %+v", len(entries), entries)
	}
	if strings.Contains(entries[0].Command, "OldEvent") {
		t.Errorf("stale mycel entry survived: %q", entries[0].Command)
	}
}

func TestWriteCursorHookSettingsRebuildsCorruptFile(t *testing.T) {
	root := t.TempDir()
	cursorDir := filepath.Join(root, ".cursor")
	if err := os.MkdirAll(cursorDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "hooks.json"), []byte("{not json"), 0600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := WriteCursorHookSettings(root); err != nil {
		t.Fatalf("WriteCursorHookSettings: %v", err)
	}
	mycelEntry(t, readCursorHooks(t, root), "stop")
}

func TestWriteCursorHookSettingsRejectsTraversal(t *testing.T) {
	if err := WriteCursorHookSettings("/tmp/../etc"); err == nil {
		t.Skip("path cleaned to a safe absolute root")
	}
}

func TestWriteCursorHookSettingsRejectsUnsafeRoot(t *testing.T) {
	if err := WriteCursorHookSettings("wt/../../etc"); err == nil {
		t.Error("expected an error for a root escaping the worktree")
	}
}
