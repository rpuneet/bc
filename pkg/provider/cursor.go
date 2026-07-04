package provider

import (
	"context"
	"strings"
)

// CursorProvider implements the Provider interface for Cursor Agent.
type CursorProvider struct {
	CursorConfigAdapter
	name        string
	description string
	command     string
	binary      string
}

// NewCursorProvider creates a new Cursor provider.
func NewCursorProvider() *CursorProvider {
	return &CursorProvider{
		name:        "cursor",
		description: "Cursor Agent CLI",
		command:     "cursor-agent",
		binary:      "cursor-agent",
	}
}

// Name returns the provider's unique identifier.
func (p *CursorProvider) Name() string {
	return p.name
}

// Description returns a human-readable description.
func (p *CursorProvider) Description() string {
	return p.description
}

// Command returns the shell command to start this provider.
func (p *CursorProvider) Command() string {
	return p.command
}

// Binary returns the executable name for LookPath/version checks.
func (p *CursorProvider) Binary() string {
	return p.binary
}

// InstallHint returns a human-readable install instruction.
func (p *CursorProvider) InstallHint() string {
	return "https://cursor.sh"
}

// BuildCommand returns the full command for a given runtime context.
// Supports --resume with session ID for session continuation. The ID is
// spliced into a shell command line, so unsafe values are dropped.
func (p *CursorProvider) BuildCommand(opts CommandOpts) string {
	cmd := p.command
	if SafeSessionID(opts.SessionID) {
		cmd += " --resume " + opts.SessionID
	}
	return cmd
}

// IsInstalled checks if the provider binary is available.
func (p *CursorProvider) IsInstalled(ctx context.Context) bool {
	return checkBinaryExists(ctx, p.binary)
}

// Version returns the installed version.
func (p *CursorProvider) Version(ctx context.Context) string {
	return getBinaryVersion(ctx, p.binary, "--version")
}

// DetectState analyzes output to determine agent state.
// Cursor Agent uses specific output patterns for state detection.
func (p *CursorProvider) DetectState(output string) State {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return StateUnknown
	}

	for i := len(lines) - 1; i >= 0 && i >= len(lines)-5; i-- {
		line := strings.TrimSpace(lines[i])
		lineLower := strings.ToLower(line)

		// Done indicators (check before working to avoid keyword overlap)
		if strings.Contains(line, "✓") ||
			strings.Contains(line, "✔") ||
			strings.Contains(lineLower, "complete") ||
			strings.Contains(lineLower, "finished") ||
			strings.Contains(lineLower, "applied") {
			return StateDone
		}

		// Stuck indicators (check before working/error)
		if strings.Contains(lineLower, "rate limit") ||
			strings.Contains(lineLower, "quota") ||
			strings.Contains(lineLower, "timeout") ||
			strings.Contains(lineLower, "connection refused") {
			return StateStuck
		}

		// Error indicators (check before working)
		if strings.Contains(lineLower, "error") ||
			strings.Contains(lineLower, "failed") ||
			strings.Contains(line, "✗") ||
			strings.Contains(line, "❌") {
			return StateError
		}

		// Working indicators — Cursor activity patterns
		if strings.HasPrefix(line, "⠋") ||
			strings.HasPrefix(line, "⠙") ||
			strings.HasPrefix(line, "⠹") ||
			strings.HasPrefix(line, "⠸") ||
			strings.Contains(lineLower, "thinking") ||
			strings.Contains(lineLower, "generating") ||
			strings.Contains(lineLower, "processing") ||
			strings.Contains(lineLower, "applying") {
			return StateWorking
		}

		// Idle/prompt indicators
		if strings.HasPrefix(line, ">") ||
			strings.HasPrefix(line, "$") ||
			strings.HasPrefix(line, "cursor>") ||
			strings.Contains(lineLower, "ready") ||
			strings.Contains(lineLower, "awaiting") {
			return StateIdle
		}
	}

	return StateUnknown
}

// Ensure CursorProvider implements Provider interface.
var _ Provider = (*CursorProvider)(nil)
