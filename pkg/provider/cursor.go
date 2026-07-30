package provider

import (
	"context"
)

// CursorProvider implements the Provider interface for Cursor Agent.
type CursorProvider struct {
	CursorConfigAdapter
	name        string
	description string
	command     string
	binary      string
}

func init() { Register(NewCursorProvider()) }

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
// Supports --resume with session ID for session continuation, and
// --model <m> for model selection. Both values are spliced into a shell
// command line, so unsafe values are dropped.
func (p *CursorProvider) BuildCommand(opts CommandOpts) string {
	cmd := p.command
	if SafeModelName(opts.Model) {
		cmd += " --model " + opts.Model
	}
	if SafeSessionID(opts.SessionID) {
		cmd += " --resume " + opts.SessionID
	}
	return cmd
}

// Models returns the curated model list for the Cursor Agent CLI,
// taken from `cursor-agent --list-models`.
func (p *CursorProvider) Models() []string {
	return []string{"auto", "gpt-5.3-codex", "gpt-5.3-codex-high", "gpt-5.2", "sonnet-4-thinking"}
}

// IsInstalled checks if the provider binary is available.
func (p *CursorProvider) IsInstalled(ctx context.Context) bool {
	return checkBinaryExists(ctx, p.binary)
}

// Version returns the installed version.
func (p *CursorProvider) Version(ctx context.Context) string {
	return getBinaryVersion(ctx, p.binary, "--version")
}

// Ensure CursorProvider implements Provider interface.
var _ Provider = (*CursorProvider)(nil)
var _ ModelLister = (*CursorProvider)(nil)
