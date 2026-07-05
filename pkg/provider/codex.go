package provider

import (
	"context"
	"regexp"
	"strings"
)

// CodexProvider implements the Provider interface for OpenAI Codex CLI.
// Codex is OpenAI's code generation model.
//
// Issue #1479: Codex CLI Provider Integration
type CodexProvider struct {
	*GenericAdapter
	name        string
	description string
	command     string
	binary      string
}

// NewCodexProvider creates a new Codex provider.
func NewCodexProvider() *CodexProvider {
	return &CodexProvider{
		GenericAdapter: NewGenericAdapter("codex"),
		name:           "codex",
		description:    "OpenAI Codex CLI",
		command:        "codex --full-auto",
		binary:         "codex",
	}
}

// Name returns the provider's unique identifier.
func (p *CodexProvider) Name() string {
	return p.name
}

// Description returns a human-readable description.
func (p *CodexProvider) Description() string {
	return p.description
}

// Command returns the shell command to start this provider.
func (p *CodexProvider) Command() string {
	return p.command
}

// Binary returns the executable name for LookPath/version checks.
func (p *CodexProvider) Binary() string {
	return p.binary
}

// InstallHint returns a human-readable install instruction.
func (p *CodexProvider) InstallHint() string {
	return "npm install -g @openai/codex"
}

// BuildCommand returns the full command for a given runtime context.
// Supports --model <m> for model selection; the value is spliced into a
// shell command line, so unsafe values are dropped.
func (p *CodexProvider) BuildCommand(opts CommandOpts) string {
	cmd := p.command
	if SafeModelName(opts.Model) {
		cmd += " --model " + opts.Model
	}
	return cmd
}

// Models returns the curated model list for the Codex CLI.
func (p *CodexProvider) Models() []string {
	return []string{"gpt-5.3-codex", "gpt-5.2-codex", "gpt-5.2"}
}

// IsInstalled checks if the provider binary is available.
func (p *CodexProvider) IsInstalled(ctx context.Context) bool {
	return checkBinaryExists(ctx, p.binary)
}

// codexVersionRe extracts a semver-like version from codex --version output
// which may look like "codex-cli 0.111.0" or "v0.111.0".
var codexVersionRe = regexp.MustCompile(`(\d+\.\d+\.\d+)`)

// Version returns the installed version, stripped of any prefix like "codex-cli".
func (p *CodexProvider) Version(ctx context.Context) string {
	raw := getBinaryVersion(ctx, p.binary, "--version")
	if m := codexVersionRe.FindString(raw); m != "" {
		return m
	}
	return raw
}

// DetectState analyzes output to determine agent state.
// Codex uses specific output patterns for state detection.
func (p *CodexProvider) DetectState(output string) State {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return StateUnknown
	}

	// Check last few lines for state indicators
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-5; i-- {
		line := strings.TrimSpace(lines[i])
		lineLower := strings.ToLower(line)

		// Working indicators - Codex spinner patterns
		if strings.HasPrefix(line, "\u280b") ||
			strings.HasPrefix(line, "\u2819") ||
			strings.HasPrefix(line, "\u2839") ||
			strings.HasPrefix(line, "\u2838") ||
			strings.HasPrefix(line, "\u283c") ||
			strings.HasPrefix(line, "\u2834") ||
			strings.Contains(lineLower, "generating") ||
			strings.Contains(lineLower, "thinking") ||
			strings.Contains(lineLower, "processing") ||
			strings.Contains(lineLower, "executing") {
			return StateWorking
		}

		// Done indicators
		if strings.Contains(line, "\u2713") ||
			strings.Contains(line, "\u2714") ||
			strings.Contains(lineLower, "complete") ||
			strings.Contains(lineLower, "finished") ||
			strings.Contains(lineLower, "done") ||
			strings.Contains(lineLower, "success") {
			return StateDone
		}

		// Error indicators
		if strings.Contains(lineLower, "error") ||
			strings.Contains(lineLower, "failed") ||
			strings.Contains(lineLower, "exception") ||
			strings.Contains(line, "\u2717") ||
			strings.Contains(line, "\u2716") {
			return StateError
		}

		// Stuck indicators
		if strings.Contains(lineLower, "timeout") ||
			strings.Contains(lineLower, "rate limit") ||
			strings.Contains(lineLower, "quota exceeded") {
			return StateStuck
		}

		// Idle/prompt indicators
		if strings.HasPrefix(line, ">") ||
			strings.HasPrefix(line, "$") ||
			strings.HasPrefix(line, "codex>") ||
			strings.Contains(lineLower, "ready") ||
			strings.Contains(lineLower, "awaiting") {
			return StateIdle
		}
	}

	return StateUnknown
}

// Ensure CodexProvider implements Provider interface.
var _ Provider = (*CodexProvider)(nil)
var _ ModelLister = (*CodexProvider)(nil)
