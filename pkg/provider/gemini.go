package provider

import (
	"context"
	"strings"
)

// GeminiProvider implements the Provider interface for Google Gemini CLI.
type GeminiProvider struct {
	*GenericAdapter // GEMINI.md prompt, no special config
	name            string
	description     string
	command         string
	binary          string
}

// NewGeminiProvider creates a new Gemini provider.
func NewGeminiProvider() *GeminiProvider {
	return &GeminiProvider{
		GenericAdapter: NewGenericAdapter("gemini"),
		name:           "gemini",
		description:    "Google Gemini CLI",
		command:        "gemini --yolo",
		binary:         "gemini",
	}
}

// Name returns the provider's unique identifier.
func (p *GeminiProvider) Name() string {
	return p.name
}

// Description returns a human-readable description.
func (p *GeminiProvider) Description() string {
	return p.description
}

// Command returns the shell command to start this provider.
func (p *GeminiProvider) Command() string {
	return p.command
}

// Binary returns the executable name for LookPath/version checks.
func (p *GeminiProvider) Binary() string {
	return p.binary
}

// InstallHint returns a human-readable install instruction.
func (p *GeminiProvider) InstallHint() string {
	return "pip install google-generativeai"
}

// BuildCommand returns the full command for a given runtime context.
// Supports --resume with session ID for session continuation, and
// -m <model> for model selection. Both values are spliced into a shell
// command line, so unsafe values are dropped.
func (p *GeminiProvider) BuildCommand(opts CommandOpts) string {
	cmd := p.command
	if SafeModelName(opts.Model) {
		cmd += " -m " + opts.Model
	}
	if SafeSessionID(opts.SessionID) {
		cmd += " --resume " + opts.SessionID
	}
	return cmd
}

// Models returns the curated model list for the Gemini CLI.
func (p *GeminiProvider) Models() []string {
	return []string{"gemini-2.5-pro", "gemini-2.5-flash"}
}

// IsInstalled checks if the provider binary is available.
func (p *GeminiProvider) IsInstalled(ctx context.Context) bool {
	return checkBinaryExists(ctx, p.binary)
}

// Version returns the installed version.
func (p *GeminiProvider) Version(ctx context.Context) string {
	return getBinaryVersion(ctx, p.binary, "--version")
}

// DetectState analyzes output to determine agent state.
// Gemini CLI uses specific output patterns for state detection.
func (p *GeminiProvider) DetectState(output string) State {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return StateUnknown
	}

	for i := len(lines) - 1; i >= 0 && i >= len(lines)-5; i-- {
		line := strings.TrimSpace(lines[i])
		lineLower := strings.ToLower(line)

		// Done indicators (check before working to avoid "finished" matching "processing")
		if strings.Contains(line, "✓") ||
			strings.Contains(line, "✔") ||
			strings.Contains(lineLower, "complete") ||
			strings.Contains(lineLower, "finished") ||
			strings.Contains(lineLower, "done") {
			return StateDone
		}

		// Stuck indicators (check before working/error)
		if strings.Contains(lineLower, "rate limit") ||
			strings.Contains(lineLower, "quota") ||
			strings.Contains(lineLower, "timeout") {
			return StateStuck
		}

		// Error indicators (check before working)
		if strings.Contains(lineLower, "error") ||
			strings.Contains(lineLower, "failed") ||
			strings.Contains(line, "✗") ||
			strings.Contains(line, "✖") {
			return StateError
		}

		// Working indicators — Gemini spinner and activity patterns
		if strings.HasPrefix(line, "⠋") ||
			strings.HasPrefix(line, "⠙") ||
			strings.HasPrefix(line, "⠹") ||
			strings.HasPrefix(line, "⠸") ||
			strings.Contains(lineLower, "thinking") ||
			strings.Contains(lineLower, "generating") ||
			strings.Contains(lineLower, "processing") ||
			strings.Contains(lineLower, "searching") {
			return StateWorking
		}

		// Idle/prompt indicators
		if strings.HasPrefix(line, ">") ||
			strings.HasPrefix(line, "$") ||
			strings.HasPrefix(line, "gemini>") ||
			strings.Contains(lineLower, "ready") ||
			strings.Contains(lineLower, "awaiting") {
			return StateIdle
		}
	}

	return StateUnknown
}

// Ensure GeminiProvider implements Provider interface.
var _ Provider = (*GeminiProvider)(nil)
var _ ModelLister = (*GeminiProvider)(nil)
