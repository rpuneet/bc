// Package provider implements AI agent provider integrations.
package provider

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// Provider represents an AI agent provider that can run in a bc workspace.
type Provider interface {
	// Name returns the provider's unique identifier (e.g., "claude", "cursor")
	Name() string

	// Description returns a human-readable description
	Description() string

	// Command returns the shell command to start this provider
	Command() string

	// Binary returns the executable name for LookPath/version checks
	Binary() string

	// InstallHint returns a human-readable install instruction
	InstallHint() string

	// BuildCommand returns the full command for a given runtime context
	BuildCommand(opts CommandOpts) string

	// IsInstalled checks if the provider binary is available on the system
	IsInstalled(ctx context.Context) bool

	// Version returns the installed version, or empty string if not installed
	Version(ctx context.Context) string

	// DetectState analyzes output to determine agent state (working, idle, done, etc.)
	DetectState(output string) State
}

// CommandOpts configures how a provider builds its command.
type CommandOpts struct {
	AgentName string
	SessionID string
	// Model is the model identifier to pass to the provider CLI
	// (e.g. "fable" for claude, "gemini-2.5-pro" for gemini). Values
	// that fail SafeModelName are dropped, never escaped — the command
	// runs under `bash -c`.
	Model  string
	Docker bool
	Resume bool
}

// ModelLister is optionally implemented by providers that expose a
// curated list of model identifiers for UI pickers. An empty list means
// the provider has no model selection (e.g. pi).
type ModelLister interface {
	Models() []string
}

// ContainerCustomizer is optionally implemented by providers needing
// special Docker container behavior.
type ContainerCustomizer interface {
	// AdjustContainerCommand modifies the command for Docker execution.
	AdjustContainerCommand(command string) string
	// DockerImage returns custom image name, or empty for default convention.
	DockerImage() string
}

// SessionCustomizer is optionally implemented by providers that need to
// adjust their command for headless execution in tmux or Docker.
type SessionCustomizer interface {
	// AdjustSessionCommand modifies the command for native tmux sessions.
	AdjustSessionCommand(command string) string
	// AdjustContainerCommand modifies the command for Docker container execution.
	AdjustContainerCommand(command string) string
}

// SessionResumer is optionally implemented by providers that support resuming
// a specific named session by ID (e.g. claude --resume <id>).
type SessionResumer interface {
	// SupportsResume reports whether this provider can resume a specific session by ID.
	SupportsResume() bool
	// ParseSessionID extracts a session ID from tool output, returning "" if none found.
	// Claude prints "claude --resume <uuid>" on graceful exit.
	ParseSessionID(output string) string
}

// State represents the detected state of a provider's agent.
type State string

const (
	StateUnknown State = "unknown"
	StateIdle    State = "idle"
	StateWorking State = "working"
	StateDone    State = "done"
	StateError   State = "error"
	StateStuck   State = "stuck"
)

// Registry holds all registered providers.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates a new provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry.
func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

// Unregister removes a provider from the registry by name.
func (r *Registry) Unregister(name string) {
	delete(r.providers, name)
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// List returns all registered providers.
func (r *Registry) List() []Provider {
	providers := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	return providers
}

// Names returns the sorted names of all registered providers. User-facing
// provider lists (e.g., CLI flag help) derive from this so the registry
// stays the single source of truth.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListInstalled returns all installed providers.
func (r *Registry) ListInstalled(ctx context.Context) []Provider {
	var installed []Provider
	for _, p := range r.providers {
		if p.IsInstalled(ctx) {
			installed = append(installed, p)
		}
	}
	return installed
}

// DefaultRegistry is the global provider registry with all built-in providers.
var DefaultRegistry = NewRegistry()

func init() {
	// Register built-in providers.
	DefaultRegistry.Register(NewClaudeProvider())
	DefaultRegistry.Register(NewCodexProvider())
	DefaultRegistry.Register(NewGeminiProvider())
	DefaultRegistry.Register(NewCursorProvider())
	DefaultRegistry.Register(NewPiProvider())
}

// checkBinaryExists checks if a binary exists in PATH.
func checkBinaryExists(_ context.Context, name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// getBinaryVersion runs a command and returns the first line of output.
func getBinaryVersion(ctx context.Context, name string, args ...string) string {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // args are trusted provider names
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 0 {
		return lines[0]
	}
	return ""
}

// GetProvider returns a provider by name from the default registry.
func GetProvider(name string) (Provider, error) {
	p, ok := DefaultRegistry.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	return p, nil
}

// ListProviders returns all registered providers.
func ListProviders() []Provider {
	return DefaultRegistry.List()
}

// ListInstalledProviders returns all installed providers.
func ListInstalledProviders(ctx context.Context) []Provider {
	return DefaultRegistry.ListInstalled(ctx)
}

// safeSessionIDPattern is the conservative charset allowed in a session
// ID that gets spliced into a provider command line (which runs under
// `bash -c`). Quoting alone is not enough — `$()` expands inside double
// quotes — so anything outside this shape is dropped, not escaped. The
// first character must not be a dash, or the value would be parsed
// as another flag (argument injection).
var safeSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._-]*$`)

// SafeSessionID reports whether a session ID is safe to interpolate
// into a shell command line.
func SafeSessionID(id string) bool {
	return safeSessionIDPattern.MatchString(id)
}

// safeModelNamePattern is the conservative charset allowed in a model
// name that gets spliced into a provider command line (which runs under
// `bash -c`). Same approach as safeSessionIDPattern: anything outside
// this shape is dropped, not escaped. Colons and slashes are allowed
// for namespaced model IDs (pi's "provider/id[:thinking]" form,
// Bedrock-style "anthropic.claude-…:0"); the first character must not
// be a dash to prevent argument injection.
var safeModelNamePattern = regexp.MustCompile(`^[A-Za-z0-9._:/][A-Za-z0-9._:/-]*$`)

// SafeModelName reports whether a model name is safe to interpolate
// into a shell command line.
func SafeModelName(model string) bool {
	return safeModelNamePattern.MatchString(model)
}

// ResumableSessionDetector is optionally implemented by providers that
// can tell whether a working directory holds a prior session that a
// bare continue flag would pick up. Callers must not emit a "continue
// last session" flag when the detector reports false — some tools
// (Claude Code) exit instead of starting fresh.
type ResumableSessionDetector interface {
	HasResumableSession(dir string) bool
}
