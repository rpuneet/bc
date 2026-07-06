package provider

import (
	"strings"
	"testing"
)

func TestSafeModelName(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{"simple alias", "fable", true},
		{"dotted version", "claude-sonnet-4.5", true},
		{"namespaced with colon", "anthropic.claude-sonnet-4:0", true},
		{"provider slash form", "anthropic/claude-sonnet-4-6", true},
		{"slash with thinking suffix", "openai/gpt-5.2:high", true},
		{"underscore", "gpt_5", true},
		{"empty", "", false},
		{"leading dash", "-model", false},
		{"flag injection", "--dangerously-skip-permissions", false},
		{"space", "fable extra", false},
		{"command substitution", "$(rm -rf /)", false},
		{"semicolon", "fable;id", false},
		{"backtick", "`id`", false},
		{"quote", `fable"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SafeModelName(tt.model); got != tt.want {
				t.Errorf("SafeModelName(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestBuildCommandModelFlag(t *testing.T) {
	tests := []struct {
		name       string
		provider   Provider
		model      string
		wantFlag   string // substring the command must contain; "" means no injection expected
		wantAbsent string // substring the command must NOT contain
	}{
		{"claude injects --model", NewClaudeProvider(), "fable", " --model fable", ""},
		{"agy injects quoted --model", NewAgyProvider(), "Gemini 3 Flash", " --model 'Gemini 3 Flash'", ""},
		{"cursor injects --model", NewCursorProvider(), "sonnet-4-thinking", " --model sonnet-4-thinking", ""},
		{"codex injects --model", NewCodexProvider(), "gpt-5.3-codex", " --model gpt-5.3-codex", ""},
		// pi splits "provider/model" into separate --provider + --model flags for unambiguous routing.
		{"pi injects --model slash form", NewPiProvider(), "anthropic/claude-sonnet-4-6", " --provider anthropic --model claude-sonnet-4-6", ""},
		{"claude drops unsafe model", NewClaudeProvider(), "$(id)", "", "id"},
		{"agy drops unsafe model", NewAgyProvider(), "a$(id)", "", "id"},
		{"cursor drops leading dash", NewCursorProvider(), "--yolo", "", "--yolo"},
		{"codex drops unsafe model", NewCodexProvider(), "x;y", "", ";"},
		{"pi drops unsafe model", NewPiProvider(), "$(id)", "", "id"},
		{"claude empty model no flag", NewClaudeProvider(), "", "", "--model"},
		{"pi empty model no flag", NewPiProvider(), "", "", "--model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.provider.BuildCommand(CommandOpts{Model: tt.model})
			if tt.wantFlag != "" && !strings.Contains(cmd, tt.wantFlag) {
				t.Errorf("BuildCommand() = %q, want it to contain %q", cmd, tt.wantFlag)
			}
			if tt.wantFlag == "" && tt.wantAbsent != "" && strings.Contains(cmd, tt.wantAbsent) {
				t.Errorf("BuildCommand() = %q, must not contain %q", cmd, tt.wantAbsent)
			}
		})
	}
}

func TestProviderModels(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		want     []string
	}{
		{"claude", NewClaudeProvider(), []string{"fable", "opus", "opusplan", "sonnet", "haiku"}},
		{"cursor", NewCursorProvider(), []string{"auto", "gpt-5.3-codex", "gpt-5.3-codex-high", "gpt-5.2", "sonnet-4-thinking"}},
		{"codex", NewCodexProvider(), []string{"gpt-5.3-codex", "gpt-5.2-codex", "gpt-5.2"}},
		{"pi", NewPiProvider(), []string{"amazon-bedrock/moonshotai.kimi-k2.5", "groq/llama-3.3-70b-versatile", "groq/llama-3.1-8b-instant"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ml, ok := tt.provider.(ModelLister)
			if !ok {
				t.Fatalf("provider %s does not implement ModelLister", tt.name)
			}
			got := ml.Models()
			if len(got) != len(tt.want) {
				t.Fatalf("Models() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Models()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
			// Every curated model must survive SafeModelName so the UI
			// list and the injection gate never disagree.
			for _, m := range got {
				if !SafeModelName(m) {
					t.Errorf("curated model %q fails SafeModelName", m)
				}
			}
		})
	}
}
