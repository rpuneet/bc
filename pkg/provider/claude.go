package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// ClaudeProvider implements the Provider interface for Claude Code.
// Claude Code is the Anthropic CLI for Claude.
type ClaudeProvider struct { //nolint:govet // trailing cost cache grouped by role; padding is negligible for a singleton
	ClaudeConfigAdapter // embeds ConfigAdapter implementation
	name                string
	description         string
	command             string
	binary              string

	// costCache memoizes parsed session transcripts by file path so
	// repeated ReadCosts scans only re-read files whose mtime or size
	// changed. Without it, every cost query full-reparsed tens of
	// thousands of JSONL entries (~10s, ~1 full CPU core), which the
	// 60s TTL + UI polling turned into a sustained multi-core burn.
	costCacheMu sync.Mutex
	costCache   map[string]claudeFileCacheEntry
}

// claudeFileCacheEntry is a parsed transcript file plus the stat
// fingerprint that validates it. A file is re-parsed only when its
// mtime or size differs from the cached fingerprint.
type claudeFileCacheEntry struct {
	entries []claudeSessionEntry
	modTime int64 // UnixNano
	size    int64
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
// Includes --dangerously-skip-permissions. mycel manages worktrees itself and starts
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

// Commands returns the curated CLI command list for Claude Code.
func (p *ClaudeProvider) Commands() []Command {
	return []Command{
		{Name: "mcp add", Command: "claude mcp add <name> <command>", Description: "Add MCP server", Args: "<name> <command|url>"},
		{Name: "mcp list", Command: "claude mcp list", Description: "List MCP servers"},
		{Name: "mcp remove", Command: "claude mcp remove <name>", Description: "Remove MCP server", Args: "<name>"},
		{Name: "config set", Command: "claude config set <key> <value>", Description: "Set config value", Args: "<key> <value>"},
		{Name: "config list", Command: "claude config list", Description: "List config values"},
		{Name: "version", Command: "claude --version", Description: "Show version"},
		{Name: "help", Command: "claude --help", Description: "Show CLI help"},
		{Name: "resume", Command: "claude --resume <id>", Description: "Resume session", Args: "<session-id>", Interactive: true},
	}
}

// claudeSessionIDPattern is the full-string UUID shape of a Claude session
// ID. The ID is spliced into a shell command line, so anything else —
// including an empty string — is rejected rather than quoted.
var claudeSessionIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// AdjustSessionCommand is a no-op for native tmux sessions.
// Claude auto-detects the tmux environment when running inside a mycel-managed tmux session.
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

// claudeLookPath resolves the claude binary; overridable in tests to
// force the .mcp.json fallback regardless of the host environment.
var claudeLookPath = exec.LookPath

// ReadMCPs lists the MCP servers Claude Code sees for the repo at
// rootDir. `claude mcp list` (run in rootDir when non-empty) wins; the
// repo .mcp.json is the fallback. An empty rootDir means no
// repo is loaded, so the file fallback returns nothing.
func (p *ClaudeProvider) ReadMCPs(ctx context.Context, rootDir string) []MCPServerInfo {
	if servers := p.readMCPsViaCLI(ctx, rootDir); servers != nil {
		return servers
	}
	if rootDir == "" {
		return []MCPServerInfo{}
	}
	return readMCPJSONFile(filepath.Join(rootDir, ".mcp.json"))
}

// readMCPsViaCLI runs `claude mcp list` and parses its output. Returns
// nil (not empty) when the CLI is unavailable or fails, so the caller
// falls through to the file-based config.
func (p *ClaudeProvider) readMCPsViaCLI(ctx context.Context, rootDir string) []MCPServerInfo {
	claudePath, err := claudeLookPath("claude")
	if err != nil {
		return nil
	}

	cmd := exec.CommandContext(ctx, claudePath, "mcp", "list") //nolint:gosec // trusted binary
	if rootDir != "" {
		cmd.Dir = rootDir
	}
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseClaudeMCPList(string(output))
}

// parseClaudeMCPList parses `claude mcp list` text output where each
// line is "<name>: <type> <url/command>". Returns nil for output with
// no parseable lines.
func parseClaudeMCPList(output string) []MCPServerInfo {
	var servers []MCPServerInfo
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		sName := strings.TrimSpace(parts[0])
		rest := strings.TrimSpace(parts[1])

		s := MCPServerInfo{Name: sName, Enabled: true}
		switch {
		case strings.HasPrefix(rest, "sse"), strings.HasPrefix(rest, "SSE"):
			s.Transport = "sse"
			s.URL = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(rest, "sse"), "SSE"))
		case strings.HasPrefix(rest, "stdio"), strings.HasPrefix(rest, "STDIO"):
			s.Transport = "stdio"
			s.Command = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(rest, "stdio"), "STDIO"))
		default:
			s.Transport = "stdio"
			s.Command = rest
		}
		servers = append(servers, s)
	}
	return servers
}

// ActivityMode reports that Claude Code emits activity via lifecycle hooks
// (configured in .claude/settings.json) that POST to the daemon's hook endpoint.
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
var _ CommandLister = (*ClaudeProvider)(nil)
var _ MCPConfigReader = (*ClaudeProvider)(nil)
