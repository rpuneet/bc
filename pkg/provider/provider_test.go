package provider

import (
	"context"
	"strings"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(r.providers) != 0 {
		t.Errorf("expected empty registry, got %d providers", len(r.providers))
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	p := NewClaudeProvider()
	r.Register(p)

	got, ok := r.Get("claude")
	if !ok {
		t.Fatal("expected to find registered provider")
	}
	if got.Name() != "claude" {
		t.Errorf("expected name 'claude', got %q", got.Name())
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected not to find unregistered provider")
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(NewClaudeProvider())
	r.Register(NewGeminiProvider())

	list := r.List()
	if len(list) != 2 {
		t.Errorf("expected 2 providers, got %d", len(list))
	}
}

func TestDefaultRegistryHasProviders(t *testing.T) {
	// The default registry holds exactly the built-in providers — nothing
	// more, nothing less. Names() is sorted, so compare directly.
	want := []string{"claude", "codex", "cursor", "gemini", "pi"}
	got := DefaultRegistry.Names()
	if len(got) != len(want) {
		t.Fatalf("expected providers %v, got %v", want, got)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("expected providers %v, got %v", want, got)
		}
	}
}

func TestGetProvider(t *testing.T) {
	p, err := GetProvider("claude")
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}
	if p.Name() != "claude" {
		t.Errorf("expected name 'claude', got %q", p.Name())
	}
}

func TestGetProviderNotFound(t *testing.T) {
	_, err := GetProvider("nonexistent")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestClaudeProvider(t *testing.T) {
	p := NewClaudeProvider()

	if p.Name() != "claude" {
		t.Errorf("expected name 'claude', got %q", p.Name())
	}
	if p.Description() == "" {
		t.Error("expected non-empty description")
	}
	if p.Command() == "" {
		t.Error("expected non-empty command")
	}
}

func TestClaudeDetectState(t *testing.T) {
	p := NewClaudeProvider()

	tests := []struct {
		name   string
		output string
		want   State
	}{
		{
			name:   "working spinner 1",
			output: "✻ Reading file...\n",
			want:   StateWorking,
		},
		{
			name:   "working spinner 2",
			output: "✳ Analyzing code\n",
			want:   StateWorking,
		},
		{
			name:   "working tool",
			output: "⏺ Running bash command\n",
			want:   StateWorking,
		},
		{
			name:   "idle prompt",
			output: "❯ ",
			want:   StateIdle,
		},
		{
			name:   "unknown",
			output: "some output\n",
			want:   StateUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.DetectState(tt.output)
			if got != tt.want {
				t.Errorf("DetectState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCodexProvider(t *testing.T) {
	p := NewCodexProvider()

	if p.Name() != "codex" {
		t.Errorf("expected name 'codex', got %q", p.Name())
	}
	if p.Description() == "" {
		t.Error("expected non-empty description")
	}
	if p.Command() == "" {
		t.Error("expected non-empty command")
	}
}

func TestCodexDetectState(t *testing.T) {
	p := NewCodexProvider()

	tests := []struct {
		name   string
		output string
		want   State
	}{
		{
			name:   "working spinner",
			output: "⠋ Generating code...\n",
			want:   StateWorking,
		},
		{
			name:   "working thinking",
			output: "thinking about your request\n",
			want:   StateWorking,
		},
		{
			name:   "working executing",
			output: "Executing command...\n",
			want:   StateWorking,
		},
		{
			name:   "done checkmark",
			output: "✓ Code generated\n",
			want:   StateDone,
		},
		{
			name:   "done success",
			output: "Operation completed with success\n",
			want:   StateDone,
		},
		{
			name:   "error",
			output: "Error: API call failed\n",
			want:   StateError,
		},
		{
			name:   "error exception",
			output: "Exception occurred during generation\n",
			want:   StateError,
		},
		{
			name:   "stuck rate limit",
			output: "Rate limit exceeded\n",
			want:   StateStuck,
		},
		{
			name:   "stuck quota",
			output: "Quota exceeded for API\n",
			want:   StateStuck,
		},
		{
			name:   "idle prompt",
			output: "codex> ",
			want:   StateIdle,
		},
		{
			name:   "idle ready",
			output: "Ready for input\n",
			want:   StateIdle,
		},
		{
			name:   "unknown",
			output: "some random output\n",
			want:   StateUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.DetectState(tt.output)
			if got != tt.want {
				t.Errorf("DetectState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListProviders(t *testing.T) {
	providers := ListProviders()
	if len(providers) < 4 {
		t.Errorf("expected at least 4 providers, got %d", len(providers))
	}
}

// TestCheckBinaryExists tests the checkBinaryExists helper function.
func TestCheckBinaryExists(t *testing.T) {
	ctx := context.Background()

	// Test with a binary that definitely exists (sh is on all Unix systems)
	if !checkBinaryExists(ctx, "sh") {
		t.Error("expected sh to exist")
	}

	// Test with a binary that definitely doesn't exist
	if checkBinaryExists(ctx, "definitely-not-a-real-binary-12345") {
		t.Error("expected nonexistent binary to return false")
	}
}

// TestGetBinaryVersion tests the getBinaryVersion helper function.
func TestGetBinaryVersion(t *testing.T) {
	ctx := context.Background()

	// Test with echo command
	version := getBinaryVersion(ctx, "echo", "test-version")
	if version != "test-version" {
		t.Errorf("expected 'test-version', got %q", version)
	}

	// Test with nonexistent binary
	version = getBinaryVersion(ctx, "definitely-not-a-real-binary-12345", "--version")
	if version != "" {
		t.Errorf("expected empty string for nonexistent binary, got %q", version)
	}
}

// TestProviderIsInstalled tests IsInstalled methods across providers.
func TestProviderIsInstalled(t *testing.T) {
	ctx := context.Background()

	// Test each provider's IsInstalled method
	// These will return false unless the actual binaries are installed
	providers := []Provider{
		NewClaudeProvider(),
		NewCodexProvider(),
		NewGeminiProvider(),
		NewCursorProvider(),
	}

	for _, p := range providers {
		t.Run(p.Name(), func(t *testing.T) {
			// Just verify the method doesn't panic and returns a bool
			_ = p.IsInstalled(ctx)
		})
	}
}

// TestProviderVersion tests Version methods across providers.
func TestProviderVersion(t *testing.T) {
	ctx := context.Background()

	// Test each provider's Version method
	providers := []Provider{
		NewClaudeProvider(),
		NewCodexProvider(),
		NewGeminiProvider(),
		NewCursorProvider(),
	}

	for _, p := range providers {
		t.Run(p.Name(), func(t *testing.T) {
			// Just verify the method doesn't panic
			// It will return empty string if not installed
			_ = p.Version(ctx)
		})
	}
}

// TestRegistryListInstalled tests the ListInstalled method.
func TestRegistryListInstalled(t *testing.T) {
	ctx := context.Background()

	// Create a fresh registry
	r := NewRegistry()

	// Register some providers
	r.Register(NewClaudeProvider())
	r.Register(NewGeminiProvider())

	// Test ListInstalled - result depends on what's actually installed
	installed := r.ListInstalled(ctx)

	// Verify the result is a valid slice (may be empty if nothing is installed)
	if installed == nil {
		// nil is valid if nothing is installed - convert to empty slice for consistency
		installed = []Provider{}
	}

	// Each returned provider should be installed
	for _, p := range installed {
		if !p.IsInstalled(ctx) {
			t.Errorf("ListInstalled returned %s but IsInstalled returns false", p.Name())
		}
	}
}

// TestListInstalledProviders tests the package-level ListInstalledProviders function.
func TestListInstalledProviders(t *testing.T) {
	ctx := context.Background()

	// Get installed providers from default registry
	installed := ListInstalledProviders(ctx)

	// Verify the result is valid
	for _, p := range installed {
		if !p.IsInstalled(ctx) {
			t.Errorf("ListInstalledProviders returned %s but IsInstalled returns false", p.Name())
		}
	}
}

func TestGeminiProvider(t *testing.T) {
	p := NewGeminiProvider()

	if p.Name() != "gemini" {
		t.Errorf("expected name 'gemini', got %q", p.Name())
	}
	if p.Description() == "" {
		t.Error("expected non-empty description")
	}
	if p.Command() != "gemini --yolo" {
		t.Errorf("expected command 'gemini --yolo', got %q", p.Command())
	}
}

func TestGeminiDetectState(t *testing.T) {
	p := NewGeminiProvider()

	tests := []struct {
		name   string
		output string
		want   State
	}{
		{"working spinner", "⠋ Loading model...\n", StateWorking},
		{"working thinking", "thinking about your question\n", StateWorking},
		{"working generating", "generating response\n", StateWorking},
		{"working searching", "searching for information\n", StateWorking},
		{"done check", "✓ Response complete\n", StateDone},
		{"done finished", "finished processing\n", StateDone},
		{"stuck rate limit", "rate limit exceeded\n", StateStuck},
		{"stuck quota", "quota exceeded for today\n", StateStuck},
		{"stuck timeout", "request timeout\n", StateStuck},
		{"error", "error: model not found\n", StateError},
		{"error failed", "failed to generate\n", StateError},
		{"idle prompt", "> ", StateIdle},
		{"idle gemini prompt", "gemini> ", StateIdle},
		{"idle ready", "ready for input\n", StateIdle},
		{"unknown", "some random output\n", StateUnknown},
		{"empty", "", StateUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.DetectState(tt.output)
			if got != tt.want {
				t.Errorf("DetectState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCursorProvider(t *testing.T) {
	p := NewCursorProvider()

	if p.Name() != "cursor" {
		t.Errorf("expected name 'cursor', got %q", p.Name())
	}
	if p.Description() == "" {
		t.Error("expected non-empty description")
	}
	if p.Command() != "cursor-agent" {
		t.Errorf("expected command 'cursor-agent', got %q", p.Command())
	}
}

func TestProviderBinaryAndInstallHint(t *testing.T) {
	tests := []struct {
		name        string
		provider    Provider
		binary      string
		installHint string
	}{
		{"claude", NewClaudeProvider(), "claude", "npx -y @anthropic-ai/claude-code"},
		{"gemini", NewGeminiProvider(), "gemini", "pip install google-generativeai"},
		{"cursor", NewCursorProvider(), "cursor-agent", "https://cursor.sh"},
		{"codex", NewCodexProvider(), "codex", "npm install -g @openai/codex"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.Binary(); got != tt.binary {
				t.Errorf("Binary() = %q, want %q", got, tt.binary)
			}
			if got := tt.provider.InstallHint(); got != tt.installHint {
				t.Errorf("InstallHint() = %q, want %q", got, tt.installHint)
			}
		})
	}
}

func TestProviderBuildCommand(t *testing.T) {
	tests := []struct {
		name     string
		want     string
		provider Provider
		opts     CommandOpts
	}{
		{"claude no opts", "claude --dangerously-skip-permissions", NewClaudeProvider(), CommandOpts{}},
		{"claude with agent", "claude --dangerously-skip-permissions", NewClaudeProvider(), CommandOpts{AgentName: "eng-01"}},
		{"gemini no opts", "gemini --yolo", NewGeminiProvider(), CommandOpts{}},
		{"gemini with agent", "gemini --yolo", NewGeminiProvider(), CommandOpts{AgentName: "eng-01"}},
		{"codex no opts", "codex --full-auto", NewCodexProvider(), CommandOpts{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.BuildCommand(tt.opts)
			if got != tt.want {
				t.Errorf("BuildCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContainerCustomizer(t *testing.T) {
	claude := NewClaudeProvider()

	// Claude implements ContainerCustomizer
	cc, ok := interface{}(claude).(ContainerCustomizer)
	if !ok {
		t.Fatal("ClaudeProvider should implement ContainerCustomizer")
	}

	// Test AdjustContainerCommand — wraps in explicit tmux session
	adjusted := cc.AdjustContainerCommand("claude --dangerously-skip-permissions")
	if !strings.Contains(adjusted, "tmux new-session") {
		t.Errorf("AdjustContainerCommand() should wrap in tmux, got %q", adjusted)
	}
	if !strings.Contains(adjusted, "claude --dangerously-skip-permissions") {
		t.Errorf("AdjustContainerCommand() should preserve original command, got %q", adjusted)
	}

	// DockerImage returns empty
	if img := cc.DockerImage(); img != "" {
		t.Errorf("DockerImage() = %q, want empty", img)
	}

	// Gemini does NOT implement ContainerCustomizer
	gemini := NewGeminiProvider()
	if _, ok := interface{}(gemini).(ContainerCustomizer); ok {
		t.Error("GeminiProvider should not implement ContainerCustomizer")
	}
}

func TestCursorDetectState(t *testing.T) {
	p := NewCursorProvider()

	tests := []struct {
		name   string
		output string
		want   State
	}{
		{"working spinner", "⠙ Processing request...\n", StateWorking},
		{"working thinking", "thinking about changes\n", StateWorking},
		{"working applying", "applying edits to file\n", StateWorking},
		{"done check", "✔ Changes applied\n", StateDone},
		{"done applied", "applied changes to 3 files\n", StateDone},
		{"done complete", "edit complete\n", StateDone},
		{"stuck rate limit", "rate limit hit, retrying\n", StateStuck},
		{"stuck connection", "connection refused\n", StateStuck},
		{"stuck timeout", "request timeout\n", StateStuck},
		{"error", "error: file not found\n", StateError},
		{"error failed", "failed to apply edit\n", StateError},
		{"error emoji", "❌ Operation failed\n", StateError},
		{"idle prompt", "> ", StateIdle},
		{"idle cursor prompt", "cursor> ", StateIdle},
		{"idle ready", "ready\n", StateIdle},
		{"unknown", "some output text\n", StateUnknown},
		{"empty", "", StateUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.DetectState(tt.output)
			if got != tt.want {
				t.Errorf("DetectState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClaudeSessionResumer(t *testing.T) {
	p := NewClaudeProvider()

	// Verify interface implementation
	sr, ok := interface{}(p).(SessionResumer)
	if !ok {
		t.Fatal("ClaudeProvider must implement SessionResumer")
	}
	if !sr.SupportsResume() {
		t.Error("ClaudeProvider.SupportsResume() must return true")
	}
}

func TestClaudeParseSessionID(t *testing.T) {
	p := NewClaudeProvider()

	tests := []struct {
		name   string
		output string
		wantID string
	}{
		{
			name: "standard resume line",
			output: `Some output here...
Resume this session with:
claude --resume cc78cadf-89ce-4820-ab6e-950afd2b6838`,
			wantID: "cc78cadf-89ce-4820-ab6e-950afd2b6838",
		},
		{
			name: "resume line in middle of output",
			output: `❯ 
claude --resume aa11bb22-cc33-dd44-ee55-ff6677889900
Some more output`,
			wantID: "aa11bb22-cc33-dd44-ee55-ff6677889900",
		},
		{
			name:   "no session ID present",
			output: "Normal claude output without resume line",
			wantID: "",
		},
		{
			name:   "empty output",
			output: "",
			wantID: "",
		},
		{
			name:   "malformed UUID",
			output: "claude --resume not-a-valid-uuid-here",
			wantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.ParseSessionID(tt.output)
			if got != tt.wantID {
				t.Errorf("ParseSessionID() = %q, want %q", got, tt.wantID)
			}
		})
	}
}

func TestClaudeBuildCommandSessionID(t *testing.T) {
	p := NewClaudeProvider()

	tests := []struct { //nolint:govet // test struct, field order matches literal values
		name string
		want string
		opts CommandOpts
	}{
		{
			name: "session ID takes priority over resume flag",
			opts: CommandOpts{
				AgentName: "eng-01",
				SessionID: "cc78cadf-89ce-4820-ab6e-950afd2b6838",
				Resume:    true,
			},
			want: "claude --dangerously-skip-permissions --resume cc78cadf-89ce-4820-ab6e-950afd2b6838",
		},
		{
			name: "session ID alone",
			opts: CommandOpts{
				AgentName: "eng-01",
				SessionID: "cc78cadf-89ce-4820-ab6e-950afd2b6838",
			},
			want: "claude --dangerously-skip-permissions --resume cc78cadf-89ce-4820-ab6e-950afd2b6838",
		},
		{
			name: "resume flag without session ID — no special flag",
			opts: CommandOpts{
				AgentName: "eng-01",
				Resume:    true,
			},
			want: "claude --dangerously-skip-permissions",
		},
		{
			name: "no resume flags — fresh session",
			opts: CommandOpts{AgentName: "eng-01"},
			want: "claude --dangerously-skip-permissions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.BuildCommand(tt.opts)
			if got != tt.want {
				t.Errorf("BuildCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}
