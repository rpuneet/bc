package provider

import (
	"testing"
)

func TestOpenclawProviderIdentity(t *testing.T) {
	p := NewOpenclawProvider()
	if p.Name() != "openclaw" {
		t.Errorf("Name() = %q, want openclaw", p.Name())
	}
	if p.Binary() != "openclaw" {
		t.Errorf("Binary() = %q, want openclaw", p.Binary())
	}
	if p.Command() != "openclaw tui --local" {
		t.Errorf("Command() = %q, want %q", p.Command(), "openclaw tui --local")
	}
	if p.Description() == "" {
		t.Error("expected non-empty description")
	}
	if p.InstallHint() != "npm install -g openclaw" {
		t.Errorf("InstallHint() = %q, want %q", p.InstallHint(), "npm install -g openclaw")
	}
}

func TestOpenclawInterfaces(t *testing.T) {
	var p Provider = NewOpenclawProvider()
	if _, ok := p.(ContainerCustomizer); !ok {
		t.Error("openclaw must implement ContainerCustomizer")
	}
	if _, ok := p.(SessionCustomizer); !ok {
		t.Error("openclaw must implement SessionCustomizer")
	}
	// openclaw tui has no --model flag, so it must NOT claim ModelLister.
	if _, ok := p.(ModelLister); ok {
		t.Error("openclaw must not implement ModelLister (tui has no model flag)")
	}
}

func TestOpenclawBuildCommand(t *testing.T) {
	p := NewOpenclawProvider()
	tests := []struct { //nolint:govet // test struct, field order matches literal values
		name string
		want string
		opts CommandOpts
	}{
		{
			name: "no opts — base interactive command",
			opts: CommandOpts{},
			want: "openclaw tui --local",
		},
		{
			name: "session ID resumes via --session",
			opts: CommandOpts{SessionID: "main"},
			want: "openclaw tui --local --session main",
		},
		{
			name: "session ID with dotted key",
			opts: CommandOpts{SessionID: "agent.other.main"},
			want: "openclaw tui --local --session agent.other.main",
		},
		{
			name: "model is ignored — openclaw tui has no --model flag",
			opts: CommandOpts{Model: "anthropic/claude-sonnet-4-6"},
			want: "openclaw tui --local",
		},
		{
			name: "docker flag does not change the base command (wrapping is done by AdjustContainerCommand)",
			opts: CommandOpts{Docker: true},
			want: "openclaw tui --local",
		},
		{
			name: "resume without session ID is a no-op — bare tui auto-resumes",
			opts: CommandOpts{Resume: true},
			want: "openclaw tui --local",
		},
		{
			name: "resume with session ID emits --session for that key",
			opts: CommandOpts{SessionID: "main", Resume: true},
			want: "openclaw tui --local --session main",
		},
		{
			name: "unsafe session ID is dropped, not escaped",
			opts: CommandOpts{SessionID: "$(rm -rf /)"},
			want: "openclaw tui --local",
		},
		{
			name: "leading-dash session ID is dropped (arg injection prevention)",
			opts: CommandOpts{SessionID: "-deliver"},
			want: "openclaw tui --local",
		},
		{
			name: "session ID with shell metacharacters is dropped",
			opts: CommandOpts{SessionID: "main;cat /etc/passwd"},
			want: "openclaw tui --local",
		},
		{
			name: "model + docker + resume together — only base command, model/resume ignored",
			opts: CommandOpts{Model: "gpt-5", Docker: true, Resume: true},
			want: "openclaw tui --local",
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

func TestOpenclawAdjustSessionCommand(t *testing.T) {
	p := NewOpenclawProvider()
	// Native tmux: openclaw runs directly, no wrapping.
	in := "openclaw tui --local"
	if got := p.AdjustSessionCommand(in); got != in {
		t.Errorf("AdjustSessionCommand() = %q, want unchanged %q", got, in)
	}
}

func TestOpenclawAdjustContainerCommand(t *testing.T) {
	p := NewOpenclawProvider()
	// Docker: wrap in a tmux session so mycel can drive the TUI via SendKeys.
	in := "openclaw tui --local"
	want := `tmux new-session -s "$MYCEL_WORKTREE_NAME" "openclaw tui --local"`
	if got := p.AdjustContainerCommand(in); got != want {
		t.Errorf("AdjustContainerCommand() = %q, want %q", got, want)
	}
}

func TestOpenclawDockerImage(t *testing.T) {
	p := NewOpenclawProvider()
	// Empty means use the default mycel-agent-openclaw:latest convention.
	if got := p.DockerImage(); got != "" {
		t.Errorf("DockerImage() = %q, want empty", got)
	}
}

func TestOpenclawVersionParse(t *testing.T) {
	// Verify the version regex handles OpenClaw's date-based version scheme.
	tests := []struct {
		raw  string
		want string
	}{
		{"2026.6.11", "2026.6.11"},
		{"openclaw 2026.6.11", "2026.6.11"},
		{"v2026.6.11", "2026.6.11"},
		{"no-version-here", ""},
	}
	for _, tt := range tests {
		if got := openclawVersionRe.FindString(tt.raw); got != tt.want {
			t.Errorf("openclawVersionRe.FindString(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}
