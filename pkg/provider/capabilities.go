package provider

import "context"

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

// CostSource is optionally implemented by providers whose usage/cost data
// can be imported from local files. CostRoots returns the directories the
// cost importer should scan; home is the user's home directory and
// agentStateDir is the agent's bc state directory (.bc/agents/<name>).
type CostSource interface {
	CostRoots(home, agentStateDir string) []string
}

// DynamicModelLister is optionally implemented by providers that can
// enumerate their available models at runtime (e.g. by querying an API or
// the provider CLI), as opposed to the static curated list returned by
// ModelLister.
type DynamicModelLister interface {
	ListModels(ctx context.Context) ([]string, error)
}

// EnvContributor is optionally implemented by providers that need to inject
// additional environment variables into the agent session beyond the standard
// MYCEL_* and gateway vars. Typical use: AWS credential pointer vars for
// cloud-SDK providers (e.g. pi + AWS Bedrock). Implementations must NOT
// inject secret key material — only pointer values (profile names, region
// names) that direct the SDK to credentials stored on disk.
//
// ContributeEnv must be idempotent: check whether the key is already set
// before writing so that user-configured env always wins.
type EnvContributor interface {
	ContributeEnv(env map[string]string)
}
