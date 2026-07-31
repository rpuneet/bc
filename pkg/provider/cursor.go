package provider

import (
	"context"
	"fmt"
	"path/filepath"
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

// AdjustSessionCommand is a no-op for native tmux sessions: cursor-agent runs
// directly inside the mycel-managed tmux pane.
func (p *CursorProvider) AdjustSessionCommand(command string) string { return command }

// AdjustContainerCommand wraps the command in a tmux session for Docker so
// mycel can drive it via SendKeys. Double quotes let bash expand
// $MYCEL_WORKTREE_NAME for the session name.
func (p *CursorProvider) AdjustContainerCommand(command string) string {
	return fmt.Sprintf(`tmux new-session -s "$MYCEL_WORKTREE_NAME" "%s"`, command)
}

// DockerImage returns empty to use the default image-name convention.
func (p *CursorProvider) DockerImage() string { return "" }

// Models returns the curated model list for the Cursor Agent CLI,
// taken from `cursor-agent --list-models`.
func (p *CursorProvider) Models() []string {
	return []string{"auto", "gpt-5.3-codex", "gpt-5.3-codex-high", "gpt-5.2", "sonnet-4-thinking"}
}

// ReadMCPs lists the MCP servers from the workspace .cursor/mcp.json.
// An empty rootDir means no workspace is loaded and yields nothing.
func (p *CursorProvider) ReadMCPs(_ context.Context, rootDir string) []MCPServerInfo {
	if rootDir == "" {
		return []MCPServerInfo{}
	}
	return readMCPJSONFile(filepath.Join(rootDir, ".cursor", "mcp.json"))
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
var _ MCPConfigReader = (*CursorProvider)(nil)
var _ ContainerCustomizer = (*CursorProvider)(nil)
var _ SessionCustomizer = (*CursorProvider)(nil)
