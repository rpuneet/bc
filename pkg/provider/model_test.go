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
		{"claude injects --model", NewClaudeProvider(), "fable", " --model 'fable'", ""},
		{"agy injects quoted --model", NewAgyProvider(), "gemini-3.5-flash-medium", " --model 'gemini-3.5-flash-medium'", ""},
		{"cursor injects --model", NewCursorProvider(), "composer-2.5", " --model composer-2.5", ""},
		{"codex injects --model", NewCodexProvider(), "gpt-5.6-sol", " --model gpt-5.6-sol", ""},
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
		{"claude", NewClaudeProvider(), []string{"sonnet", "opus", "haiku", "fable", "best", "opusplan", "default", "sonnet[1m]", "opus[1m]", "fable[1m]"}},
		{"cursor", NewCursorProvider(), []string{"auto", "gpt-5.3-codex", "gpt-5.3-codex-high", "gpt-5.2", "composer-2.5"}},
		{"codex", NewCodexProvider(), []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.2"}},
		// pi has no static curated list — ListModels (DynamicModelLister) provides the live list from pi --list-models.
		{"pi", NewPiProvider(), []string{}},
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
			// Every curated model must survive the provider's injection gate.
			for _, m := range got {
				switch tt.name {
				case "claude":
					if !SafeClaudeModelName(m) {
						t.Errorf("curated model %q fails SafeClaudeModelName", m)
					}
				default:
					if !SafeModelName(m) {
						t.Errorf("curated model %q fails SafeModelName", m)
					}
				}
			}
		})
	}
}
