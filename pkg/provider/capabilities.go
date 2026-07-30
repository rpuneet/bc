package provider

import (
	"context"
	"time"
)

// Activity modes reported by ActivitySource.ActivityMode.
const (
	// ActivityModeHooks means the provider emits lifecycle events by
	// invoking configured hook commands (e.g. Claude Code hooks that POST
	// to bcd's /api/agents/{name}/hook endpoint).
	ActivityModeHooks = "hooks"
	// ActivityModeTranscript means the provider writes a session
	// transcript on disk that bcd tails to derive lifecycle events.
	ActivityModeTranscript = "transcript"
	// ActivityModeNone means the provider exposes no activity signal;
	// agent state is not updated from provider output.
	ActivityModeNone = "none"
)

// ActivitySource is optionally implemented by providers that expose a
// stream of agent activity (lifecycle/tool events). It tells bcd how to
// observe an agent driven by this provider: via push (hooks POSTing to the
// daemon) or via pull (tailing a transcript file).
type ActivitySource interface {
	// ActivityMode returns how activity is sourced for this provider:
	// ActivityModeHooks, ActivityModeTranscript, or ActivityModeNone.
	ActivityMode() string

	// WriteHookConfig writes the provider-specific hook configuration into
	// the agent's worktree so the provider reports lifecycle events to the
	// daemon. daemonAddr is the base URL of the bcd HTTP API and agentID is
	// the agent's identifier; implementations may ignore them when the
	// generated config resolves these at runtime (e.g. via environment
	// variables). Only meaningful when ActivityMode is ActivityModeHooks.
	WriteHookConfig(worktreeDir, daemonAddr, agentID string) error

	// TranscriptGlobs returns glob patterns matching the provider's session
	// transcript files for an agent whose working directory is cwd. Only
	// meaningful when ActivityMode is ActivityModeTranscript, though
	// hook-based providers may also expose transcripts for enrichment.
	TranscriptGlobs(cwd string) []string
}

// CostEntry is one usage record read from a provider's local session
// files (e.g. a Claude Code JSONL transcript line with token usage).
type CostEntry struct {
	// Timestamp is when the usage occurred.
	Timestamp time.Time
	// Agent is the mycel agent the usage is attributed to: the entity
	// name when the session belongs to an agent dir, otherwise a loose
	// session label (e.g. the working-dir basename).
	Agent string
	// Repo is the working directory the session ran in, used for
	// repo-level attribution. May be a container path for docker
	// agents and empty when the source doesn't record it.
	Repo string
	// Model is the provider model identifier.
	Model string
	// SessionID identifies the provider session the entry came from.
	SessionID string
	// Token counts. A "total" is never stored — it is always
	// input+output; cache tokens are reported separately.
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	// CostUSD is the provider-priced cost of this entry.
	CostUSD float64
}

// CostReadOptions tells a CostReader where to look for session files.
type CostReadOptions struct {
	// Since filters out entries older than this timestamp when non-zero.
	Since time.Time
	// Home is the user's home directory (host sessions, e.g.
	// ~/.claude/projects).
	Home string
	// AgentsDir is the mycel agent entity root (~/.mycel/agents).
	// Docker agents persist provider state under
	// <AgentsDir>/<name>/session/.
	AgentsDir string
}

// CostReader is optionally implemented by providers whose usage/cost
// data can be computed directly from local session files. Providers
// without documented session formats simply don't implement it — the
// UI shows what exists, no fakes.
type CostReader interface {
	ReadCosts(ctx context.Context, opts CostReadOptions) ([]CostEntry, error)
}

// Command describes a CLI command a provider exposes for UI surfaces.
type Command struct {
	// Name is the short display name (e.g. "mcp add").
	Name string
	// Command is the full invocation shape (e.g. "claude mcp add <name> <command>").
	Command string
	// Description is a one-line human-readable summary.
	Description string
	// Args describes expected arguments, empty when the command takes none.
	Args string
}

// CommandLister is optionally implemented by providers that expose a
// curated list of CLI commands. Providers without it get a generic
// run/version/help default from callers.
type CommandLister interface {
	Commands() []Command
}

// MCPServerInfo describes an MCP server configured for a provider.
type MCPServerInfo struct {
	Name      string
	Transport string
	// URL is set for remote (sse/http) transports.
	URL string
	// Command is set for stdio transports.
	Command string
	Enabled bool
}

// MCPConfigReader is optionally implemented by providers whose configured
// MCP servers can be read from the host (provider CLI or config files).
// rootDir is the workspace root; implementations must return an empty
// result rather than touching relative paths when it is empty.
type MCPConfigReader interface {
	ReadMCPs(ctx context.Context, rootDir string) []MCPServerInfo
}

// DynamicModelLister is optionally implemented by providers that can
// enumerate their available models at runtime (e.g. by querying an API or
// the provider CLI), as opposed to the static curated list returned by
// ModelLister.
type DynamicModelLister interface {
	ListModels(ctx context.Context) ([]string, error)
}
