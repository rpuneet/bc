package provider

import (
	"context"
	"regexp"
	"strings"
)

// PiProvider implements the Provider interface for pi CLI.
// pi is an AI coding assistant with read, bash, edit, write tools.
//
// Issue: Add Pi provider support to mycel
// https://github.com/rpuneet/mycel/issues/TODO
//
// pi supports session resumption via --session and --resume flags:
//   - --session <path|id>  Use specific session file or partial UUID
//   - --resume           Select a session to resume
//   - --continue         Continue previous session
//
// Session resumption is tracked through pi's session files in PI_CODING_AGENT_SESSION_DIR.
// Use the full session ID (UUID) to resume a specific session.
type PiProvider struct {
	*GenericAdapter
	name        string
	description string
	command     string
	binary      string
}

// NewPiProvider creates a new pi provider.
func NewPiProvider() *PiProvider {
	return &PiProvider{
		GenericAdapter: NewGenericAdapter("pi"),
		name:           "pi",
		description:    "Pi coding assistant CLI",
		command:        "pi",
		binary:         "pi",
	}
}

// Name returns the provider's unique identifier.
func (p *PiProvider) Name() string {
	return p.name
}

// Description returns a human-readable description.
func (p *PiProvider) Description() string {
	return p.description
}

// Command returns the shell command to start this provider.
func (p *PiProvider) Command() string {
	return p.command
}

// Binary returns the executable name for LookPath/version checks.
func (p *PiProvider) Binary() string {
	return p.binary
}

// InstallHint returns a human-readable install instruction.
func (p *PiProvider) InstallHint() string {
	return "npm install -g @earendil-works/pi-coding-agent"
}

// BuildCommand returns the full command for a given runtime context.
// For pi, we start with base command and add model/session flags as
// needed. Both values are spliced into a shell command line — double
// quotes don't stop `$()` expansion, so unsafe values are dropped.
func (p *PiProvider) BuildCommand(opts CommandOpts) string {
	cmd := p.Command()
	if SafeModelName(opts.Model) {
		cmd += " --model " + opts.Model
	}
	if SafeSessionID(opts.SessionID) {
		cmd += " --session " + opts.SessionID
	}
	if opts.Resume {
		cmd += " --continue"
	}
	return cmd
}

// Models returns the curated model list for the pi CLI. pi's --model
// takes a "provider/id" pattern with an optional ":<thinking>" suffix.
func (p *PiProvider) Models() []string {
	return []string{"google/gemini-2.5-pro", "anthropic/claude-sonnet-4-6", "openai/gpt-5.2"}
}

// IsInstalled checks if the provider binary is available.
func (p *PiProvider) IsInstalled(ctx context.Context) bool {
	return checkBinaryExists(ctx, p.binary)
}

// piVersionRe extracts version from pi --version output.
var piVersionRe = regexp.MustCompile(`(\d+\.\d+\.\d+)`)

// Version returns the installed version.
func (p *PiProvider) Version(ctx context.Context) string {
	raw := getBinaryVersion(ctx, p.binary, "--version")
	if m := piVersionRe.FindString(raw); m != "" {
		return m
	}
	return raw
}

// DetectState analyzes output to determine agent state.
// Pi outputs state information when using appropriate flags.
func (p *PiProvider) DetectState(output string) State {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return StateUnknown
	}

	// Check last few lines for state indicators
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-5; i-- {
		line := strings.TrimSpace(lines[i])
		lineLower := strings.ToLower(line)

		// Working indicators
		if strings.Contains(lineLower, "thinking") ||
			strings.Contains(lineLower, "processing") ||
			strings.Contains(lineLower, "executing") ||
			strings.Contains(lineLower, "running") ||
			strings.Contains(lineLower, "generating") ||
			strings.Contains(lineLower, "analyzing") ||
			strings.Contains(lineLower, "planning") {
			return StateWorking
		}

		// Tool call indicators
		if strings.Contains(lineLower, "tool call") ||
			strings.Contains(lineLower, "calling tool") ||
			strings.Contains(lineLower, "running command") {
			return StateWorking
		}

		// Done indicators
		if strings.Contains(lineLower, "complete") ||
			strings.Contains(lineLower, "finished") ||
			strings.Contains(lineLower, "done") ||
			strings.Contains(lineLower, "success") ||
			strings.Contains(line, "✓") ||
			strings.Contains(line, "✔") {
			return StateDone
		}

		// Error indicators
		if strings.Contains(lineLower, "error") ||
			strings.Contains(lineLower, "failed") ||
			strings.Contains(lineLower, "exception") ||
			strings.Contains(line, "✗") ||
			strings.Contains(line, "✘") {
			return StateError
		}

		// Stuck indicators
		if strings.Contains(lineLower, "timeout") ||
			strings.Contains(lineLower, "rate limit") ||
			strings.Contains(lineLower, "quota exceeded") ||
			strings.Contains(lineLower, "blocked") {
			return StateStuck
		}

		// Idle indicators - pi specific markers
		if strings.Contains(lineLower, "ready") ||
			strings.Contains(lineLower, "awaiting") ||
			strings.Contains(lineLower, "prompt") ||
			strings.Contains(lineLower, "waiting") ||
			strings.Contains(lineLower, "listening") {
			return StateIdle
		}
	}

	// If we see the prompt indicator, we're idle
	if strings.Contains(output, ">") || strings.Contains(output, "$") {
		return StateIdle
	}

	return StateUnknown
}

// Ensure PiProvider implements Provider interface.
var _ Provider = (*PiProvider)(nil)
var _ ModelLister = (*PiProvider)(nil)
