package provider

import (
	"context"
	"os"
	"path/filepath"
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
	r.Register(NewAgyProvider())

	list := r.List()
	if len(list) != 2 {
		t.Errorf("expected 2 providers, got %d", len(list))
	}
}

func TestDefaultRegistryHasProviders(t *testing.T) {
	// The default registry holds exactly the built-in providers — nothing
	// more, nothing less. Names() is sorted, so compare directly.
	want := []string{"agy", "claude", "codex", "cursor", "openclaw", "pi"}
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
		NewAgyProvider(),
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
		NewAgyProvider(),
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
	r.Register(NewAgyProvider())

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

func TestAgyProvider(t *testing.T) {
	p := NewAgyProvider()

	if p.Name() != "agy" {
		t.Errorf("expected name 'agy', got %q", p.Name())
	}
	if p.Description() == "" {
		t.Error("expected non-empty description")
	}
	if p.Command() != "agy --dangerously-skip-permissions" {
		t.Errorf("expected command 'agy --dangerously-skip-permissions', got %q", p.Command())
	}
	if p.Binary() != "agy" {
		t.Errorf("expected binary 'agy', got %q", p.Binary())
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
		{"claude", NewClaudeProvider(), "claude", "npm install -g @anthropic-ai/claude-code"},
		{"agy", NewAgyProvider(), "agy", "curl -fsSL https://antigravity.google/install.sh | sh"},
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
		{"agy no opts", "agy --dangerously-skip-permissions", NewAgyProvider(), CommandOpts{}},
		{"agy with agent", "agy --dangerously-skip-permissions", NewAgyProvider(), CommandOpts{AgentName: "eng-01"}},
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

	// Codex now implements ContainerCustomizer so it can be tmux-wrapped and
	// driven under the Docker runtime (was previously unimplemented, leaving
	// codex undrivable in containers).
	codex := NewCodexProvider()
	cc2, ok := interface{}(codex).(ContainerCustomizer)
	if !ok {
		t.Fatal("CodexProvider should implement ContainerCustomizer")
	}
	if adjusted := cc2.AdjustContainerCommand("codex --full-auto"); !strings.Contains(adjusted, "tmux new-session") {
		t.Errorf("AdjustContainerCommand() should wrap in tmux, got %q", adjusted)
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
			name: "resume flag without session ID — continue last session",
			opts: CommandOpts{
				AgentName: "eng-01",
				Resume:    true,
			},
			want: "claude --dangerously-skip-permissions --continue",
		},
		{
			name: "no resume flags — fresh session",
			opts: CommandOpts{AgentName: "eng-01"},
			want: "claude --dangerously-skip-permissions",
		},
		{
			name: "malformed session ID is dropped, resume flag wins",
			opts: CommandOpts{
				AgentName: "eng-01",
				SessionID: "$(rm -rf /)",
				Resume:    true,
			},
			want: "claude --dangerously-skip-permissions --continue",
		},
		{
			name: "malformed session ID without resume — fresh session",
			opts: CommandOpts{
				AgentName: "eng-01",
				SessionID: "not-a-uuid-shape!",
			},
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

// TestClaudeHasResumableSession covers the --continue gate: Claude Code
// exits instead of starting fresh when the project has no session, so
// the detector must only report true when a transcript exists.
func TestClaudeHasResumableSession(t *testing.T) {
	p := NewClaudeProvider()
	home := t.TempDir()
	orig := claudeHomeDir
	claudeHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { claudeHomeDir = orig })

	wt := "/Users/u/.mycel/worktrees/eng-01"
	encoded := "-Users-u--mycel-worktrees-eng-01"
	projDir := filepath.Join(home, ".claude", "projects", encoded)

	if p.HasResumableSession(wt) {
		t.Error("no projects dir — must report false")
	}

	if err := os.MkdirAll(projDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if p.HasResumableSession(wt) {
		t.Error("empty projects dir — must report false")
	}

	if err := os.WriteFile(filepath.Join(projDir, "cc78cadf-89ce-4820-ab6e-950afd2b6838.jsonl"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !p.HasResumableSession(wt) {
		t.Error("transcript present — must report true")
	}

	if p.HasResumableSession("") {
		t.Error("empty dir must report false")
	}
}

// TestSafeSessionID covers the shell-splice guard, including argument
// injection via a leading dash.
func TestSafeSessionID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"cc78cadf-89ce-4820-ab6e-950afd2b6838", true},
		{"session_1.2", true},
		{"", false},
		{"$(rm -rf /)", false},
		{"a b", false},
		{"-continue", false},
		{"--dangerously-skip-permissions", false},
		{"a-b-c", true},
	}
	for _, tt := range tests {
		if got := SafeSessionID(tt.id); got != tt.want {
			t.Errorf("SafeSessionID(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

// TestExtractSemver covers the version-line normalization shared by every
// provider's Version() via getBinaryVersion. Guards the claude/cursor/agy
// regression where the decorated first line ("2.1.205 (Claude Code)") leaked
// through raw instead of the bare semver.
func TestExtractSemver(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"2.1.205 (Claude Code)", "2.1.205"},
		{"cursor-agent 0.4.1", "0.4.1"},
		{"codex-cli 0.111.0", "0.111.0"},
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"  0.9.0  ", "0.9.0"},
		{"no-version-here", "no-version-here"},
	}
	for _, tt := range tests {
		if got := extractSemver(tt.in); got != tt.want {
			t.Errorf("extractSemver(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
