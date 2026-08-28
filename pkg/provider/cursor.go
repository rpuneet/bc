package provider

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
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

func init() { Register(NewCursorProvider()) }

// NewCursorProvider creates a new Cursor provider.
//
// --trust: cursor-agent asks "do you trust the contents of this directory?" on
// first use of a workspace, and a tmux pane has nobody to answer it. The agent
// sat at that prompt indefinitely — reporting no events at all, since even
// SessionStart fires after trust is granted — which reads from the outside as an
// agent that started fine and a Live tab that doesn't work.
//
// Answering it is not a decision being taken away from anyone: mycel created the
// worktree it is pointing the agent at, moments earlier, from a repo the user
// had already adopted. The prompt exists for a directory of unknown provenance,
// which this never is.
func NewCursorProvider() *CursorProvider {
	return &CursorProvider{
		name:        "cursor",
		description: "Cursor Agent CLI",
		command:     "cursor-agent --trust",
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
// Resume priority: SessionID (--resume <id>) > Resume flag (--continue).
// cursor-agent accepts both; without either it starts a fresh chat.
// Model and session values are spliced into a shell command line, so
// unsafe values are dropped.
func (p *CursorProvider) BuildCommand(opts CommandOpts) string {
	cmd := p.command
	if SafeModelName(opts.Model) {
		cmd += " --model " + opts.Model
	}
	switch {
	case SafeSessionID(opts.SessionID):
		cmd += " --resume " + opts.SessionID
	case opts.Resume:
		cmd += " --continue"
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

// Models returns a small static fallback for Cursor Agent. Live catalogs
// come from ListModels (`cursor-agent --list-models`).
func (p *CursorProvider) Models() []string {
	return []string{"auto", "gpt-5.3-codex", "gpt-5.3-codex-high", "gpt-5.2", "composer-2.5"}
}

// cursorListModels is overridable in tests.
var cursorListModels = func(ctx context.Context) (string, error) {
	return runProviderCommand(ctx, "cursor-agent", "--list-models")
}

// ListModels returns model ids from `cursor-agent --list-models`.
func (p *CursorProvider) ListModels(ctx context.Context) ([]string, error) {
	out, err := cursorListModels(ctx)
	if err != nil {
		return p.Models(), nil
	}
	models := parseCursorListModels(out)
	if len(models) == 0 {
		return p.Models(), nil
	}
	return models, nil
}

// parseCursorListModels extracts ids from lines shaped like
// "gpt-5.3-codex - Codex 5.3" (id before the first " - ").
func parseCursorListModels(out string) []string {
	var models []string
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "Available models") {
			continue
		}
		id, _, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		id = strings.TrimSpace(id)
		if id == "" || !SafeModelName(id) || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, id)
	}
	return models
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

// cursorResumePattern matches cursor-agent's resume hint in pane/log output.
var cursorResumePattern = regexp.MustCompile(`(?:cursor-agent|agent)\s+--resume\s+([A-Za-z0-9._][A-Za-z0-9._-]*)`)

// cursorSessionJSONPattern matches "session_id":"<id>" in hook/usage JSON lines.
var cursorSessionJSONPattern = regexp.MustCompile(`"session_id"\s*:\s*"([A-Za-z0-9._][A-Za-z0-9._-]*)"`)

// SupportsResume reports that Cursor Agent can resume a chat by ID or via --continue.
func (p *CursorProvider) SupportsResume() bool { return true }

// ParseSessionID extracts a Cursor chat/session ID from tool output or JSONL.
func (p *CursorProvider) ParseSessionID(output string) string {
	if m := cursorResumePattern.FindStringSubmatch(output); len(m) == 2 && SafeSessionID(m[1]) {
		return m[1]
	}
	if m := cursorSessionJSONPattern.FindStringSubmatch(output); len(m) == 2 && SafeSessionID(m[1]) {
		return m[1]
	}
	return ""
}

// HasResumableSession reports whether Cursor has a prior chat that --continue
// can pick up. Cursor keys history by workspace; without a reliable on-disk
// probe from the worktree alone we stay permissive (true) so restart still
// attempts --continue. Prefer a stored SessionID when available.
func (p *CursorProvider) HasResumableSession(_ string) bool { return true }

// Ensure CursorProvider implements Provider interface.
var _ Provider = (*CursorProvider)(nil)
var _ ActivitySource = (*CursorProvider)(nil)
var _ ModelLister = (*CursorProvider)(nil)
var _ DynamicModelLister = (*CursorProvider)(nil)
var _ MCPConfigReader = (*CursorProvider)(nil)
var _ ContainerCustomizer = (*CursorProvider)(nil)
var _ SessionCustomizer = (*CursorProvider)(nil)
var _ SessionResumer = (*CursorProvider)(nil)
var _ ResumableSessionDetector = (*CursorProvider)(nil)
