package provider

import (
	"context"
	"regexp"
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

// Ensure CodexProvider implements Provider interface.
var _ Provider = (*CodexProvider)(nil)
var _ ModelLister = (*CodexProvider)(nil)
