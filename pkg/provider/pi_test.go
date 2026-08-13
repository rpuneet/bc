package provider

import (
	"context"
	"strings"
	"testing"
)

func TestPiProviderIdentity(t *testing.T) {
	p := NewPiProvider()
	if p.Name() != "pi" {
		t.Errorf("Name() = %q, want pi", p.Name())
	}
	if p.Binary() != "pi" {
		t.Errorf("Binary() = %q, want pi", p.Binary())
	}
	if p.Command() != "pi" {
		t.Errorf("Command() = %q, want pi", p.Command())
	}
	if p.Description() == "" {
		t.Error("expected non-empty description")
	}
	if p.InstallHint() == "" {
		t.Error("expected non-empty install hint")
	}
}

func TestPiInterfaces(t *testing.T) {
	var p Provider = NewPiProvider()
	if _, ok := p.(ModelLister); !ok {
		t.Error("pi must implement ModelLister")
	}
	if _, ok := p.(DynamicModelLister); !ok {
		t.Error("pi must implement DynamicModelLister")
	}
}

func TestSafePiModelName(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"amazon-bedrock/moonshotai.kimi-k2.5", true},
		{"groq/llama-3.3-70b-versatile", true},
		{"groq/llama-3.1-8b-instant", true},
		{"anthropic/claude-sonnet-4-6", true},
		{"google/gemini-2.5-pro", true},
		{"moonshotai.kimi-k2.5", true},    // bare model without provider
		{"llama-3.3-70b-versatile", true}, // bare model with dashes
		{"", false},
		{"$(rm -rf /)", false},
		{"a b c", false},
		{"-model", false},            // leading dash = arg injection
		{"--provider amazon", false}, // spaces
		{"model;cat /etc/passwd", false},
		{"model`whoami`", false},
	}
	for _, tt := range tests {
		if got := SafePiModelName(tt.model); got != tt.want {
			t.Errorf("SafePiModelName(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

// piSpawnBase is the isolation prefix every mycel pi spawn must carry so
// ancestor ~/AGENTS.md cannot leak into context (#3678).
const piSpawnBase = "pi --no-context-files --append-system-prompt Pi.md"

func TestPiBuildCommand(t *testing.T) {
	p := NewPiProvider()
	tests := []struct { //nolint:govet // test struct, field order matches literal values
		name string
		want string
		opts CommandOpts
	}{
		{
			name: "no opts — isolation flags only",
			opts: CommandOpts{},
			want: piSpawnBase,
		},
		{
			name: "bedrock provider/model splits into --provider and --model",
			opts: CommandOpts{Model: "amazon-bedrock/moonshotai.kimi-k2.5"},
			want: piSpawnBase + " --provider amazon-bedrock --model moonshotai.kimi-k2.5",
		},
		{
			name: "groq provider/model splits correctly",
			opts: CommandOpts{Model: "groq/llama-3.3-70b-versatile"},
			want: piSpawnBase + " --provider groq --model llama-3.3-70b-versatile",
		},
		{
			name: "bare model without slash — single --model flag",
			opts: CommandOpts{Model: "llama-3.3-70b-versatile"},
			want: piSpawnBase + " --model llama-3.3-70b-versatile",
		},
		{
			name: "unsafe model is dropped",
			opts: CommandOpts{Model: "$(rm -rf /)"},
			want: piSpawnBase,
		},
		{
			name: "leading-dash model is dropped (arg injection prevention)",
			opts: CommandOpts{Model: "-continue"},
			want: piSpawnBase,
		},
		{
			name: "session ID appended",
			opts: CommandOpts{SessionID: "abc123"},
			want: piSpawnBase + " --session abc123",
		},
		{
			name: "resume flag",
			opts: CommandOpts{Resume: true},
			want: piSpawnBase + " --continue",
		},
		{
			name: "model + session + resume — session wins over --continue",
			opts: CommandOpts{
				Model:     "groq/llama-3.3-70b-versatile",
				SessionID: "abc123",
				Resume:    true,
			},
			want: piSpawnBase + " --provider groq --model llama-3.3-70b-versatile --session abc123",
		},
		{
			name: "model with dots in model id",
			opts: CommandOpts{Model: "anthropic/claude-sonnet-4-6"},
			want: piSpawnBase + " --provider anthropic --model claude-sonnet-4-6",
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

// TestPiIsolateSpawnCommand_Idempotent covers prefs overrides that already
// include -nc / --no-context-files so we do not double-append (#3678).
func TestPiIsolateSpawnCommand_Idempotent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bare pi gets both flags",
			in:   "pi",
			want: piSpawnBase,
		},
		{
			name: "existing long flag keeps append only",
			in:   "pi --no-context-files --provider amazon-bedrock",
			want: "pi --no-context-files --provider amazon-bedrock --append-system-prompt Pi.md",
		},
		{
			name: "existing short -nc is respected",
			in:   "pi -nc --model foo",
			want: "pi -nc --model foo --append-system-prompt Pi.md",
		},
		{
			name: "both flags already present — unchanged",
			in:   piSpawnBase + " --continue",
			want: piSpawnBase + " --continue",
		},
		{
			name: "empty command untouched",
			in:   "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PiIsolateSpawnCommand(tt.in)
			if got != tt.want {
				t.Errorf("PiIsolateSpawnCommand(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestPiPromptFileIsManagedAppendTarget pins the file name that
// --append-system-prompt must reference (GenericAdapter → Pi.md).
func TestPiPromptFileIsManagedAppendTarget(t *testing.T) {
	p := NewPiProvider()
	if got := p.PromptFile(); got != piManagedPromptFile {
		t.Fatalf("PromptFile() = %q, want %q (append-system-prompt target)", got, piManagedPromptFile)
	}
	cmd := p.BuildCommand(CommandOpts{})
	if !strings.Contains(cmd, "--append-system-prompt "+piManagedPromptFile) {
		t.Fatalf("BuildCommand missing managed append: %q", cmd)
	}
	if !strings.Contains(cmd, "--no-context-files") {
		t.Fatalf("BuildCommand missing --no-context-files: %q", cmd)
	}
}

func TestPiModels(t *testing.T) {
	p := NewPiProvider()
	// Static Models() returns empty — live list comes from ListModels.
	// Mycel must not bake in model choices; the user picks from what pi reports.
	models := p.Models()
	if len(models) != 0 {
		t.Errorf("Models() = %v, want empty static list (use ListModels for live list)", models)
	}
}

func TestPiListModels(t *testing.T) {
	p := NewPiProvider()
	orig := piListModels
	t.Cleanup(func() { piListModels = orig })

	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name: "two-column rows joined with slash",
			output: "groq           llama-3.3-70b-versatile\n" +
				"anthropic      claude-sonnet-4-6\n" +
				"amazon-bedrock moonshotai.kimi-k2.5\n",
			want: []string{
				"groq/llama-3.3-70b-versatile",
				"anthropic/claude-sonnet-4-6",
				"amazon-bedrock/moonshotai.kimi-k2.5",
			},
		},
		{
			name:   "empty output returns empty list",
			output: "",
			want:   []string{},
		},
		{
			name:   "single-column rows are skipped",
			output: "header\ngroq  llama-3.3-70b-versatile\n",
			want:   []string{"groq/llama-3.3-70b-versatile"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captured := tt.output
			piListModels = func(_ context.Context) (string, error) { return captured, nil }
			got, err := p.ListModels(t.Context())
			if err != nil {
				t.Fatalf("ListModels() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ListModels() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ListModels()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestPiListModels_AllUnparseableFallback verifies that when `pi --list-models`
// exits 0 but emits no parseable two-column rows, ListModels returns the static
// fallback (p.Models()) rather than nil (bug #4).
func TestPiListModels_AllUnparseableFallback(t *testing.T) {
	p := NewPiProvider()
	orig := piListModels
	t.Cleanup(func() { piListModels = orig })

	// Output is non-empty but every row has only one column — all skipped.
	piListModels = func(_ context.Context) (string, error) {
		return "MODELS\nNOTE: run pi --setup to configure providers\n", nil
	}
	got, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() unexpected error: %v", err)
	}
	// pi's static Models() is empty, so we expect empty (not nil).
	want := p.Models()
	if len(got) != len(want) {
		t.Errorf("ListModels() = %v (len=%d), want static fallback %v (len=%d)", got, len(got), want, len(want))
	}
}
