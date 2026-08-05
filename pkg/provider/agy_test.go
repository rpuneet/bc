package provider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAgyProviderIdentity(t *testing.T) {
	p := NewAgyProvider()
	if p.Name() != "agy" {
		t.Errorf("Name() = %q, want agy", p.Name())
	}
	if p.Binary() != "agy" {
		t.Errorf("Binary() = %q, want agy", p.Binary())
	}
	if p.Command() != "agy --dangerously-skip-permissions" {
		t.Errorf("Command() = %q", p.Command())
	}
	if p.Description() == "" {
		t.Error("expected non-empty description")
	}
	if p.InstallHint() == "" {
		t.Error("expected non-empty install hint")
	}
}

// TestAgyInterfaces asserts agy reaches Claude-level integration depth.
func TestAgyInterfaces(t *testing.T) {
	var p Provider = NewAgyProvider()
	if _, ok := p.(ModelLister); !ok {
		t.Error("agy must implement ModelLister")
	}
	if _, ok := p.(DynamicModelLister); !ok {
		t.Error("agy must implement DynamicModelLister")
	}
	if _, ok := p.(ContainerCustomizer); !ok {
		t.Error("agy must implement ContainerCustomizer")
	}
	if _, ok := p.(SessionCustomizer); !ok {
		t.Error("agy must implement SessionCustomizer")
	}
	if _, ok := p.(SessionResumer); !ok {
		t.Error("agy must implement SessionResumer")
	}
	if _, ok := p.(ResumableSessionDetector); !ok {
		t.Error("agy must implement ResumableSessionDetector")
	}
	if _, ok := p.(ActivitySource); !ok {
		t.Error("agy must implement ActivitySource")
	}
	if _, ok := p.(ConfigAdapter); !ok {
		t.Error("agy must implement ConfigAdapter")
	}
}

func TestAgyBuildCommand(t *testing.T) {
	p := NewAgyProvider()
	tests := []struct { //nolint:govet // test struct, field order matches literal values
		name string
		want string
		opts CommandOpts
	}{
		{
			name: "no opts — base command",
			opts: CommandOpts{},
			want: "agy --dangerously-skip-permissions",
		},
		{
			name: "spaced+paren model is single-quoted",
			opts: CommandOpts{Model: "Gemini 3.5 Flash (High)"},
			want: "agy --dangerously-skip-permissions --model 'Gemini 3.5 Flash (High)'",
		},
		{
			name: "default-style model",
			opts: CommandOpts{Model: "Gemini 3 Flash"},
			want: "agy --dangerously-skip-permissions --model 'Gemini 3 Flash'",
		},
		{
			name: "conversation ID resumes by UUID",
			opts: CommandOpts{SessionID: "f3c78084-630c-473d-8842-17f12ccdd971"},
			want: "agy --dangerously-skip-permissions --conversation f3c78084-630c-473d-8842-17f12ccdd971",
		},
		{
			name: "conversation ID takes priority over resume flag",
			opts: CommandOpts{SessionID: "f3c78084-630c-473d-8842-17f12ccdd971", Resume: true},
			want: "agy --dangerously-skip-permissions --conversation f3c78084-630c-473d-8842-17f12ccdd971",
		},
		{
			name: "resume flag alone — continue",
			opts: CommandOpts{Resume: true},
			want: "agy --dangerously-skip-permissions --continue",
		},
		{
			name: "model + resume combine",
			opts: CommandOpts{Model: "Gemini 3.1 Pro (High)", Resume: true},
			want: "agy --dangerously-skip-permissions --model 'Gemini 3.1 Pro (High)' --continue",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.BuildCommand(tt.opts); got != tt.want {
				t.Errorf("BuildCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAgyBuildCommandInjectionSafe verifies shell-breaking model values are
// dropped, and that a value with an embedded single quote (which cannot pass
// the validator anyway) can never break out of the surrounding quotes.
func TestAgyBuildCommandInjectionSafe(t *testing.T) {
	p := NewAgyProvider()
	unsafe := []string{
		"$(rm -rf /)",
		"Gemini`id`",
		`Gemini";id;"`,
		"Gemini'; id; '",
		"Gemini$IFS",
		"-flag-injection",
		"Gemini|whoami",
		"Gemini;whoami",
		"Gemini&whoami",
		"Gemini>out",
		"",
	}
	for _, m := range unsafe {
		t.Run(m, func(t *testing.T) {
			cmd := p.BuildCommand(CommandOpts{Model: m})
			if strings.Contains(cmd, "--model") {
				t.Errorf("unsafe model %q must be dropped, got %q", m, cmd)
			}
			// Base command must remain intact regardless of the input.
			if cmd != "agy --dangerously-skip-permissions" {
				t.Errorf("unexpected command for unsafe model %q: %q", m, cmd)
			}
		})
	}
}

func TestSafeAgyModelName(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"Gemini 3 Flash", true},
		{"Gemini 3.5 Flash (High)", true},
		{"Gemini 3.1 Pro (Low)", true},
		{"gemini-2.5-pro", true},
		{"", false},
		{"-leading-dash", false},
		{"$(id)", false},
		{"a`b`", false},
		{`a"b`, false},
		{"a'b", false},
		{"a;b", false},
		{"a|b", false},
		{"a$b", false},
	}
	for _, tt := range tests {
		if got := SafeAgyModelName(tt.model); got != tt.want {
			t.Errorf("SafeAgyModelName(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestAgyModels(t *testing.T) {
	p := NewAgyProvider()
	models := p.Models()
	if len(models) != 6 {
		t.Fatalf("expected 6 static models, got %d: %v", len(models), models)
	}
	// Every static model must be usable by BuildCommand's injection gate.
	for _, m := range models {
		if !SafeAgyModelName(m) {
			t.Errorf("static model %q fails SafeAgyModelName", m)
		}
	}
	// The default model must be in the curated list.
	found := false
	for _, m := range models {
		if m == agyDefaultModel {
			found = true
		}
	}
	if !found {
		t.Errorf("default model %q not in static list", agyDefaultModel)
	}
}

func TestAgyListModels(t *testing.T) {
	p := NewAgyProvider()
	orig := agyListModels
	t.Cleanup(func() { agyListModels = orig })

	// Live parse: agy models output, with a blank line to be skipped.
	agyListModels = func(_ context.Context) (string, error) {
		return "Gemini 3.5 Flash (High)\n\nGemini 3 Flash\n", nil
	}
	got, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Gemini 3.5 Flash (High)", "Gemini 3 Flash"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("ListModels() = %v, want %v", got, want)
	}

	// CLI error → static fallback.
	agyListModels = func(_ context.Context) (string, error) {
		return "", errors.New("agy not installed")
	}
	got, err = p.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(p.Models()) {
		t.Errorf("fallback ListModels() = %v, want static %v", got, p.Models())
	}

	// Only-unsafe output → static fallback.
	agyListModels = func(_ context.Context) (string, error) {
		return "$(danger)\n", nil
	}
	got, _ = p.ListModels(context.Background())
	if len(got) != len(p.Models()) {
		t.Errorf("unsafe-only ListModels() = %v, want static fallback", got)
	}
}

func TestAgyParseSessionID(t *testing.T) {
	p := NewAgyProvider()
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"resume hint", "Resume with: agy --conversation f3c78084-630c-473d-8842-17f12ccdd971", "f3c78084-630c-473d-8842-17f12ccdd971"},
		{"no hint", "just some output", ""},
		{"malformed uuid", "agy --conversation not-a-uuid", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.ParseSessionID(tt.output); got != tt.want {
				t.Errorf("ParseSessionID() = %q, want %q", got, tt.want)
			}
		})
	}
	if !p.SupportsResume() {
		t.Error("SupportsResume() must be true")
	}
}

// TestAgyHasResumableSession covers the --continue gate: agy has nothing to
// continue when no conversation database exists.
func TestAgyHasResumableSession(t *testing.T) {
	p := NewAgyProvider()
	home := t.TempDir()
	orig := agyHomeDir
	agyHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { agyHomeDir = orig })

	if p.HasResumableSession("/any/worktree") {
		t.Error("no conversations dir — must report false")
	}

	convDir := agyConversationsDir(home)
	if err := os.MkdirAll(convDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if p.HasResumableSession("/any/worktree") {
		t.Error("empty conversations dir — must report false")
	}

	if err := os.WriteFile(filepath.Join(convDir, "f3c78084-630c-473d-8842-17f12ccdd971.db"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !p.HasResumableSession("/any/worktree") {
		t.Error("conversation db present — must report true")
	}
}

func TestAgyActivitySource(t *testing.T) {
	p := NewAgyProvider()
	if p.ActivityMode() != ActivityModeTranscript {
		t.Errorf("ActivityMode() = %q, want %q", p.ActivityMode(), ActivityModeTranscript)
	}
	globs := p.TranscriptGlobs("/wt/eng-01")
	if len(globs) != 1 || !strings.Contains(globs[0], filepath.Join("antigravity-cli", "brain")) {
		t.Errorf("TranscriptGlobs() = %v", globs)
	}
	if p.TranscriptGlobs("") != nil {
		t.Error("empty cwd must yield nil globs")
	}
}

// TestWriteAgyHookSettings verifies hooks.json is well-formed agy schema with
// all five lifecycle events and injection-free commands.
func TestWriteAgyHookSettings(t *testing.T) {
	wt := t.TempDir()
	if err := WriteAgyHookSettings(wt); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(wt, ".agents", "hooks.json")) //nolint:gosec // test reads a path under t.TempDir()
	if err != nil {
		t.Fatal(err)
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(raw, &hooks); err != nil {
		t.Fatalf("hooks.json is not valid JSON: %v", err)
	}
	entry, ok := hooks["mycel-activity"]
	if !ok {
		t.Fatal("missing mycel-activity hook entry")
	}
	var spec struct {
		PreToolUse     []json.RawMessage `json:"PreToolUse"`
		PostToolUse    []json.RawMessage `json:"PostToolUse"`
		PreInvocation  []json.RawMessage `json:"PreInvocation"`
		PostInvocation []json.RawMessage `json:"PostInvocation"`
		Stop           []json.RawMessage `json:"Stop"`
	}
	if err := json.Unmarshal(entry, &spec); err != nil {
		t.Fatal(err)
	}
	if len(spec.PreToolUse) == 0 || len(spec.PostToolUse) == 0 ||
		len(spec.PreInvocation) == 0 || len(spec.PostInvocation) == 0 || len(spec.Stop) == 0 {
		t.Errorf("expected all five lifecycle events populated: %s", entry)
	}
	// Hook commands must reference the daemon hook endpoint.
	if !strings.Contains(string(entry), "/api/agents/${MYCEL_AGENT_ID}/hook") {
		t.Error("hook command must POST to daemon hook endpoint")
	}

	// Merge preserves a user-defined hook and refreshes the mycel entry.
	if err := os.WriteFile(filepath.Join(wt, ".agents", "hooks.json"),
		[]byte(`{"user-lint":{"Stop":[{"type":"command","command":"echo hi"}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgyHookSettings(wt); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(wt, ".agents", "hooks.json")) //nolint:gosec // test reads a path under t.TempDir()
	if err := json.Unmarshal(raw, &hooks); err != nil {
		t.Fatal(err)
	}
	if _, ok := hooks["user-lint"]; !ok {
		t.Error("merge must preserve user-defined hooks")
	}
	if _, ok := hooks["mycel-activity"]; !ok {
		t.Error("merge must keep the mycel-activity hook")
	}
}

func TestAgyConfigAdapter(t *testing.T) {
	a := &AgyConfigAdapter{}
	if a.PromptFile() != "AGENTS.md" {
		t.Errorf("PromptFile() = %q, want AGENTS.md", a.PromptFile())
	}
	if a.ConfigDir() != ".agents" {
		t.Errorf("ConfigDir() = %q, want .agents", a.ConfigDir())
	}
	if !a.SupportsRules() || !a.SupportsSkills() {
		t.Error("agy adapter should support rules and skills")
	}

	// SetupMCP writes agy-native mcp_config.json (serverUrl for SSE).
	dir := t.TempDir()
	err := a.SetupMCP(context.Background(), dir, "eng-01", map[string]MCPEntry{
		"mycel":  {URL: "http://127.0.0.1:9374/mcp/sse", Transport: "sse"},
		"github": {Command: "github-mcp-server", Args: []string{"--stdio"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "mcp_config.json")) //nolint:gosec // test reads a path under t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.MCPServers["mycel"]["serverUrl"]; !ok {
		t.Error("SSE server must use serverUrl key")
	}
	if cfg.MCPServers["github"]["command"] != "github-mcp-server" {
		t.Error("stdio server must use command key")
	}
}

func TestAgyContainerCommand(t *testing.T) {
	p := NewAgyProvider()
	base := p.BuildCommand(CommandOpts{Model: "Gemini 3.5 Flash (High)"})
	wrapped := p.AdjustContainerCommand(base)
	if !strings.Contains(wrapped, "tmux new-session") {
		t.Errorf("container command must wrap in tmux: %q", wrapped)
	}
	if !strings.Contains(wrapped, "'Gemini 3.5 Flash (High)'") {
		t.Errorf("container command must preserve single-quoted model: %q", wrapped)
	}
	if p.AdjustSessionCommand(base) != base {
		t.Error("AdjustSessionCommand must be a no-op for native tmux")
	}
	if p.DockerImage() != "" {
		t.Errorf("DockerImage() = %q, want empty", p.DockerImage())
	}
}

// agyCurlRecorder builds a PATH in which curl is shadowed by a shim that writes
// the payload it was handed to a file, so a generated hook command can be run
// for real and its POST body inspected. The rest of the system PATH is kept so
// bash, cat and jq resolve normally.
func agyCurlRecorder(t *testing.T) (pathEnv, payloadFile string) {
	t.Helper()
	shimDir := t.TempDir()
	payloadFile = filepath.Join(t.TempDir(), "payload.json")
	shim := "#!/usr/bin/env bash\n" +
		"# Record the value that followed -d, which is the POST body.\n" +
		"prev=\"\"\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"-d\" ]; then printf '%s' \"$arg\" > " + strconv.Quote(payloadFile) + "; fi\n" +
		"  prev=\"$arg\"\n" +
		"done\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(shimDir, "curl"), []byte(shim), 0700); err != nil { //nolint:gosec // shim must be executable
		t.Fatalf("write curl shim: %v", err)
	}
	return shimDir + string(os.PathListSeparator) + os.Getenv("PATH"), payloadFile
}

// The agy hook must forward the payload agy pipes in on stdin. It used to drain
// stdin to /dev/null and POST only the three fields known at generation time, so
// the prompt and every tool detail agy reported was discarded — leaving agy
// agents with a Live feed of bare event names and no task line.
func TestAgyHookCommandForwardsStdin(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
	pathEnv, payloadFile := agyCurlRecorder(t)

	cmd := exec.CommandContext(t.Context(), "bash", "-c", //nolint:gosec // running the generated command is the point of the test
		agyHookCommand("PreInvocation", "working", "{}"))
	cmd.Env = append(os.Environ(),
		"PATH="+pathEnv,
		"MYCEL_AGENT_ID=agent-x",
		"MYCEL_DAEMON_ADDR=http://127.0.0.1:9374",
	)
	cmd.Stdin = strings.NewReader(`{"prompt":"fix the login bug","tool_name":"Bash"}`)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run hook command: %v", err)
	}

	// agy requires the handler to print its JSON result, whatever else happens.
	if strings.TrimSpace(string(out)) != "{}" {
		t.Errorf("stdout = %q, want %q", out, "{}")
	}

	raw, err := os.ReadFile(payloadFile) //nolint:gosec // test-local temp dir
	if err != nil {
		t.Fatalf("no payload was POSTed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("payload is not valid JSON: %v (%s)", err, raw)
	}
	for field, want := range map[string]string{
		"event":     "PreInvocation",
		"state":     "working",
		"prompt":    "fix the login bug",
		"tool_name": "Bash",
	} {
		if got[field] != want {
			t.Errorf("payload %s = %v, want %q", field, got[field], want)
		}
	}
}

// Without jq the reporter must still POST the bare event so the agent's state
// stays correct, and must still honor agy's stdout contract. Losing the detail
// is acceptable; a stalled turn is not.
func TestAgyHookCommandFallsBackWithoutJQ(t *testing.T) {
	pathEnv, payloadFile := agyCurlRecorder(t)
	// Shadow jq with a shim that always fails, in front of the recorder's PATH.
	jqDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(jqDir, "jq"), []byte("#!/usr/bin/env bash\nexit 1\n"), 0700); err != nil { //nolint:gosec // shim must be executable
		t.Fatalf("write jq shim: %v", err)
	}

	cmd := exec.CommandContext(t.Context(), "bash", "-c", //nolint:gosec // running the generated command is the point of the test
		agyHookCommand("Stop", "idle", "{}"))
	cmd.Env = append(os.Environ(),
		"PATH="+jqDir+string(os.PathListSeparator)+pathEnv,
		"MYCEL_AGENT_ID=agent-x",
	)
	cmd.Stdin = strings.NewReader(`{"prompt":"ignored"}`)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run hook command: %v", err)
	}
	if strings.TrimSpace(string(out)) != "{}" {
		t.Errorf("stdout = %q, want %q", out, "{}")
	}

	raw, err := os.ReadFile(payloadFile) //nolint:gosec // test-local temp dir
	if err != nil {
		t.Fatalf("no payload was POSTed without jq: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("fallback payload is not valid JSON: %v (%s)", err, raw)
	}
	if got["event"] != "Stop" || got["state"] != "idle" {
		t.Errorf("fallback payload = %s, want the bare Stop/idle event", raw)
	}
}

// Every generated agy command must be a runnable shell program. A quoting slip
// would make each hook a silent no-op with nothing in the UI to explain it.
func TestAgyHookCommandsAreValidBash(t *testing.T) {
	for _, tc := range []struct{ event, state, stdout string }{
		{"PreInvocation", "working", "{}"},
		{"PostInvocation", "", "{}"},
		{"Stop", "idle", "{}"},
		{"PreToolUse", "", `{"decision":"allow"}`},
		{"PostToolUse", "", "{}"},
	} {
		t.Run(tc.event, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), "bash", "-n", "-c", //nolint:gosec // syntax-checking the generated command is the point of the test
				agyHookCommand(tc.event, tc.state, tc.stdout))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("bash -n rejected the %s command: %v\n%s", tc.event, err, out)
			}
		})
	}
}

// TestAgyParseTranscriptLine verifies agy's transcript parser correctly
// extracts activity events from JSONL lines.
func TestAgyParseTranscriptLine(t *testing.T) {
	p := NewAgyProvider()

	tests := []struct {
		name    string
		line    string
		want    []TranscriptActivity
		wantErr bool
	}{
		{
			name: "USER_INPUT with prompt",
			line: `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-07-06T21:54:52Z","content":"<USER_REQUEST>\nHello, can you help me with this task?\n</USER_REQUEST>"}`,
			want: []TranscriptActivity{{
				Event:  "UserPromptSubmit",
				Prompt: "Hello, can you help me with this task?",
			}},
		},
		{
			name: "PLANNER_RESPONSE with tool calls",
			line: `{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-07-06T21:55:57Z","thinking":"I'll check the file","tool_calls":[{"name":"Read","args":{"path":"/tmp/test.txt"}}]}`,
			want: []TranscriptActivity{{
				Event:     "PreToolUse",
				ToolName:  "Read",
				ToolInput: map[string]any{"path": "/tmp/test.txt"},
			}},
		},
		{
			name:    "RUN_COMMAND - skipped",
			line:    `{"step_index":7,"source":"MODEL","type":"RUN_COMMAND","status":"DONE","created_at":"2026-07-06T21:56:45Z","content":"exit 0"}`,
			want:    nil,
		},
		{
			name:    "empty line",
			line:    "",
			want:    nil,
		},
		{
			name:    "invalid JSON",
			line:    "not json at all",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.ParseTranscriptLine([]byte(tt.line))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTranscriptLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("ParseTranscriptLine() returned %d activities, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i].Event != tt.want[i].Event {
					t.Errorf("activity[%d].Event = %q, want %q", i, got[i].Event, tt.want[i].Event)
				}
				if got[i].Prompt != tt.want[i].Prompt {
					t.Errorf("activity[%d].Prompt = %q, want %q", i, got[i].Prompt, tt.want[i].Prompt)
				}
				if got[i].ToolName != tt.want[i].ToolName {
					t.Errorf("activity[%d].ToolName = %q, want %q", i, got[i].ToolName, tt.want[i].ToolName)
				}
			}
		})
	}
}
