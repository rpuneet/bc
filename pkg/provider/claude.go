package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ClaudeProvider implements the Provider interface for Claude Code.
// Claude Code is the Anthropic CLI for Claude.
type ClaudeProvider struct {
	ClaudeConfigAdapter // embeds ConfigAdapter implementation
	name                string
	description         string
	command             string
	binary              string
}

func init() { Register(NewClaudeProvider()) }

// NewClaudeProvider creates a new Claude provider.
func NewClaudeProvider() *ClaudeProvider {
	return &ClaudeProvider{
		name:        "claude",
		description: "Anthropic Claude Code CLI",
		command:     "claude --dangerously-skip-permissions",
		binary:      "claude",
	}
}

// Name returns the provider's unique identifier.
func (p *ClaudeProvider) Name() string {
	return p.name
}

// Description returns a human-readable description.
func (p *ClaudeProvider) Description() string {
	return p.description
}

// Command returns the shell command to start this provider.
func (p *ClaudeProvider) Command() string {
	return p.command
}

// Binary returns the executable name for LookPath/version checks.
func (p *ClaudeProvider) Binary() string {
	return p.binary
}

// InstallHint returns a human-readable install instruction.
func (p *ClaudeProvider) InstallHint() string {
	return "npx -y @anthropic-ai/claude-code"
}

// BuildCommand returns the full command for a given runtime context.
// Includes --dangerously-skip-permissions. bc manages worktrees itself and starts
// agents directly in the worktree directory, so no -w flag is needed.
// --tmux is NOT included here — it's added by AdjustSessionCommand for Docker only.
// For native tmux, claude auto-detects the tmux environment.
// Resume priority: SessionID (--resume <id>) > Resume flag (--continue).
// Model priority: opts.Model is injected as --model <m> when it passes
// SafeModelName; unsafe values are dropped, never escaped.
func (p *ClaudeProvider) BuildCommand(opts CommandOpts) string {
	cmd := "claude --dangerously-skip-permissions"
	if SafeModelName(opts.Model) {
		cmd += " --model " + opts.Model
	}
	switch {
	case claudeSessionIDPattern.MatchString(opts.SessionID):
		cmd += " --resume " + opts.SessionID
	case opts.Resume:
		cmd += " --continue"
	}
	return cmd
}

// Models returns the curated model list for the Claude Code CLI.
func (p *ClaudeProvider) Models() []string {
	return []string{"fable", "opus", "opusplan", "sonnet", "haiku"}
}

// claudeSessionIDPattern is the full-string UUID shape of a Claude session
// ID. The ID is spliced into a shell command line, so anything else —
// including an empty string — is rejected rather than quoted.
var claudeSessionIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// AdjustSessionCommand is a no-op for native tmux sessions.
// Claude auto-detects the tmux environment when running inside a bc-managed tmux session.
func (p *ClaudeProvider) AdjustSessionCommand(command string) string {
	return command
}

// AdjustContainerCommand wraps in a tmux session for Docker.
// Uses double quotes so bash expands $MYCEL_WORKTREE_NAME for the session name.
func (p *ClaudeProvider) AdjustContainerCommand(command string) string {
	return fmt.Sprintf(`tmux new-session -s "$MYCEL_WORKTREE_NAME" "%s"`, command)
}

// DockerImage returns empty to use default convention.
func (p *ClaudeProvider) DockerImage() string { return "" }

// IsInstalled checks if the provider binary is available.
func (p *ClaudeProvider) IsInstalled(ctx context.Context) bool {
	return checkBinaryExists(ctx, p.binary)
}

// Version returns the installed version.
func (p *ClaudeProvider) Version(ctx context.Context) string {
	return getBinaryVersion(ctx, p.binary, "--version")
}

// claudeResumePattern matches Claude's "Resume this session with: claude --resume <uuid>" output.
// The UUID format is standard 8-4-4-4-12 hex.
var claudeResumePattern = regexp.MustCompile(`claude --resume ([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// SupportsResume reports that Claude Code supports resuming sessions by ID.
func (p *ClaudeProvider) SupportsResume() bool { return true }

// claudeHomeDir is overridable in tests.
var claudeHomeDir = os.UserHomeDir

// HasResumableSession reports whether Claude Code has a prior session
// transcript for the given working directory. `claude --continue` EXITS
// (instead of starting fresh) when the project has no session, so the
// flag must only be passed when a transcript exists. Claude keys
// transcripts by the project path with `/` and `.` replaced by `-`:
// ~/.claude/projects/<encoded>/<session-uuid>.jsonl.
func (p *ClaudeProvider) HasResumableSession(dir string) bool {
	home, err := claudeHomeDir()
	if err != nil || dir == "" {
		return false
	}
	encoded := strings.NewReplacer("/", "-", ".", "-").Replace(dir)
	entries, err := os.ReadDir(filepath.Join(home, ".claude", "projects", encoded))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			return true
		}
	}
	return false
}

// ParseSessionID scans tool output for Claude's resume hint and returns the session UUID.
// Returns "" if no session ID is found.
// Claude prints "Resume this session with:\nclaude --resume <uuid>" on graceful exit.
func (p *ClaudeProvider) ParseSessionID(output string) string {
	m := claudeResumePattern.FindStringSubmatch(output)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// ActivityMode reports that Claude Code emits activity via lifecycle hooks
// (configured in .claude/settings.json) that POST to bcd's hook endpoint.
func (p *ClaudeProvider) ActivityMode() string { return ActivityModeHooks }

// WriteHookConfig writes Claude Code hook settings into the agent worktree.
// daemonAddr and agentID are unused: the generated hook commands resolve the
// daemon address and agent identity at runtime via the MYCEL_DAEMON_ADDR and
// MYCEL_AGENT_ID environment variables set on the agent session.
func (p *ClaudeProvider) WriteHookConfig(worktreeDir, _, _ string) error {
	return WriteClaudeHookSettings(worktreeDir)
}

// TranscriptGlobs returns glob patterns matching Claude Code session
// transcripts for an agent working in cwd. Claude keys transcripts by the
// project path with `/` and `.` replaced by `-`:
// ~/.claude/projects/<encoded>/<session-uuid>.jsonl.
func (p *ClaudeProvider) TranscriptGlobs(cwd string) []string {
	home, err := claudeHomeDir()
	if err != nil || cwd == "" {
		return nil
	}
	encoded := strings.NewReplacer("/", "-", ".", "-").Replace(cwd)
	return []string{filepath.Join(home, ".claude", "projects", encoded, "*.jsonl")}
}

// Ensure ClaudeProvider implements all declared interfaces.
var _ Provider = (*ClaudeProvider)(nil)
var _ ModelLister = (*ClaudeProvider)(nil)
var _ ContainerCustomizer = (*ClaudeProvider)(nil)
var _ SessionCustomizer = (*ClaudeProvider)(nil)
var _ SessionResumer = (*ClaudeProvider)(nil)
var _ ActivitySource = (*ClaudeProvider)(nil)
