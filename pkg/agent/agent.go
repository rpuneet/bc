// Package agent provides agent lifecycle management for mycel.
//
// An agent is an AI assistant running in an isolated tmux session with its own
// git worktree. Agents have roles (engineer, manager, etc.) that determine
// their capabilities and permissions.
//
// # Basic Usage
//
// Create an agent manager:
//
//	mgr := agent.NewManagerWithRepo(".mycel/agents", "/path/to/repo")
//	if err := mgr.LoadState(); err != nil {
//	    log.Warn("failed to load state", "error", err)
//	}
//
// List agents:
//
//	for _, a := range mgr.ListAgents() {
//	    fmt.Printf("%s: %s (%s)\n", a.Name, a.Role, a.State)
//	}
//
// Start an agent:
//
//	ag, err := mgr.Start(ctx, "eng-01", agent.Role("engineer"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # Roles and Capabilities
//
// Agents have roles that define their capabilities:
//
//	if agent.HasCapability(agent.Role("engineer"), agent.CapImplementTasks) {
//	    // Engineer can implement tasks
//	}
//
// Check if a role can create another:
//
//	if agent.CanCreateRole(agent.Role("manager"), agent.Role("engineer")) {
//	    // Manager can spawn engineers
//	}
//
// # States
//
// Agents transition through states: Idle -> Working -> Done/Error.
// State transitions are validated:
//
//	if err := agent.ValidateTransition(agent.StateIdle, agent.StateWorking); err != nil {
//	    log.Error("invalid transition", "error", err)
//	}
package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/mycel/pkg/app"
	// Register the built-in app plugins so descriptor-driven env injection
	// and prompt docs work in every binary that manages agents.
	_ "github.com/rpuneet/mycel/pkg/app/builtin"
	"github.com/rpuneet/mycel/pkg/container"
	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/names"
	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/pkg/runtime"
	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/tmux"
	"github.com/rpuneet/mycel/pkg/worktree"
)

// MaxAgentNameLength is the maximum allowed length for an agent name.
const MaxAgentNameLength = 64

// Default configuration constants.
const (
	// DefaultSessionPrefix is the tmux session / container name prefix
	// for mycel agents. Sourced from pkg/tmux so there is a single
	// source of truth.
	DefaultSessionPrefix = tmux.DefaultPrefix

	// DefaultProvider is the default AI provider for new agents.
	DefaultProvider = "claude"

	// DefaultMaxLogBytes is the maximum log file size in bytes before lazy truncation.
	DefaultMaxLogBytes = 10 * 1024 * 1024 // 10MB
)

// agentNameRe matches valid agent names: alphanumeric characters, hyphens,
// and underscores only (never path separators or ".."). Length is checked
// separately so MaxAgentNameLength stays a named constant.
var agentNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// IsValidAgentName validates that agent names contain only alphanumeric characters, hyphens, and underscores,
// and are at most MaxAgentNameLength characters long.
// This ensures agent names are safe for use in file paths, shell environments, and tmux sessions.
func IsValidAgentName(name string) bool {
	if len(name) > MaxAgentNameLength {
		return false
	}
	return agentNameRe.MatchString(name)
}

// envNameRe matches valid environment variable names: a letter or
// underscore followed by letters, digits, or underscores.
var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// IsValidEnvName reports whether name is a valid environment variable
// name (^[A-Za-z_][A-Za-z0-9_]*$). Used by the API layer to reject
// malformed keys in per-agent env configuration.
func IsValidEnvName(name string) bool {
	return envNameRe.MatchString(name)
}

// Role defines the type of agent.
type Role string

const (
	// RoleRoot is the only hardcoded role - a singleton root agent.
	// All other roles are defined in repo .mycel/roles/*.md files.
	RoleRoot Role = "root"
)

// Capability defines what actions a role can perform.
type Capability string

const (
	CapCreateAgents   Capability = "create_agents"   // Can spawn child agents
	CapAssignWork     Capability = "assign_work"     // Can assign work to others
	CapCreateEpics    Capability = "create_epics"    // Can create high-level epics
	CapImplementTasks Capability = "implement_tasks" // Can write code/implement
	CapReviewWork     Capability = "review_work"     // Can review others' work
	CapTestWork       Capability = "test_work"       // Can test and validate implementations
)

// Permission defines RBAC permissions for agent operations.
// Issue #1191: RBAC permissions for agent capabilities
type Permission string

const (
	// Agent lifecycle permissions
	PermCreateAgents  Permission = "can_create_agents"  // Can spawn new agents
	PermStopAgents    Permission = "can_stop_agents"    // Can stop running agents
	PermDeleteAgents  Permission = "can_delete_agents"  // Can permanently delete agents
	PermRestartAgents Permission = "can_restart_agents" // Can restart stopped agents

	// Communication permissions
	PermSendCommands Permission = "can_send_commands" // Can send commands to agents
	PermViewLogs     Permission = "can_view_logs"     // Can view agent logs/output

	// Configuration permissions
	PermModifyConfig Permission = "can_modify_config" // Can modify global config
	PermModifyRoles  Permission = "can_modify_roles"  // Can edit role definitions

	// Channel permissions
	PermCreateChannels Permission = "can_create_channels" // Can create new channels
	PermDeleteChannels Permission = "can_delete_channels" // Can delete channels
	PermSendMessages   Permission = "can_send_messages"   // Can send messages to channels
)

// AllPermissions lists all available permissions.
var AllPermissions = []Permission{
	PermCreateAgents,
	PermStopAgents,
	PermDeleteAgents,
	PermRestartAgents,
	PermSendCommands,
	PermViewLogs,
	PermModifyConfig,
	PermModifyRoles,
	PermCreateChannels,
	PermDeleteChannels,
	PermSendMessages,
}

// DefaultPermissions returns default permissions for a role level.
// Higher level roles (root, manager) have more permissions by default.
func DefaultPermissions(roleLevel int) []Permission {
	switch {
	case roleLevel <= -1:
		// Root level - all permissions
		return AllPermissions
	case roleLevel == 0:
		// Manager level
		return []Permission{
			PermCreateAgents,
			PermStopAgents,
			PermRestartAgents,
			PermSendCommands,
			PermViewLogs,
			PermCreateChannels,
			PermSendMessages,
		}
	default:
		// Engineer/worker level
		return []Permission{
			PermViewLogs,
			PermSendCommands,
			PermSendMessages,
		}
	}
}

// CheckPermission verifies an agent has the required permission.
// Returns nil if permitted, error otherwise.
func CheckPermission(permissions []string, required Permission) error {
	requiredStr := string(required)
	for _, p := range permissions {
		if p == requiredStr {
			return nil
		}
	}
	return fmt.Errorf("permission denied: %s required", required)
}

// HasPermissionStr checks if a permission string is in the list.
func HasPermissionStr(permissions []string, required string) bool {
	return slices.Contains(permissions, required)
}

// RoleCapabilities and RoleHierarchy are empty here.
// All role definitions (capabilities, hierarchy, metadata) are loaded from
// repo .mycel/roles/*.md files via RoleManager.
// Only the root role has hardcoded capabilities.
var RoleCapabilities = map[Role][]Capability{
	RoleRoot: {CapCreateAgents, CapAssignWork, CapCreateEpics, CapReviewWork}, // Root can do everything
}

var RoleHierarchy = map[Role][]Role{
	// Root can create any role defined in roles (checked at runtime)
	RoleRoot: {}, // Empty - all roles allowed, checked dynamically
}

// CanCreateRole checks if a parent role can create a child role.
func CanCreateRole(parent, child Role) bool {
	allowed, ok := RoleHierarchy[parent]
	if !ok {
		return false
	}
	for _, r := range allowed {
		if r == child {
			return true
		}
	}
	return false
}

// HasCapability checks if a role has a specific capability.
func HasCapability(role Role, cap Capability) bool {
	caps, ok := RoleCapabilities[role]
	if !ok {
		return false
	}
	for _, c := range caps {
		if c == cap {
			return true
		}
	}
	return false
}

// RoleLevel returns the hierarchy level for built-in roles.
// Custom roles loaded from .mycel/roles/*.md return level 1 by default.
func RoleLevel(role Role) int {
	switch role {
	case RoleRoot:
		return -1 // Root is at the top
	default:
		return 1 // All custom roles are at level 1
	}
}

// State represents the current state of an agent.
type State string

const (
	StateIdle     State = "idle"
	StateStarting State = "starting"
	StateWorking  State = "working"
	StateDone     State = "done"
	StateStuck    State = "stuck"
	StateError    State = "error"
	StateStopped  State = "stopped"
)

// validStates is the set of known agent states.
var validStates = map[State]bool{
	StateIdle:     true,
	StateStarting: true,
	StateWorking:  true,
	StateDone:     true,
	StateStuck:    true,
	StateError:    true,
	StateStopped:  true,
}

// IsValidState reports whether s is a known agent state.
func IsValidState(s string) bool {
	return validStates[State(s)]
}

// validTransitions defines allowed state transitions. Internal transitions
// (e.g. spawn setting starting→idle, stop setting →stopped) bypass this
// validation and set state directly. This map governs transitions through
// UpdateAgentState, which is called by mycel report.
var validTransitions = map[State][]State{
	StateStarting: {StateIdle, StateError, StateStopped},
	StateIdle:     {StateIdle, StateWorking, StateDone, StateStuck, StateError, StateStopped},
	StateWorking:  {StateWorking, StateIdle, StateDone, StateStuck, StateError, StateStopped},
	StateDone:     {StateIdle, StateWorking, StateStopped},
	StateStuck:    {StateStuck, StateIdle, StateWorking, StateError, StateStopped},
	StateError:    {StateIdle, StateWorking, StateStopped},
	StateStopped:  {StateIdle, StateStarting},
}

// ValidateTransition checks whether a state transition from → to is allowed.
// Returns an error if the transition is invalid.
func ValidateTransition(from, to State) error {
	allowed, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("unknown current state: %s", from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("invalid state transition: %s → %s", from, to)
}

// AgentMemory holds role-specific content loaded from prompts/<role>.md.
type AgentMemory struct {
	// LoadedAt is when the memory was loaded.
	LoadedAt time.Time `json:"loaded_at,omitempty"`
	// RolePrompt is the full content of the role's prompt file.
	RolePrompt string `json:"role_prompt,omitempty"`
	// Plugins lists Claude Code plugin names to install on agent start (#1959).
	Plugins []string `json:"plugins,omitempty"`
}

// Agent represents a running AI agent.
type Agent struct {
	UpdatedAt time.Time  `json:"updated_at"`
	StartedAt time.Time  `json:"started_at"`
	CreatedAt time.Time  `json:"created_at"`
	StoppedAt *time.Time `json:"stopped_at,omitempty"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	// ArchivedAt is set when the agent has been moved out of the
	// default listing via POST /api/agents/{name}/archive. A non-nil
	// value hides the agent from List() unless IncludeArchived is set.
	// Archiving does NOT delete state; unarchive clears this field.
	ArchivedAt *time.Time   `json:"archived_at,omitempty"`
	RolePrompt *AgentMemory `json:"memory,omitempty"`
	// Env holds user-configured environment variables injected into the
	// agent's session at spawn time. Values may contain ${secret:NAME}
	// references, which are stored verbatim and resolved against the
	// secrets vault only when the session is created — never persisted
	// or returned resolved.
	Env       map[string]string `json:"env,omitempty"`
	Workspace string            `json:"workspace"`
	Repo      string            `json:"repo,omitempty"`
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Task      string            `json:"task,omitempty"`
	Session   string            `json:"session"`
	SessionID string            `json:"session_id,omitempty"` // For session resume (#1939)
	Tool      string            `json:"tool,omitempty"`
	// Model is the provider model identifier the agent runs with (e.g.
	// "fable" for claude). Empty means the provider default — no model
	// flag is injected. Restarts reuse the stored value.
	Model          string `json:"model,omitempty"`
	ParentID       string `json:"parent_id,omitempty"`
	HookedWork     string `json:"hooked_work,omitempty"`
	WorktreeDir    string `json:"worktree_dir,omitempty"`
	LogFile        string `json:"log_file,omitempty"`
	Team           string `json:"team,omitempty"`
	RecoveredFrom  string `json:"recovered_from,omitempty"`
	EnvFile        string `json:"env_file,omitempty"`
	RuntimeBackend string `json:"runtime_backend,omitempty"`
	// Template is the name of the template this agent was spawned from
	// (empty when spawned without one). Guardrails read the template's
	// MaxCostUSD / StuckTimeoutMin by this name at check time rather than
	// copying the limits onto the agent row, so template edits apply to
	// already-running agents without a respawn.
	Template      string     `json:"template,omitempty"`
	LastCrashTime *time.Time `json:"last_crash_time,omitempty"`
	Role          Role       `json:"role"`
	State         State      `json:"state"`
	Children      []string   `json:"children,omitempty"`
	// CPUs is a per-agent Docker CPU cap (cores, e.g. 1.5). Zero means
	// inherit the fleet default (prefs runtime.docker.cpus). Enforced only
	// for the Docker runtime — tmux agents run unconstrained on the host.
	// Applied at the next session (re)start via the --cpus docker flag.
	CPUs float64 `json:"cpus,omitempty"`
	// MemoryMB is a per-agent Docker memory cap (MB). Zero means inherit
	// the fleet default (prefs runtime.docker.memory_mb). Docker-only,
	// like CPUs; applied at the next session (re)start via --memory.
	MemoryMB   int64 `json:"memory_mb,omitempty"`
	CrashCount int   `json:"crash_count,omitempty"`
	IsRoot     bool  `json:"is_root,omitempty"`
}

// HasCapability checks if this agent has a specific capability.
func (a *Agent) HasCapability(cap Capability) bool {
	return HasCapability(a.Role, cap)
}

// CanCreate checks if this agent can create an agent with the given role.
func (a *Agent) CanCreate(childRole Role) bool {
	return CanCreateRole(a.Role, childRole)
}

// IsLeaf returns true if this agent has no children.
func (a *Agent) IsLeaf() bool {
	return len(a.Children) == 0
}

// Level returns the hierarchy level of this agent.
func (a *Agent) Level() int {
	return RoleLevel(a.Role)
}

// LoadRoleMemory loads role-specific prompt content from the global
// role store. Returns nil AgentMemory when the role has neither a
// prompt nor plugins.
func LoadRoleMemory(_ string, role Role) *AgentMemory {
	// Load role via a RoleManager backed by the single global database —
	// roles are global, not per-repo state.
	rm, err := home.NewGlobalRoleManager(mycelHomeOrEmpty())
	if err != nil {
		log.Debug("failed to open global role store", "role", role, "error", err)
		return nil
	}
	roleObj, err := rm.LoadRole(string(role))
	if err != nil {
		log.Debug("failed to load role prompt", "role", role, "error", err)
		return nil
	}

	if roleObj.Prompt == "" && len(roleObj.Metadata.Plugins) == 0 {
		return nil
	}

	return &AgentMemory{
		RolePrompt: roleObj.Prompt,
		Plugins:    roleObj.Metadata.Plugins,
		LoadedAt:   time.Now(),
	}
}

// DefaultBootstrapDelay is the default time to wait before sending bootstrap
// prompts after starting an agent. Different AI tools have different startup
// times, so this can be configured per-manager.
const DefaultBootstrapDelay = 3 * time.Second

// Manager handles agent lifecycle.
type Manager struct { //nolint:govet // field order is intentional for readability; struct is a singleton
	agents           map[string]*Agent
	backends         map[string]runtime.Backend // keyed by "tmux", "docker"
	agentLocks       map[string]*sync.Mutex     // per-agent locks for slow I/O operations
	store            *SQLiteStore               // SQLite-backed agent persistence
	providerRegistry *provider.Registry

	// worktreeMgr manages per-agent git worktrees for isolation.
	worktreeMgr *worktree.Manager

	// onStateChange is called when an agent's state changes.
	// Set by AgentService to publish SSE events.
	onStateChange func(name string, state State, task string)

	// toolHealthCancel stops the background tool health check loop.
	toolHealthCancel context.CancelFunc

	// roleManager validates role existence (shared with the home)
	roleManager *home.RoleManager

	defaultBackend string // "tmux" or "docker"
	stateDir       string

	// Agent command (e.g., "claude" or "claude --dangerously-skip-permissions")
	agentCmd string

	// defaultTool is the provider name for the default agentCmd (for BuildCommand)
	defaultTool string

	// Repo path for env vars
	repoPath string

	// BootstrapDelay is the time to wait before sending bootstrap prompts.
	// If zero, DefaultBootstrapDelay is used.
	BootstrapDelay time.Duration

	// appsConfig holds connected app instances whose descriptor fields
	// drive credential env-var injection and prompt documentation.
	appsConfig map[string]app.InstanceConfig

	// wsConfig points at the live global config so spawn paths can read
	// mycel-authored injected instructions. It is the same pointer the
	// settings API mutates in place, so edits take effect on the next spawn.
	wsConfig *home.Config

	// providersConfig holds the global provider command overrides
	// (prefs.json `providers.<tool>.command`). Used by
	// getAgentCommand to layer a user-supplied command on top of the
	// provider's hardcoded BuildCommand — e.g. so `pi` can be pointed
	// at AWS Bedrock via global config.
	providersConfig *home.ProvidersConfig

	// maxLogBytes is the maximum log file size before truncation.
	// Defaults to DefaultMaxLogBytes; overridden by ApplyConfig.
	maxLogBytes int64

	mu           sync.RWMutex // protects maps (agents, agentLocks) only
	toolHealthMu sync.Mutex   // protects toolHealthCancel
}

// SetOnStateChange registers a callback invoked whenever an agent's state
// changes through hook-event processing.
func (m *Manager) SetOnStateChange(fn func(name string, state State, task string)) {
	m.mu.Lock()
	m.onStateChange = fn
	m.mu.Unlock()
}

// SetRoleManager sets the role manager used for role validation.
func (m *Manager) SetRoleManager(rm *home.RoleManager) {
	m.roleManager = rm
}

// ApplyConfig applies global configuration overrides to the manager.
// This should be called after creating a manager to pick up global settings.
func (m *Manager) ApplyConfig(cfg *home.Config) {
	if cfg == nil {
		return
	}
	if cfg.Logs.MaxBytes > 0 {
		m.maxLogBytes = cfg.Logs.MaxBytes
	}
	m.appsConfig = cfg.Apps
	m.providersConfig = &cfg.Providers
	m.wsConfig = cfg
}

// notifyStateChange calls the onStateChange callback if set.
// Caller must NOT hold m.mu — this method acquires RLock internally.
func (m *Manager) notifyStateChange(name string, state State, task string) {
	m.mu.RLock()
	fn := m.onStateChange
	m.mu.RUnlock()
	if fn != nil {
		fn(name, state, task)
	}
}

// getAgentLock returns the per-agent mutex, creating it if needed.
// Must be called while NOT holding mu (to avoid deadlock).
func (m *Manager) getAgentLock(name string) *sync.Mutex {
	m.mu.Lock()
	if m.agentLocks == nil {
		m.agentLocks = make(map[string]*sync.Mutex)
	}
	lock, ok := m.agentLocks[name]
	if !ok {
		lock = &sync.Mutex{}
		m.agentLocks[name] = lock
	}
	m.mu.Unlock()
	return lock
}

// rollbackCreate undoes a failed createAgent: the in-memory entry AND the
// store row both go. Background loops (tool health) snapshot m.agents to
// the store while creation is still in flight, so the row may already be
// persisted even though createAgent never saved it — without the store
// delete the name stays reserved by a phantom 'starting' row forever.
func (m *Manager) rollbackCreate(ctx context.Context, name string) {
	m.mu.Lock()
	delete(m.agents, name)
	store := m.store
	m.mu.Unlock()
	if store == nil {
		return
	}
	if err := store.Delete(ctx, name); err != nil {
		log.Warn("rollback: failed to delete agent row after failed create", "agent", name, "error", err)
	}
}

// runtime returns the default runtime backend.
func (m *Manager) runtime() runtime.Backend {
	return m.backends[m.defaultBackend]
}

// runtimeForAgent returns the appropriate runtime backend for an agent,
// based on the agent's stored RuntimeBackend. Falls back to the default.
func (m *Manager) runtimeForAgent(name string) runtime.Backend {
	if a, ok := m.agents[name]; ok && a.RuntimeBackend != "" {
		rt := normalizeRuntime(a.RuntimeBackend)
		if be, ok := m.backends[rt]; ok {
			return be
		}
	}
	return m.runtime()
}

// normalizeRuntime maps runtime aliases to canonical backend names.
// "localhost" → "tmux" (runs directly on host via tmux session)
func normalizeRuntime(rt string) string {
	switch rt {
	case "localhost", "local", "host":
		return "tmux"
	default:
		return rt
	}
}

// NewManager creates a new agent manager with repo-scoped tmux sessions.
func NewManager(stateDir string) *Manager {
	cmd, tool := defaultAgentCmd()
	tmuxBe := runtime.NewTmuxBackend(tmux.NewManager(DefaultSessionPrefix))
	return &Manager{
		agents:           make(map[string]*Agent),
		agentLocks:       make(map[string]*sync.Mutex),
		backends:         map[string]runtime.Backend{"tmux": tmuxBe},
		defaultBackend:   "tmux",
		providerRegistry: provider.DefaultRegistry,
		stateDir:         stateDir,
		agentCmd:         cmd,
		defaultTool:      tool,
		maxLogBytes:      DefaultMaxLogBytes,
	}
}

// NewManagerWithRepo creates an agent manager.
//
// stateDir is the agent entity root (~/.mycel/agents): each agent owns
// <stateDir>/<name>/ with worktree/, session/, logs/ and tmp/ inside.
// repoPath is the anchor repo new agents default to (may be "").
func NewManagerWithRepo(stateDir, repoPath string) *Manager {
	cmd, tool := defaultAgentCmd()
	tmuxBe := runtime.NewTmuxBackend(tmux.NewManagerWithRepo(DefaultSessionPrefix, repoPath))
	return &Manager{
		agents:           make(map[string]*Agent),
		agentLocks:       make(map[string]*sync.Mutex),
		backends:         map[string]runtime.Backend{"tmux": tmuxBe},
		defaultBackend:   "tmux",
		providerRegistry: provider.DefaultRegistry,
		stateDir:         stateDir,
		agentCmd:         cmd,
		defaultTool:      tool,
		repoPath:         repoPath,
		maxLogBytes:      DefaultMaxLogBytes,
		worktreeMgr:      entityWorktreeManager(repoPath, stateDir),
	}
}

// NewManagerWithRuntime creates an agent manager with a specific runtime backend.
// rtName should be "docker" or "tmux".
func NewManagerWithRuntime(stateDir, repoPath string, rt runtime.Backend, rtName string) *Manager {
	cmd, tool := defaultAgentCmd()
	bes := map[string]runtime.Backend{rtName: rt}
	// Always register a tmux backend so agents with RuntimeBackend="tmux" work
	if rtName != "tmux" {
		bes["tmux"] = runtime.NewTmuxBackend(tmux.NewManagerWithRepo(DefaultSessionPrefix, repoPath))
	}
	return &Manager{
		agents:           make(map[string]*Agent),
		agentLocks:       make(map[string]*sync.Mutex),
		backends:         bes,
		defaultBackend:   rtName,
		providerRegistry: provider.DefaultRegistry,
		stateDir:         stateDir,
		agentCmd:         cmd,
		defaultTool:      tool,
		repoPath:         repoPath,
		maxLogBytes:      DefaultMaxLogBytes,
		worktreeMgr:      entityWorktreeManager(repoPath, stateDir),
	}
}

// entityWorktreeManager builds a worktree manager for the entity-scoped
// layout: everything an agent owns lives at <agentsRoot>/<name>/
// (worktree/, session/, logs/, tmp/). agentsRoot is normally
// ~/.mycel/agents; when empty it is resolved from the mycel home.
func entityWorktreeManager(repoRoot, agentsRoot string) *worktree.Manager {
	if agentsRoot == "" {
		if dir, err := home.AgentsDir(); err == nil {
			agentsRoot = dir
		} else {
			log.Warn("cannot resolve mycel agents dir", "error", err)
		}
	}
	return worktree.NewManager(repoRoot, agentsRoot)
}

// agentsRoot returns the agent entity root this manager operates on:
// its configured stateDir, or ~/.mycel/agents when unset.
func (m *Manager) agentsRoot() string {
	if m.stateDir != "" {
		return m.stateDir
	}
	if dir, err := home.AgentsDir(); err == nil {
		return dir
	}
	return ""
}

// worktreeManagerFor returns the worktree manager to use when spawning
// an agent bound to repo. The boot repo (or an empty repo) uses the
// shared manager. Any other repo gets a manager anchored at THAT repo
// with the same data-dir layout, so the agent's worktree is checked out
// from the repo it is bound to — not from the boot repo.
//
// A repo that is not a git repository falls back to the shared manager:
// older agent rows may carry stale/synthetic repo values, and the
// boot repo is the only safe source for them.
func (m *Manager) worktreeManagerFor(repo string) *worktree.Manager {
	if repo == "" || filepath.Clean(repo) == filepath.Clean(m.repoPath) {
		return m.worktreeMgr
	}
	// The repo value arrives from the create-agent API — clean it and
	// require an absolute, traversal-free path before it touches the
	// filesystem or anchors a worktree manager.
	repo = filepath.Clean(repo)
	if !filepath.IsAbs(repo) || strings.Contains(repo, "..") {
		log.Warn("agent repo path is not absolute/safe — using boot repo for worktree", "repo", repo)
		return m.worktreeMgr
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		log.Warn("agent repo is not a git repository — using boot repo for worktree", "repo", repo)
		return m.worktreeMgr
	}
	// Cross-repo managers share the same flat dirs — only repoRoot differs.
	return entityWorktreeManager(repo, m.stateDir)
}

// seedHostClaudeTrust pre-trusts an agent's worktree in the claude.json a
// host (tmux) claude agent will read — $HOME/.claude.json — so fresh
// agents don't hang at Claude Code's interactive "trust this folder"
// prompt. Docker agents are seeded separately with the container-side
// path by the container backend.
func seedHostClaudeTrust(tool, worktreeDir string) {
	if tool != "claude" || worktreeDir == "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Warn("cannot resolve home dir for claude trust seeding", "error", err)
		return
	}
	if err := container.SeedClaudeTrust(filepath.Join(home, ".claude.json"), worktreeDir); err != nil {
		log.Warn("failed to seed claude trust", "worktree", worktreeDir, "error", err)
	}
}

// storedWorktreeExists reports whether an existing agent's worktree is
// present on disk. The stored worktreeDir (DB column) wins — agents
// created under older layouts keep working from their old paths until
// migrated. Only when no dir is recorded does the manager-computed path
// serve as fallback.
func storedWorktreeExists(worktreeDir string, wtMgr *worktree.Manager, name string) bool {
	if worktreeDir != "" {
		_, err := os.Stat(worktreeDir)
		return err == nil
	}
	return wtMgr.Exists(name)
}

// mycelHomeOrEmpty resolves the mycel home directory, returning ""
// when it cannot be determined. Used by package-level helpers that do
// not have access to a *Home.
func mycelHomeOrEmpty() string {
	home, err := home.MycelHome()
	if err != nil {
		log.Warn("cannot resolve mycel home", "error", err)
		return ""
	}
	return home
}

// defaultAgentCmd returns the command and tool name for the default provider.
func defaultAgentCmd() (string, string) {
	name := DefaultProvider
	if name == "" {
		return "", ""
	}
	p, ok := provider.DefaultRegistry.Get(name)
	if !ok {
		return "", ""
	}
	return p.Command(), name
}

// getAgentCommand looks up the command for a tool from the manager's
// provider registry, layering a global override on top when
// prefs.json defines one. The override path is
// what makes a user able to spawn pi against AWS Bedrock by writing:
//
//	providers:
//	  pi:
//	    command: pi --provider amazon-bedrock --model anthropic.claude-…
//
// Resolution order: global ProvidersConfig command (if non-empty)
// → provider's own BuildCommand. Session flags from the provider
// (--continue / --session) are appended even when the global config
// overrides the base command so resume still works.
// SessionID takes priority over the resume flag when non-empty.
func (m *Manager) getAgentCommand(toolName, agentName string, resume bool, sessionID, model string) (string, bool) {
	if m.providerRegistry == nil {
		return "", false
	}
	p, ok := m.providerRegistry.Get(toolName)
	if !ok {
		return "", false
	}
	opts := provider.CommandOpts{
		AgentName: agentName,
		Resume:    resume,
		SessionID: sessionID,
		Model:     model,
	}
	// Apply global command override when present. We rebuild
	// the session flags via the provider so we don't lose --continue
	// or --session "<id>" handling.
	if m.providersConfig != nil {
		if cfg, ok := m.providersConfig.Providers[toolName]; ok && cfg.Command != "" {
			return appendSessionFlags(cfg.Command, opts), true
		}
	}
	return p.BuildCommand(opts), true
}

// appendSessionFlags adds the same --continue / --session / --model
// flags the provider's BuildCommand would, but to an arbitrary base
// command. We keep the logic in one place so config overrides
// cooperate with resume and model selection cleanly. Both values land
// in a `bash -c` line, where quoting alone can't stop `$()` expansion —
// unsafe values are dropped. The model flag is a generic --model here
// because the override command is arbitrary and we can't consult the
// provider's own flag spelling.
func appendSessionFlags(base string, opts provider.CommandOpts) string {
	cmd := base
	if provider.SafeModelName(opts.Model) {
		cmd += " --model " + opts.Model
	}
	// Mirror provider priority: an explicit session wins over --continue.
	if provider.SafeSessionID(opts.SessionID) {
		cmd += " --session " + opts.SessionID
	} else if opts.Resume {
		cmd += " --continue"
	}
	return cmd
}

// toolHasResumableSession consults the tool's provider about whether the
// working directory holds a session a bare continue flag would pick up.
// Providers without a detector keep the permissive default.
func (m *Manager) toolHasResumableSession(toolName, dir string) bool {
	if m.providerRegistry == nil {
		return true
	}
	p, ok := m.providerRegistry.Get(toolName)
	if !ok {
		return true
	}
	det, ok := p.(provider.ResumableSessionDetector)
	if !ok {
		return true
	}
	return det.HasResumableSession(dir)
}

// listAvailableTools returns tool names from the manager's provider registry.
func (m *Manager) listAvailableTools() []string {
	if m.providerRegistry == nil {
		return nil
	}
	providers := m.providerRegistry.List()
	tools := make([]string, 0, len(providers))
	for _, p := range providers {
		tools = append(tools, p.Name())
	}
	return tools
}

// SetAgentCommand sets the command to run for agents.
func (m *Manager) SetAgentCommand(cmd string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agentCmd = cmd
}

// SetAgentByName sets the agent command by looking up the provider name in the registry.
func (m *Manager) SetAgentByName(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.providerRegistry.Get(name)
	if !ok {
		return false
	}
	m.agentCmd = p.Command()
	m.defaultTool = name
	return true
}

// writeActivityConfig writes the provider's activity configuration (hook
// settings) into the agent worktree. Providers that implement
// provider.ActivitySource in hooks mode own their config; everything else
// falls back to the Claude hook settings — today every agent gets Claude
// hook settings regardless of tool (claude-compatible sessions), and this
// refactor preserves that behavior exactly. Per-provider activity sources
// land in follow-up PRs.
func (m *Manager) writeActivityConfig(toolName, wtDir, agentName string) error {
	if toolName != "" && m.providerRegistry != nil {
		if p, ok := m.providerRegistry.Get(toolName); ok {
			if src, ok := p.(provider.ActivitySource); ok && src.ActivityMode() == provider.ActivityModeHooks {
				return src.WriteHookConfig(wtDir, "", agentName)
			}
		}
	}
	return provider.WriteClaudeHookSettings(wtDir)
}

// SetBootstrapDelay sets the delay before sending bootstrap prompts.
func (m *Manager) SetBootstrapDelay(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BootstrapDelay = d
}

// getBootstrapDelay returns the configured bootstrap delay or the default.
func (m *Manager) getBootstrapDelay() time.Duration {
	if m.BootstrapDelay > 0 {
		return m.BootstrapDelay
	}
	return DefaultBootstrapDelay
}

// GetAgentCommand returns the command for a tool name from the provider registry.
// Returns the command and true if found, or empty string and false if not.
func GetAgentCommand(toolName string) (string, bool) {
	p, ok := provider.DefaultRegistry.Get(toolName)
	if !ok {
		return "", false
	}
	return p.Command(), true
}

// GetAgentCommandFromConfig returns the command for a tool name,
// checking the global ProvidersConfig first, then falling back to global config.
// This enables per-provider tool customization.
func GetAgentCommandFromConfig(toolName string, homeCfg *home.Config) (string, bool) {
	// Check the global ProvidersConfig first
	if homeCfg != nil {
		if p := homeCfg.GetProvider(toolName); p != nil && p.Command != "" {
			return p.Command, true
		}
	}
	// Fall back to global config
	return GetAgentCommand(toolName)
}

// ListAvailableTools returns a list of configured tool names from the provider registry.
func ListAvailableTools() []string {
	providers := provider.DefaultRegistry.List()
	tools := make([]string, 0, len(providers))
	for _, p := range providers {
		tools = append(tools, p.Name())
	}
	return tools
}

// SpawnOptions holds all parameters for creating an agent.
type SpawnOptions struct {
	// Env holds user-configured environment variables. Values may contain
	// ${secret:NAME} references resolved against the vault at spawn time.
	Env       map[string]string
	Name      string
	Role      Role
	Workspace string
	ParentID  string
	Tool      string
	Model     string // provider model identifier; empty uses the provider default
	EnvFile   string
	Runtime   string // override runtime backend ("tmux" or "docker"); empty uses manager default
	Team      string // optional team assignment
	SessionID string // Explicit session ID to resume (overrides stored session_id)
	// Template is the name of the template this agent is spawned from.
	// Recorded on the agent row so the guardrail loop can look up
	// MaxCostUSD / StuckTimeoutMin at check time. Empty disables guardrails.
	Template string
}

// SpawnAgent creates and starts a new agent.
// Idempotent: if the agent already exists and its tmux session is alive, reuse it.
func (m *Manager) SpawnAgent(ctx context.Context, name string, role Role, repo string) (*Agent, error) {
	return m.SpawnAgentWithOptions(ctx, SpawnOptions{Name: name, Role: role, Workspace: repo})
}

// SpawnAgentWithTool creates and starts a new agent with a specific tool.
// If tool is empty, uses the manager's default agent command.
func (m *Manager) SpawnAgentWithTool(ctx context.Context, name string, role Role, repo string, tool string) (*Agent, error) {
	return m.SpawnAgentWithOptions(ctx, SpawnOptions{Name: name, Role: role, Workspace: repo, Tool: tool})
}

// SpawnAgentWithParent creates and starts a new agent with a parent relationship.
// Idempotent: if the agent already exists and its tmux session is alive, reuse it.
func (m *Manager) SpawnAgentWithParent(ctx context.Context, name string, role Role, repo string, parentID string) (*Agent, error) {
	return m.SpawnAgentWithOptions(ctx, SpawnOptions{Name: name, Role: role, Workspace: repo, ParentID: parentID})
}

// SpawnAgentWithOptions creates and starts a new agent with all options.
// If tool is empty, uses the manager's default agent command.
// Idempotent: if the agent already exists and its tmux session is alive, reuse it.
func (m *Manager) SpawnAgentWithOptions(ctx context.Context, opts SpawnOptions) (*Agent, error) {
	name := opts.Name
	role := opts.Role
	repoPath := opts.Workspace
	parentID := opts.ParentID

	m.mu.Lock()

	// Auto-generate name if empty. Agent names are globally unique
	// (name is the primary key of the global agents table), so seed the
	// collision set with every name in the store, not just this
	// manager's agents.
	if name == "" {
		existing := make(map[string]bool, len(m.agents))
		for n := range m.agents {
			existing[n] = true
		}
		if m.store != nil {
			if global, namesErr := m.store.LoadNames(ctx); namesErr == nil {
				for n := range global {
					existing[n] = true
				}
			}
		}
		generated, genErr := names.GenerateUnique(existing, 100)
		if genErr != nil {
			m.mu.Unlock()
			return nil, fmt.Errorf("failed to generate agent name: %w", genErr)
		}
		name = generated
		opts.Name = name
	}

	log.Debug("spawning agent", "name", name, "role", role, "repo", repoPath, "parentID", parentID, "tool", opts.Tool)

	// Validate agent name format
	if !IsValidAgentName(name) {
		m.mu.Unlock()
		return nil, fmt.Errorf("agent name %q is invalid: use letters, numbers, dash, underscore (max %d chars)", name, MaxAgentNameLength)
	}

	// Validate role is not empty or null-like
	if role == "" || role == "null" || role == "<nil>" {
		m.mu.Unlock()
		return nil, fmt.Errorf("role is required and cannot be empty or null")
	}

	// Validate role exists. Skip validation if no role manager is available
	// (e.g., standalone agent manager without a home). Built-in roles
	// like "root" are always valid.
	if role != RoleRoot && m.roleManager != nil {
		if !m.roleManager.HasRole(string(role)) {
			m.mu.Unlock()
			return nil, fmt.Errorf("role %q does not exist; create it via the API or in .mycel/roles/%s.md", role, role)
		}
	}

	// Enforce root singleton constraint
	if role == RoleRoot {
		if err := m.enforceRootSingleton(repoPath); err != nil {
			m.mu.Unlock()
			return nil, err
		}
	}

	// Validate parent relationship if specified
	if parentID != "" {
		parent, exists := m.agents[parentID]
		if !exists {
			m.mu.Unlock()
			return nil, fmt.Errorf("parent agent %s not found", parentID)
		}
		if !CanCreateRole(parent.Role, role) {
			m.mu.Unlock()
			return nil, fmt.Errorf("agent %s (role %s) cannot create child with role %s", parentID, parent.Role, role)
		}
	}

	// Check if already exists in our state
	if existing, exists := m.agents[name]; exists {
		// If its tmux session is still alive, reuse it
		if m.runtimeForAgent(name).HasSession(ctx, name) {
			// Correct stale stopped/error state when session is actually alive
			if existing.State == StateStopped || existing.State == StateError {
				existing.State = StateIdle
				existing.StartedAt = time.Now()
			}
			existing.UpdatedAt = time.Now()
			if err := m.saveState(ctx); err != nil {
				log.Warn("failed to save agent state", "error", err)
			}
			m.mu.Unlock()
			return existing, nil
		}
		// Agent exists but session is dead — restart it.
		// Release global lock; startAgent handles its own locking.
		m.mu.Unlock()
		return m.startAgent(ctx, name, opts)
	}

	// Global uniqueness: the name is not in this manager's state, but it
	// may belong to an agent from another repo — the agents table is
	// global and keyed by name, so a blind create would silently
	// overwrite that row. Reject with a pointer to the owner.
	if err := m.checkNameAvailable(ctx, name); err != nil {
		m.mu.Unlock()
		return nil, err
	}

	// Fresh create — release global lock; createAgent handles its own locking.
	m.mu.Unlock()
	return m.createAgent(ctx, opts)
}

// startAgent restarts an existing agent whose session has died.
// Acquires per-agent lock internally for slow I/O; does NOT require caller to hold mu.
func (m *Manager) startAgent(ctx context.Context, name string, opts SpawnOptions) (*Agent, error) {
	// Phase 1: global lock — read agent state and build command config
	m.mu.Lock()
	existing := m.agents[name]
	// Restarts happen in the repo the agent is bound to, never the
	// caller's boot repo — opts.Workspace only fills in when the row
	// has no repo recorded.
	repoPath := existing.Repo
	if repoPath == "" {
		repoPath = opts.Workspace
	}
	wtMgr := m.worktreeManagerFor(repoPath)

	if opts.Runtime != "" {
		existing.RuntimeBackend = normalizeRuntime(opts.Runtime)
	}

	sessionID := existing.SessionID
	if opts.SessionID != "" {
		sessionID = opts.SessionID
		existing.SessionID = sessionID
	}
	// Resume if a session ID exists (auto-continue previous session)
	isRealSessionID := len(sessionID) == 36 && sessionID[8] == '-'
	resume := isRealSessionID

	// Check if existing worktree can be reused for resume (tmux only).
	// Existing agents keep working from their stored WorktreeDir (which
	// may predate the current flat layout) — only fall back to the
	// manager-computed path when no dir is recorded.
	agentRuntime := existing.RuntimeBackend
	if storedWorktreeExists(existing.WorktreeDir, wtMgr, name) && agentRuntime == "tmux" {
		// Worktree exists — check for active session conflict
		for beName, be := range m.backends {
			if be.HasSession(ctx, name) {
				m.mu.Unlock()
				return nil, fmt.Errorf("worktree for %s is already in use by active session on %s backend", name, beName)
			}
		}
		// Worktree exists with no active session — enable resume, but
		// only when the tool actually has a session to continue: some
		// tools (Claude Code) exit instead of starting fresh when the
		// continue flag finds nothing.
		if !resume && sessionID == "" {
			wtPath := existing.WorktreeDir
			if wtPath == "" {
				wtPath = wtMgr.Path(name)
			}
			// Resolve the tool the same way command construction does —
			// an empty Tool falls back to the default, and passing ""
			// to the detector would skip the check entirely.
			resumeTool := existing.Tool
			if resumeTool == "" {
				resumeTool = m.defaultTool
			}
			if m.toolHasResumableSession(resumeTool, wtPath) {
				resume = true
				log.Debug("prior session found, will use --continue", "agent", name)
			} else {
				log.Debug("no prior session transcript — starting fresh", "agent", name, "worktree", wtPath)
			}
		}
	}

	toolName := existing.Tool
	if toolName == "" {
		toolName = m.defaultTool
	}
	agentCmd := m.agentCmd
	if toolName != "" {
		// Restarts use the STORED model so the agent keeps running on
		// the model it was created with.
		if cmd, ok := m.getAgentCommand(toolName, name, resume, sessionID, existing.Model); ok {
			agentCmd = cmd
		}
	}

	// Docker: wrap command in tmux session inside the container so SendKeys works.
	if agentRuntime != "tmux" {
		if toolName != "" && m.providerRegistry != nil {
			if p, ok := m.providerRegistry.Get(toolName); ok {
				if sc, ok := p.(provider.SessionCustomizer); ok {
					agentCmd = sc.AdjustContainerCommand(agentCmd)
				}
			}
		}
	}

	// The worktree label is the agent name — entity dirs are keyed by
	// it, so the in-container tmux session name stays stable.
	worktreeName := name
	env := map[string]string{
		"MYCEL_AGENT_ID":      name,
		"MYCEL_AGENT_ROLE":    string(existing.Role),
		"MYCEL_WORKSPACE":     repoPath,
		"MYCEL_AGENT_RUNTIME": agentRuntime,
		"MYCEL_DAEMON_ADDR":   daemonAddrForRuntime(agentRuntime),
		"MYCEL_WORKTREE_NAME": worktreeName,
	}
	if toolName != "" {
		env["MYCEL_AGENT_TOOL"] = toolName
	}
	if existing.ParentID != "" {
		env["MYCEL_PARENT_ID"] = existing.ParentID
	}
	// Pass through MYCEL_API_KEY from the host environment so agents inside
	// containers can authenticate back to the daemon when --api-key is enabled.
	if apiKey := os.Getenv("MYCEL_API_KEY"); apiKey != "" {
		env["MYCEL_API_KEY"] = apiKey
	}
	injectResourceLimits(env, existing.CPUs, existing.MemoryMB)
	injectEnv(env, repoPath, name, existing.EnvFile, existing.Env)
	injectAppEnv(env, m.appsConfig)
	secretEnvKeys := injectVaultSecrets(env, repoPath, resolveRoleSecrets(repoPath, string(existing.Role)), m.appsConfig)

	rt := m.runtimeForAgent(name)
	m.mu.Unlock()

	// Phase 2: per-agent lock — slow I/O (create session, pipe-pane)
	agentLock := m.getAgentLock(name)
	agentLock.Lock()

	// Ensure worktree exists and is valid (may have been cleaned up, moved,
	// or corrupted by runtime changes like Docker→localhost migration).
	// The stored WorktreeDir is authoritative — never recompute it from
	// the manager for an existing agent (pre-migration agents live at
	// older paths). The stat checks below decide whether it is usable.
	wtDir := existing.WorktreeDir
	needsRecreate := wtDir == ""

	// Also check that the worktree has a valid .git reference
	if !needsRecreate && wtDir != "" {
		gitPath := filepath.Join(wtDir, ".git")
		if _, statErr := os.Stat(gitPath); statErr != nil {
			log.Warn("worktree .git missing, will recreate", "agent", name, "path", wtDir)
			needsRecreate = true
		}
	}

	// Check for stale Docker paths (e.g., /workspace/... when running locally)
	if !needsRecreate && wtDir != "" && !filepath.IsAbs(wtDir) {
		needsRecreate = true
	}
	if !needsRecreate && wtDir != "" {
		if _, statErr := os.Stat(wtDir); statErr != nil {
			log.Warn("worktree path inaccessible, will recreate", "agent", name, "path", wtDir, "error", statErr)
			needsRecreate = true
		}
	}

	if needsRecreate {
		// Remove stale worktree if it exists
		_ = wtMgr.Remove(ctx, name) //nolint:errcheck
		var wtErr error
		wtDir, wtErr = wtMgr.Create(ctx, name)
		if wtErr != nil {
			agentLock.Unlock()
			return nil, fmt.Errorf("failed to create worktree for agent %s: %w", name, wtErr)
		}
		existing.WorktreeDir = wtDir
		log.Info("worktree recreated", "agent", name, "path", wtDir)
	}

	// Pre-trust the worktree so a restarted tmux claude agent doesn't
	// hang at the interactive trust prompt.
	if agentRuntime == "tmux" {
		seedHostClaudeTrust(toolName, wtDir)
	}

	// Write hook settings and role files to worktree (regenerate on every start
	// so config changes like MCP URLs take effect without manual intervention).
	if err := m.writeActivityConfig(toolName, wtDir, name); err != nil {
		log.Error("failed to write hook settings", "dir", wtDir, "error", err)
	}
	if setupErr := SetupAgentFromRoleWithRuntime(ctx, repoPath, name, string(existing.Role), wtDir, agentRuntime, existing.Tool); setupErr != nil {
		log.Warn("role setup failed on restart", "agent", name, "error", setupErr)
	}
	appendAppPrompt(wtDir, existing.Tool, m.appsConfig)
	if err := appendInjectedInstructions(ctx, injectedPromptFile(wtDir, existing.Tool), m.wsConfig,
		resolveRoleMCPServers(repoPath, string(existing.Role)), secretEnvKeys); err != nil {
		log.Warn("failed to append injected instructions", "agent", name, "error", err)
	}

	if err := rt.CreateSessionWithEnv(ctx, name, wtDir, agentCmd, env); err != nil {
		agentLock.Unlock()
		return nil, fmt.Errorf("failed to recreate session: %w", err)
	}

	// Resume log streaming
	if existing.LogFile != "" {
		truncateLogFile(existing.LogFile, m.maxLogBytes)
		if pipeErr := rt.PipePane(ctx, name, existing.LogFile); pipeErr != nil {
			log.Warn("failed to resume pipe-pane", "agent", name, "error", pipeErr)
		}
	} else {
		existing.LogFile = m.setupLogPipe(ctx, name, repoPath)
	}

	if existing.State == StateStopped || existing.State == StateError {
		existing.State = StateStarting
		// Clear the task so the UI doesn't render the previous session's
		// stale task next to a fresh "starting" badge. Lifecycle progress
		// ("Starting…") is conveyed by State, never by Task — Task holds the
		// prompt the agent is working on, and it has not been given one yet.
		existing.Task = ""
	}
	existing.UpdatedAt = time.Now()

	agentLock.Unlock()

	// Phase 3: global lock — persist state
	m.mu.Lock()
	if err := m.saveState(ctx); err != nil {
		log.Warn("failed to save agent state", "error", err)
	}
	m.mu.Unlock()

	return existing, nil
}

// createAgent creates a brand-new agent and its runtime session.
// Acquires per-agent lock internally for slow I/O; does NOT require caller to hold mu.
func (m *Manager) createAgent(ctx context.Context, opts SpawnOptions) (*Agent, error) {
	name := opts.Name
	role := opts.Role
	repoPath := opts.Workspace
	parentID := opts.ParentID
	tool := opts.Tool
	// Worktrees come from the repo the agent is bound to, not the boot repo.
	wtMgr := m.worktreeManagerFor(repoPath)

	// Phase 1: global lock — build command config, register agent in map
	m.mu.Lock()

	// If a session exists from a previous crash, kill it in all backends
	for beName, be := range m.backends {
		if be.HasSession(ctx, name) {
			log.Debug("killing stale session", "session", name, "backend", beName)
			if err := be.KillSession(ctx, name); err != nil {
				log.Warn("failed to kill existing session", "session", name, "backend", beName, "error", err)
			}
		}
	}

	// Resolve effective tool: use explicit tool or fall back to default.
	// Persist the resolved value so restarts use the same tool.
	effectiveTool := tool
	if effectiveTool == "" {
		effectiveTool = m.defaultTool
	}

	// Determine runtime backend for this agent
	agentRuntime := m.defaultBackend
	if opts.Runtime != "" {
		agentRuntime = normalizeRuntime(opts.Runtime)
	}

	// Determine the command to use
	agentCmd := m.agentCmd
	if effectiveTool != "" {
		if cmd, ok := m.getAgentCommand(effectiveTool, name, false, "", opts.Model); ok {
			agentCmd = cmd
		} else if tool != "" {
			m.mu.Unlock()
			return nil, fmt.Errorf("unknown tool %q, available tools: %v", tool, m.listAvailableTools())
		}
	}

	// Docker: wrap command in tmux session inside the container so SendKeys works.
	if agentRuntime != "tmux" {
		if effectiveTool != "" && m.providerRegistry != nil {
			if p, ok := m.providerRegistry.Get(effectiveTool); ok {
				if sc, ok := p.(provider.SessionCustomizer); ok {
					agentCmd = sc.AdjustContainerCommand(agentCmd)
				}
			}
		}
	}

	// Validate tool binary exists before spawning.
	// Skip for Docker runtime — the tool is inside the agent image, not on the daemon host.
	providerValidated := false
	if agentRuntime == "docker" {
		providerValidated = true // tool lives in the agent container image
	} else if effectiveTool != "" && m.providerRegistry != nil {
		if p, ok := m.providerRegistry.Get(effectiveTool); ok {
			if !p.IsInstalled(ctx) {
				m.mu.Unlock()
				return nil, fmt.Errorf("tool %q is not installed. Install %s or configure a different tool in settings.json", effectiveTool, p.Name())
			}
			if v := p.Version(ctx); v != "" {
				log.Debug("provider validated", "tool", effectiveTool, "version", v)
			}
			providerValidated = true
		}
	}

	if !providerValidated && agentCmd != "" {
		parts := strings.Fields(agentCmd)
		if len(parts) > 0 {
			if _, err := exec.LookPath(parts[0]); err != nil {
				m.mu.Unlock()
				return nil, fmt.Errorf("tool %q command %q not found in PATH. Install it or configure a different tool in settings.json", effectiveTool, parts[0])
			}
		}
	}
	log.Debug("agent runtime selected", "agent", name, "runtime", agentRuntime, "default", m.defaultBackend, "override", opts.Runtime)

	// Create agent
	now := time.Now()
	agent := &Agent{
		ID:             name,
		Name:           name,
		Role:           role,
		State:          StateStarting,
		Workspace:      repoPath,
		Repo:           cleanRepoPath(repoPath),
		Session:        name,
		Tool:           effectiveTool,
		Model:          opts.Model,
		ParentID:       parentID,
		Team:           opts.Team,
		EnvFile:        opts.EnvFile,
		Env:            opts.Env,
		RuntimeBackend: agentRuntime,
		Children:       []string{},
		IsRoot:         role == RoleRoot,
		Template:       opts.Template,
		CreatedAt:      now,
		StartedAt:      now,
		UpdatedAt:      now,
	}

	// Register agent early so runtimeForAgent can resolve the correct backend
	m.agents[name] = agent

	// Build env vars so the spawned process sees them immediately
	env := map[string]string{
		"MYCEL_AGENT_ID":      name,
		"MYCEL_AGENT_ROLE":    string(role),
		"MYCEL_WORKSPACE":     repoPath,
		"MYCEL_AGENT_RUNTIME": agentRuntime,
		"MYCEL_DAEMON_ADDR":   daemonAddrForRuntime(agentRuntime),
		"MYCEL_WORKTREE_NAME": name,
	}
	if effectiveTool != "" {
		env["MYCEL_AGENT_TOOL"] = effectiveTool
	}
	if parentID != "" {
		env["MYCEL_PARENT_ID"] = parentID
	}
	// Pass through MYCEL_API_KEY from the host environment so agents inside
	// containers can authenticate back to the daemon when --api-key is enabled.
	if apiKey := os.Getenv("MYCEL_API_KEY"); apiKey != "" {
		env["MYCEL_API_KEY"] = apiKey
	}
	injectResourceLimits(env, agent.CPUs, agent.MemoryMB)
	injectEnv(env, repoPath, name, opts.EnvFile, opts.Env)
	injectAppEnv(env, m.appsConfig)
	secretEnvKeys := injectVaultSecrets(env, repoPath, resolveRoleSecrets(repoPath, string(role)), m.appsConfig)

	rt := m.runtimeForAgent(name)
	m.mu.Unlock()

	// Phase 2: per-agent lock — slow I/O (create session, role setup, log pipe)
	agentLock := m.getAgentLock(name)
	agentLock.Lock()

	// Create worktree for this agent — from the agent's repo (wtMgr),
	// which may differ from the boot repo.
	wtDir, wtErr := wtMgr.Create(ctx, name)
	if wtErr != nil {
		agentLock.Unlock()
		m.rollbackCreate(ctx, name)
		return nil, fmt.Errorf("create worktree: %w", wtErr)
	}
	agent.WorktreeDir = wtDir

	// Ensure Claude home dir exists
	if claudeErr := wtMgr.EnsureClaudeDir(name); claudeErr != nil {
		log.Warn("failed to ensure Claude dir", "agent", name, "error", claudeErr)
	}

	// Pre-trust the worktree so a fresh tmux claude agent doesn't hang
	// at the interactive trust prompt (docker agents are seeded with the
	// container-side path by the container backend).
	if agentRuntime == "tmux" {
		seedHostClaudeTrust(effectiveTool, wtDir)
	}

	// Write hook settings to the worktree
	if err := m.writeActivityConfig(effectiveTool, wtDir, name); err != nil {
		log.Warn("failed to write hook settings", "dir", wtDir, "error", err)
	}

	// Write role files (prompt, MCP, rules, etc.) to the worktree using provider adapter
	if setupErr := SetupAgentFromRoleWithRuntime(ctx, repoPath, name, string(role), wtDir, agentRuntime, effectiveTool); setupErr != nil {
		log.Warn("role setup failed", "agent", name, "error", setupErr)
		agent.Task = fmt.Sprintf("role setup failed: %v", setupErr)
	}

	// Append platform credential instructions to the agent's prompt file
	appendAppPrompt(wtDir, effectiveTool, m.appsConfig)
	if err := appendInjectedInstructions(ctx, injectedPromptFile(wtDir, effectiveTool), m.wsConfig,
		resolveRoleMCPServers(repoPath, string(role)), secretEnvKeys); err != nil {
		log.Warn("failed to append injected instructions", "agent", name, "error", err)
	}

	// Validate required tools before starting — fail fast with clear errors.
	if toolErrs := validateAgentTools(repoPath, string(role)); len(toolErrs) > 0 {
		for _, te := range toolErrs {
			log.Warn("tool validation failed", "agent", name, "error", te)
		}
		agent.Task = fmt.Sprintf("tool validation: %d issue(s)", len(toolErrs))
		// Non-fatal: agent starts but issues are logged for visibility
	}

	// Create session IN the worktree directory
	if err := rt.CreateSessionWithEnv(ctx, name, wtDir, agentCmd, env); err != nil {
		agentLock.Unlock()
		m.rollbackCreate(ctx, name)
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Start log streaming via pipe-pane
	agent.LogFile = m.setupLogPipe(ctx, name, repoPath)

	// Update state
	agent.State = StateIdle
	agent.UpdatedAt = time.Now()

	agentLock.Unlock()

	// Phase 3: global lock — update parent, persist
	m.mu.Lock()
	if parentID != "" {
		if parent, exists := m.agents[parentID]; exists {
			parent.Children = append(parent.Children, name)
			parent.UpdatedAt = time.Now()
		}
	}

	// Save state
	if err := m.saveState(ctx); err != nil {
		log.Warn("failed to save agent state", "error", err)
	}
	m.mu.Unlock()

	return agent, nil
}

// setupLogPipe creates the logs directory and starts pipe-pane for the agent.
// Returns the log file path.
func (m *Manager) setupLogPipe(ctx context.Context, name, _ string) string {
	// The agent name becomes a filename below — never let a crafted
	// name escape the logs directory.
	if !IsValidAgentName(name) {
		log.Warn("refusing to pipe logs for unsafe agent name", "agent", name)
		return ""
	}
	// Logs live with the rest of the agent's entity state at
	// <agentsRoot>/<name>/logs/. filepath.Base strips any path
	// separators from the (already-validated) name so it can never
	// escape the agents root — a recognized path-traversal barrier.
	safeName := filepath.Base(name)
	logsDir := filepath.Join(m.agentsRoot(), safeName, "logs")
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		log.Warn("failed to create logs dir", "error", err)
		return ""
	}

	logPath := filepath.Clean(filepath.Join(logsDir, safeName+".log"))
	if !strings.HasPrefix(logPath, filepath.Clean(m.agentsRoot())+string(filepath.Separator)) {
		log.Warn("refusing log path outside agents root", "path", logPath)
		return ""
	}

	// Truncate if over max size
	truncateLogFile(logPath, m.maxLogBytes)

	m.mu.RLock()
	rt := m.runtimeForAgent(name)
	m.mu.RUnlock()

	if err := rt.PipePane(ctx, name, logPath); err != nil {
		log.Warn("failed to start pipe-pane", "agent", name, "error", err)
		return ""
	}

	log.Debug("started log streaming", "agent", name, "path", logPath)
	return logPath
}

// truncateLogFile truncates a log file if it exceeds maxBytes.
// Keeps the last half of the file to preserve recent output.
func truncateLogFile(path string, maxBytes int64) {
	if maxBytes <= 0 {
		return
	}
	// Defense in depth: log paths are built from validated agent names,
	// but reject traversal segments so this can never touch files
	// outside the agent log directory.
	path = filepath.Clean(path)
	if strings.Contains(path, "..") {
		log.Warn("refusing to truncate log with traversal path", "path", path)
		return
	}

	info, err := os.Stat(path)
	if err != nil || info.Size() <= maxBytes {
		return
	}

	data, err := os.ReadFile(path) //nolint:gosec // path constructed from trusted repo root
	if err != nil {
		log.Warn("failed to read log for truncation", "path", path, "error", err)
		return
	}

	// Keep last half
	half := len(data) / 2
	// Find next newline to avoid cutting mid-line
	for half < len(data) && data[half] != '\n' {
		half++
	}
	if half < len(data) {
		half++ // skip the newline
	}

	if err := os.WriteFile(path, data[half:], 0600); err != nil { //nolint:gosec // path constructed from trusted repo root
		log.Warn("failed to truncate log", "path", path, "error", err)
	}
}

// SpawnChildAgent creates a child agent under a parent agent.
// Validates that the parent has permission to create the child role.
func (m *Manager) SpawnChildAgent(ctx context.Context, parentID, childName string, childRole Role, repo string) (*Agent, error) {
	return m.SpawnAgentWithOptions(ctx, SpawnOptions{Name: childName, Role: childRole, Workspace: repo, ParentID: parentID})
}

// SpawnChildAgentWithTool creates a child agent under a parent agent with a specific tool.
// Validates that the parent has permission to create the child role.
func (m *Manager) SpawnChildAgentWithTool(ctx context.Context, parentID, childName string, childRole Role, repo, tool string) (*Agent, error) {
	return m.SpawnAgentWithOptions(ctx, SpawnOptions{Name: childName, Role: childRole, Workspace: repo, ParentID: parentID, Tool: tool})
}

// removeFromParent removes an agent from its parent's children list.
// Must be called while holding the lock.
func (m *Manager) removeFromParent(name string) {
	agent, exists := m.agents[name]
	if !exists || agent.ParentID == "" {
		return
	}

	parent, exists := m.agents[agent.ParentID]
	if !exists {
		return
	}

	// Remove from parent's children
	newChildren := make([]string, 0, len(parent.Children))
	for _, childID := range parent.Children {
		if childID != name {
			newChildren = append(newChildren, childID)
		}
	}
	parent.Children = newChildren
	parent.UpdatedAt = time.Now()
}

// captureSessionIDForAgent extracts a session ID from the agent's output.
// Does NOT require holding mu — caller provides the agent and runtime directly.
func (m *Manager) captureSessionIDForAgent(ctx context.Context, ag *Agent, rt runtime.Backend) string {
	toolName := ag.Tool
	if toolName == "" {
		toolName = m.defaultTool
	}
	if m.providerRegistry == nil {
		return ""
	}
	p, ok := m.providerRegistry.Get(toolName)
	if !ok {
		return ""
	}
	sr, ok := p.(provider.SessionResumer)
	if !ok || !sr.SupportsResume() {
		return ""
	}

	// Read from log file first; fall back to runtime capture.
	var output string
	if ag.LogFile != "" {
		data, err := os.ReadFile(ag.LogFile) //nolint:gosec // trusted path
		if err == nil {
			output = string(data)
		}
	}
	if output == "" {
		var captureErr error
		output, captureErr = rt.Capture(ctx, ag.Name, 100)
		if captureErr != nil {
			log.Debug("failed to capture pane for session ID", "agent", ag.Name, "error", captureErr)
			return ""
		}
	}

	if id := sr.ParseSessionID(output); id != "" {
		return id
	}

	// Fallback: read session ID from the most recent JSONL transcript filename.
	// Claude Code writes transcripts to .mycel/agents/<name>/claude/projects/*/<uuid>.jsonl
	// where the UUID IS the session ID.
	if id := findSessionIDFromTranscripts(m.agentsRoot(), ag.Name); id != "" {
		log.Debug("captured session ID from JSONL transcript", "agent", ag.Name, "session_id", id)
		return id
	}

	return ""
}

// findSessionIDFromTranscripts scans the agent's Claude projects directory
// (<agentsRoot>/<name>/session/claude/projects) for the most recent .jsonl
// transcript and extracts the session ID from the filename (which is the
// UUID session ID).
func findSessionIDFromTranscripts(agentsRoot, agentName string) string {
	projectsDir := filepath.Join(agentsRoot, agentName, "session", "claude", "projects")
	if _, err := os.Stat(projectsDir); err != nil {
		return ""
	}

	var newestFile string
	var newestTime time.Time

	_ = filepath.WalkDir(projectsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") || d.Name() == "history.jsonl" {
			return nil
		}
		// Skip subagent transcripts
		if strings.Contains(path, "/subagents/") {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestFile = d.Name()
		}
		return nil
	})

	if newestFile == "" {
		return ""
	}

	// Extract UUID from filename: "<uuid>.jsonl" → "<uuid>"
	id := strings.TrimSuffix(newestFile, ".jsonl")
	// Validate it looks like a UUID (36 chars, hyphens at positions 8,13,18,23)
	if len(id) == 36 && id[8] == '-' && id[13] == '-' && id[18] == '-' && id[23] == '-' {
		return id
	}
	return ""
}

// writeSessionIDFile persists the session ID to a plain-text file under
// the agent's session dir (<agentsRoot>/<name>/session/) and archives it
// in the session history directory alongside a timestamp.
// Permissions are 0600 (session IDs may grant conversation access).
func writeSessionIDFile(agentsRoot, agentName, sessionID string) {
	// Agent names are validated at creation, but never allow a name to
	// escape the agents directory via path separators or "..".
	if !IsValidAgentName(agentName) {
		log.Warn("refusing to write session_id for unsafe agent name", "agent", agentName)
		return
	}
	agentsRoot = filepath.Clean(agentsRoot)
	if strings.Contains(agentsRoot, "..") || !filepath.IsLocal(agentName) {
		log.Warn("refusing to write session_id for unsafe path", "agent", agentName)
		return
	}
	agentDir := filepath.Join(agentsRoot, agentName, "session")
	if err := os.MkdirAll(agentDir, 0750); err != nil {
		log.Warn("failed to create agent dir for session_id", "error", err)
		return
	}

	sessionFile := filepath.Join(agentDir, "session_id")
	if err := os.WriteFile(sessionFile, []byte(sessionID+"\n"), 0600); err != nil {
		log.Warn("failed to write session_id file", "agent", agentName, "error", err)
		return
	}

	// Archive to session_history/ with a timestamp name.
	histDir := filepath.Join(agentDir, "session_history")
	if err := os.MkdirAll(histDir, 0750); err != nil {
		return
	}
	stamp := time.Now().UTC().Format("2006-01-02T15:04:05")
	histFile := filepath.Join(histDir, stamp+".txt")
	_ = os.WriteFile(histFile, []byte(sessionID+"\n"), 0600) //nolint:errcheck // best-effort history
}

// StopAgent stops an agent.
func (m *Manager) StopAgent(ctx context.Context, name string) error {
	log.Debug("stopping agent", "name", name)

	// Phase 1: global lock — validate agent exists, get references
	m.mu.RLock()
	agent, exists := m.agents[name]
	if !exists {
		m.mu.RUnlock()
		log.Warn("agent not found", "name", name)
		return fmt.Errorf("agent %s: %w", name, ErrNotFound)
	}
	rt := m.runtimeForAgent(name)
	agentsRoot := m.agentsRoot()
	m.mu.RUnlock()

	// Phase 2: per-agent lock — slow I/O (capture session ID, kill session)
	agentLock := m.getAgentLock(name)
	agentLock.Lock()

	// Capture session ID from output before killing the session.
	if sessionID := m.captureSessionIDForAgent(ctx, agent, rt); sessionID != "" {
		agent.SessionID = sessionID
		writeSessionIDFile(agentsRoot, name, sessionID)
		log.Debug("captured session ID on stop", "agent", name, "session_id", sessionID)
	}

	// Kill tmux session (ignore error - session might already be dead)
	_ = rt.KillSession(ctx, name)

	now := time.Now()
	agent.State = StateStopped
	agent.StoppedAt = &now
	agent.UpdatedAt = now

	agentLock.Unlock()

	// Phase 3: global lock — update parent, persist
	m.mu.Lock()
	m.removeFromParent(name)
	if err := m.saveState(ctx); err != nil {
		log.Warn("failed to save agent state", "error", err)
	}
	m.mu.Unlock()

	return nil
}

// agentTreeEntry holds pre-collected data for stopping an agent in a tree.
type agentTreeEntry struct {
	agent *Agent
	rt    runtime.Backend
	name  string
}

// StopAgentTree stops an agent and all its children recursively.
func (m *Manager) StopAgentTree(ctx context.Context, name string) error {
	log.Debug("stopping agent tree", "name", name)

	// Phase 1: global read-lock — collect all agents in the tree and their backends
	m.mu.RLock()
	entries, err := m.collectAgentTree(name)
	m.mu.RUnlock()
	if err != nil {
		return err
	}

	// Phase 2: no lock — slow I/O (kill sessions for all agents in the tree)
	for _, e := range entries {
		_ = e.rt.KillSession(ctx, e.name) //nolint:errcheck // session might already be dead
	}

	// Phase 3: global lock — update state for all agents, persist
	m.mu.Lock()
	now := time.Now()
	for _, e := range entries {
		e.agent.State = StateStopped
		e.agent.StoppedAt = &now
		e.agent.UpdatedAt = now
		e.agent.Children = []string{} // Clear children since they're stopped
	}
	if err := m.saveState(ctx); err != nil {
		log.Warn("failed to save agent state after tree stop", "error", err)
	}
	m.mu.Unlock()

	return nil
}

// collectAgentTree collects all agents in a tree depth-first. Must be called with m.mu held.
func (m *Manager) collectAgentTree(name string) ([]agentTreeEntry, error) {
	agent, exists := m.agents[name]
	if !exists {
		return nil, fmt.Errorf("agent %s: %w", name, ErrNotFound)
	}

	var entries []agentTreeEntry
	// Collect children first (depth-first)
	for _, childID := range agent.Children {
		childEntries, err := m.collectAgentTree(childID)
		if err != nil {
			continue // skip missing children
		}
		entries = append(entries, childEntries...)
	}
	// Then the agent itself
	entries = append(entries, agentTreeEntry{
		name:  name,
		agent: agent,
		rt:    m.runtimeForAgent(name),
	})
	return entries, nil
}

// DeleteOptions configures agent deletion behavior.
type DeleteOptions struct {
	// Placeholder for future options.
	Force bool
}

// DeleteAgent permanently removes an agent from the fleet.
func (m *Manager) DeleteAgent(ctx context.Context, name string) error {
	return m.DeleteAgentWithOptions(ctx, name, DeleteOptions{})
}

// DeleteAgentWithOptions permanently removes an agent with configurable options.
// Cleans up all resources: container, volume, worktree, git branch, log file,
// agent state directory, channel memberships, and child agent references.
// Partial failures are logged but do not abort the deletion.
func (m *Manager) DeleteAgentWithOptions(ctx context.Context, name string, opts DeleteOptions) error {
	log.Debug("deleting agent", "name", name)

	// Deletion removes directories derived from the name — never let a
	// crafted name (e.g. "../..") reach os.RemoveAll.
	if !IsValidAgentName(name) {
		return fmt.Errorf("invalid agent name %q", name)
	}

	// Phase 1: global lock — validate agent exists, snapshot references
	m.mu.RLock()
	agent, exists := m.agents[name]
	if !exists {
		m.mu.RUnlock()
		return fmt.Errorf("agent %s: %w", name, ErrNotFound)
	}
	rt := m.runtimeForAgent(name)
	agentsRoot := m.agentsRoot()
	logFile := agent.LogFile
	m.mu.RUnlock()

	// Deletion recursively removes directories built from this path —
	// never let a traversal sequence reach os.RemoveAll.
	agentsRoot = filepath.Clean(agentsRoot)
	if strings.Contains(agentsRoot, "..") {
		return fmt.Errorf("refusing to delete agent %s: unsafe agents root", name)
	}

	// Phase 2: per-agent lock — slow I/O (kill session, remove container, git cleanup)
	agentLock := m.getAgentLock(name)
	agentLock.Lock()

	// 1. Stop the container/session
	_ = rt.KillSession(ctx, name) //nolint:errcheck // may already be stopped

	// 2. Remove the container entirely (for Docker agents)
	if cb, ok := rt.(*container.Backend); ok {
		_ = cb.RemoveSession(ctx, name) //nolint:errcheck // may not exist
	}

	// 3. Remove git worktree — via the repo the agent is bound to, so
	// cross-repo agents unregister from THEIR repo's worktree list. The
	// stored WorktreeDir wins over the manager-computed path so deletion
	// targets the directory the agent actually used.
	wtMgr := m.worktreeManagerFor(agent.Repo)
	if agent.WorktreeDir != "" {
		if err := wtMgr.RemoveAt(ctx, name, agent.WorktreeDir); err != nil {
			log.Warn("failed to remove worktree", "agent", name, "error", err)
		}
	} else if err := wtMgr.Remove(ctx, name); err != nil {
		log.Warn("failed to remove worktree", "agent", name, "error", err)
	}

	// 4. Remove log file when it lives outside the entity dir (the
	// entity dir itself is removed wholesale below).
	if logFile != "" {
		if err := os.Remove(logFile); err != nil && !os.IsNotExist(err) {
			log.Warn("delete: failed to remove log file", "agent", name, "path", logFile, "error", err)
		}
	}

	// 5. Remove the agent's entity directory (<agentsRoot>/<name>/ —
	// worktree leftovers, session state, logs, tmp). agentsRoot is
	// cleaned and the name regexp-validated above; re-verify the
	// composed path before the recursive delete.
	if agentsRoot != "" {
		entityDir := filepath.Clean(filepath.Join(agentsRoot, name))
		if strings.Contains(entityDir, "..") {
			log.Warn("delete: refusing unsafe agent entity path", "agent", name, "path", entityDir)
		} else if err := os.RemoveAll(entityDir); err != nil {
			log.Warn("delete: failed to remove agent entity dir", "agent", name, "path", entityDir, "error", err)
		}
	}

	agentLock.Unlock()

	// Phase 3: global lock — update maps, orphan children, persist
	m.mu.Lock()

	// 8. Update children's ParentID to "" (orphan them cleanly)
	for _, childName := range agent.Children {
		if child, ok := m.agents[childName]; ok {
			child.ParentID = ""
			child.UpdatedAt = time.Now()
		}
	}

	// 9. Remove from parent's children list
	m.removeFromParent(name)

	// 10. Soft-delete in SQLite first (set deleted_at) so the agent won't be
	// resurrected by LoadAll even if the daemon crashes before the hard delete.
	if m.store != nil {
		if err := m.store.SoftDelete(ctx, name); err != nil {
			log.Warn("delete: failed to soft-delete agent in store", "agent", name, "error", err)
		}
	}

	// 11. Delete from state map and clean up per-agent lock
	delete(m.agents, name)
	delete(m.agentLocks, name)

	if err := m.saveState(ctx); err != nil {
		log.Warn("delete: failed to save state", "agent", name, "error", err)
	}

	// 12. Hard-delete the row from SQLite. The soft-delete above already
	// prevents resurrection; this removes the row entirely for cleanliness.
	if m.store != nil {
		if err := m.store.Delete(ctx, name); err != nil {
			log.Warn("delete: failed to remove agent from store", "agent", name, "error", err)
		}
	}
	m.mu.Unlock()

	log.Debug("agent fully deleted", "agent", name)
	return nil
}

// RenameAgent renames an agent from oldName to newName.
func (m *Manager) RenameAgent(ctx context.Context, oldName, newName string) error {
	if !IsValidAgentName(oldName) {
		return fmt.Errorf("agent name %q is invalid: use letters, numbers, dash, underscore (max %d chars)", oldName, MaxAgentNameLength)
	}
	if !IsValidAgentName(newName) {
		return fmt.Errorf("agent name %q is invalid: use letters, numbers, dash, underscore (max %d chars)", newName, MaxAgentNameLength)
	}
	// Renaming moves directories derived from these names — never let a
	// traversal sequence reach the filesystem operations below.
	if !filepath.IsLocal(oldName) || !filepath.IsLocal(newName) {
		return fmt.Errorf("invalid agent name")
	}

	// Phase 1: validate under global lock, snapshot agent
	m.mu.Lock()
	agent, exists := m.agents[oldName]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("agent %s: %w", oldName, ErrNotFound)
	}
	if _, newExists := m.agents[newName]; newExists {
		m.mu.Unlock()
		return fmt.Errorf("agent %s already exists", newName)
	}
	// Agent must be stopped — rename while running is unsafe
	if agent.State != StateStopped && agent.State != StateError {
		m.mu.Unlock()
		return fmt.Errorf("agent %q must be stopped before renaming (state: %s): %w", oldName, agent.State, ErrInvalidState)
	}
	rt := m.runtimeForAgent(oldName)
	m.mu.Unlock()

	// Snapshot and sanitize the paths the rename moves — never let a
	// traversal sequence reach the os.Rename/os.MkdirAll calls below.
	repoPath := filepath.Clean(m.repoPath)
	agentsRoot := filepath.Clean(m.agentsRoot())
	if strings.Contains(repoPath, "..") || strings.Contains(agentsRoot, "..") {
		return fmt.Errorf("rename: unsafe repo paths")
	}
	oldEntityDir := filepath.Join(agentsRoot, oldName)
	newEntityDir := filepath.Join(agentsRoot, newName)
	// The stored WorktreeDir wins for the source when it lives outside
	// the entity dir; the destination always uses the current layout.
	newPath := m.worktreeMgr.Path(newName)
	if strings.Contains(newPath, "..") {
		return fmt.Errorf("rename: unsafe worktree paths")
	}

	// Phase 2: slow I/O under per-agent lock
	agentLock := m.getAgentLock(oldName)
	agentLock.Lock()

	log.Debug("renaming agent", "oldName", oldName, "newName", newName)

	// Rename runtime session (tmux rename-session / docker rename)
	if err := rt.RenameSession(ctx, oldName, newName); err != nil {
		log.Warn("rename: failed to rename runtime session", "error", err)
		// Non-fatal — session may already be dead (agent is stopped)
	}

	// Move the whole entity dir (worktree/, session/, logs/, tmp/) in
	// one rename, then re-create the worktree if the move failed.
	newWorktreeDir := ""
	if err := os.Rename(oldEntityDir, newEntityDir); err != nil {
		log.Warn("rename: failed to move agent entity dir", "error", err)
		// Fall back: drop the old worktree and create a fresh one.
		_ = m.worktreeMgr.Remove(ctx, oldName)
		newPath2, wtErr := m.worktreeMgr.Create(ctx, newName)
		if wtErr != nil {
			log.Warn("rename: failed to create worktree for new name", "error", wtErr)
		}
		if newPath2 != "" {
			newWorktreeDir = newPath2
		}
	} else {
		newWorktreeDir = newPath
	}

	// Regenerate role files (CLAUDE.md, .mcp.json) with the new agent name.
	if newWorktreeDir != "" && agent.Role != "" {
		agentRuntime := agent.RuntimeBackend
		if agentRuntime == "" {
			agentRuntime = "tmux"
		}
		if setupErr := SetupAgentFromRoleWithRuntime(ctx, repoPath, newName, string(agent.Role), newWorktreeDir, agentRuntime, agent.Tool); setupErr != nil {
			log.Warn("rename: failed to regenerate role files", "agent", newName, "error", setupErr)
		}
	}

	// Rename the log file inside the moved entity dir, tracking the
	// agent's recorded log path.
	oldLogFile := filepath.Join(oldEntityDir, "logs", oldName+".log")
	newLogFile := filepath.Join(newEntityDir, "logs", newName+".log")
	movedLog := filepath.Join(newEntityDir, "logs", oldName+".log")
	if err := os.Rename(movedLog, newLogFile); err != nil && !os.IsNotExist(err) {
		log.Warn("rename: failed to rename log file", "error", err)
	}

	agentLock.Unlock()

	// Phase 3: update maps + persist under global lock
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	agent.ID = newName
	agent.Name = newName
	agent.Session = newName
	agent.UpdatedAt = now
	if newWorktreeDir != "" {
		agent.WorktreeDir = newWorktreeDir
	}
	if agent.LogFile == oldLogFile {
		agent.LogFile = newLogFile
	}

	// Update maps
	delete(m.agents, oldName)
	m.agents[newName] = agent

	// Move per-agent lock entry
	delete(m.agentLocks, oldName)

	// Update parent's children list
	if agent.ParentID != "" {
		if parent, ok := m.agents[agent.ParentID]; ok {
			for i, child := range parent.Children {
				if child == oldName {
					parent.Children[i] = newName
					break
				}
			}
		}
	}

	// Update children's ParentID (it's still the old name)
	for _, childName := range agent.Children {
		if child, ok := m.agents[childName]; ok {
			if child.ParentID == oldName {
				child.ParentID = newName
				child.UpdatedAt = now
			}
		}
	}

	if err := m.saveState(ctx); err != nil {
		return fmt.Errorf("rename: failed to save state: %w", err)
	}

	log.Debug("agent renamed", "oldName", oldName, "newName", newName)
	return nil
}

// StopAll stops all agents.
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for name, agent := range m.agents {
		_ = m.runtimeForAgent(name).KillSession(ctx, name) //nolint:errcheck // best-effort cleanup
		agent.State = StateStopped
		agent.StoppedAt = &now
		agent.UpdatedAt = now
	}

	if err := m.saveState(ctx); err != nil {
		log.Warn("failed to save agent state", "error", err)
	}
	return nil
}

// GetAgent returns a copy of an agent by name.
// Returns nil if the agent doesn't exist.
func (m *Manager) GetAgent(name string) *Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, exists := m.agents[name]
	if !exists {
		return nil
	}
	// Return a copy to avoid data races
	copy := *a
	copy.Children = append([]string{}, a.Children...)
	return &copy
}

// SetArchived stamps (or clears) ArchivedAt on an in-memory agent and
// persists the whole agent map. Returns an error if the named agent is
// missing. Safe to call concurrently — uses m.mu for write serialization.
// This is the backing primitive for the AgentService Archive/Unarchive
// methods; it intentionally does NOT kill the runtime or touch on-disk
// state beyond the SQLite agent store (archive is reversible).
func (m *Manager) SetArchived(ctx context.Context, name string, archived bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, exists := m.agents[name]
	if !exists {
		return fmt.Errorf("agent %s: %w", name, ErrNotFound)
	}
	if archived {
		if a.ArchivedAt != nil {
			return nil // idempotent
		}
		now := time.Now()
		a.ArchivedAt = &now
	} else {
		if a.ArchivedAt == nil {
			return nil
		}
		a.ArchivedAt = nil
	}
	return m.saveState(ctx)
}

// ListAgents returns copies of all agents sorted by role hierarchy then by name.
// Order: ProductManager/Coordinator → Manager → Engineer/Worker
func (m *Manager) ListAgents() []*Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return copies to avoid data races
	agents := make([]*Agent, 0, len(m.agents))
	for _, a := range m.agents {
		copy := *a
		copy.Children = append([]string{}, a.Children...)
		agents = append(agents, &copy)
	}

	sort.Slice(agents, func(i, j int) bool {
		// Sort by hierarchy level first
		levelI := RoleLevel(agents[i].Role)
		levelJ := RoleLevel(agents[j].Role)
		if levelI != levelJ {
			return levelI < levelJ
		}
		// Then by name
		return agents[i].Name < agents[j].Name
	})

	return agents
}

// ListChildren returns copies of all direct children of an agent.
func (m *Manager) ListChildren(parentID string) []*Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	parent, exists := m.agents[parentID]
	if !exists {
		return nil
	}

	children := make([]*Agent, 0, len(parent.Children))
	for _, childID := range parent.Children {
		if child, exists := m.agents[childID]; exists {
			// Return copy to avoid data races
			copy := *child
			copy.Children = append([]string{}, child.Children...)
			children = append(children, &copy)
		}
	}

	return children
}

// ListDescendants returns all descendants of an agent (children, grandchildren, etc.).
func (m *Manager) ListDescendants(parentID string) []*Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var descendants []*Agent
	m.collectDescendants(parentID, &descendants)
	return descendants
}

// collectDescendants recursively collects copies of all descendants.
func (m *Manager) collectDescendants(parentID string, result *[]*Agent) {
	parent, exists := m.agents[parentID]
	if !exists {
		return
	}

	for _, childID := range parent.Children {
		if child, exists := m.agents[childID]; exists {
			// Return copy to avoid data races
			copy := *child
			copy.Children = append([]string{}, child.Children...)
			*result = append(*result, &copy)
			m.collectDescendants(childID, result)
		}
	}
}

// GetParent returns a copy of the parent agent, or nil if no parent.
func (m *Manager) GetParent(agentID string) *Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, exists := m.agents[agentID]
	if !exists || agent.ParentID == "" {
		return nil
	}

	parent, exists := m.agents[agent.ParentID]
	if !exists {
		return nil
	}
	// Return copy to avoid data races
	copy := *parent
	copy.Children = append([]string{}, parent.Children...)
	return &copy
}

// ListByRole returns copies of all agents with a specific role.
func (m *Manager) ListByRole(role Role) []*Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var agents []*Agent
	for _, a := range m.agents {
		if a.Role == role {
			// Return copy to avoid data races
			copy := *a
			copy.Children = append([]string{}, a.Children...)
			agents = append(agents, &copy)
		}
	}

	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	return agents
}

// AgentCount returns the number of agents.
func (m *Manager) AgentCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.agents)
}

// RunningCount returns the number of non-stopped agents.
func (m *Manager) RunningCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, a := range m.agents {
		if a.State != StateStopped {
			count++
		}
	}
	return count
}

// SetAgentEnv replaces an agent's configured environment variables.
// Values may contain ${secret:NAME} references — they are stored verbatim
// and resolved at spawn time, so changes (and rotated secrets) take effect
// on the next restart.
func (m *Manager) SetAgentEnv(ctx context.Context, name string, env map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, exists := m.agents[name]
	if !exists {
		return fmt.Errorf("agent %s: %w", name, ErrNotFound)
	}
	a.Env = env
	a.UpdatedAt = time.Now()

	if err := m.saveState(ctx); err != nil {
		return fmt.Errorf("save agent state: %w", err)
	}
	return nil
}

// SetAgentResources sets a per-agent Docker CPU/memory cap. Values of 0
// clear the override and fall back to the fleet default. The new caps
// take effect on the next session (re)start — a running container is not
// resized in place. Docker-only; stored regardless of runtime so a later
// switch to Docker honors them.
func (m *Manager) SetAgentResources(ctx context.Context, name string, cpus float64, memoryMB int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, exists := m.agents[name]
	if !exists {
		return fmt.Errorf("agent %s: %w", name, ErrNotFound)
	}
	a.CPUs = cpus
	a.MemoryMB = memoryMB
	a.UpdatedAt = time.Now()

	if err := m.saveState(ctx); err != nil {
		return fmt.Errorf("save agent state: %w", err)
	}
	return nil
}

// SetAgentResourcesPartial atomically merges the supplied CPU/memory
// overrides into the agent's current stored values under a single lock.
// Nil pointers leave the corresponding field untouched, so two concurrent
// partial updates (one setting only cpus, the other only memory_mb) cannot
// lose each other's change — the read and write happen inside the same lock,
// never against a snapshot taken before it. Returns the merged values.
func (m *Manager) SetAgentResourcesPartial(ctx context.Context, name string, cpus *float64, memoryMB *int64) (resolvedCPUs float64, resolvedMemoryMB int64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, exists := m.agents[name]
	if !exists {
		return 0, 0, fmt.Errorf("agent %s: %w", name, ErrNotFound)
	}
	if cpus != nil {
		a.CPUs = *cpus
	}
	if memoryMB != nil {
		a.MemoryMB = *memoryMB
	}
	a.UpdatedAt = time.Now()

	if saveErr := m.saveState(ctx); saveErr != nil {
		return 0, 0, fmt.Errorf("save agent state: %w", saveErr)
	}
	return a.CPUs, a.MemoryMB, nil
}

// SetAgentModel sets the provider model identifier the agent runs with.
// An empty string clears the override, restoring the provider default. The
// new model is reused on the next session (re)start — the running session
// is not re-launched in place.
func (m *Manager) SetAgentModel(ctx context.Context, name, model string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, exists := m.agents[name]
	if !exists {
		return fmt.Errorf("agent %s: %w", name, ErrNotFound)
	}
	a.Model = model
	a.UpdatedAt = time.Now()

	if err := m.saveState(ctx); err != nil {
		return fmt.Errorf("save agent state: %w", err)
	}
	return nil
}

// UpdateAgentState updates an agent's state and task.
// Returns an error if the transition is invalid per the state machine.
func (m *Manager) UpdateAgentState(ctx context.Context, name string, state State, task string) error {
	var changed bool

	m.mu.Lock()
	agent, exists := m.agents[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("agent %s: %w", name, ErrNotFound)
	}

	if err := ValidateTransition(agent.State, state); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("agent %s: %w", name, err)
	}

	prevState := agent.State
	agent.State = state
	agent.Task = task
	agent.UpdatedAt = time.Now()
	changed = prevState != state

	if err := m.saveState(ctx); err != nil {
		log.Warn("failed to save agent state", "error", err)
	}
	m.mu.Unlock()

	// Notify outside the lock to avoid deadlocks with RLock in notifyStateChange.
	if changed {
		m.notifyStateChange(name, state, task)
	}
	return nil
}

// SetAgentTask updates an agent's task line without touching its lifecycle
// state, so recording what an agent is working on never has to satisfy the
// state machine. Two things write here: hook ingestion, which derives the task
// from the prompt on a user turn, and spawn_agent, which records the task a
// parent handed its child before that child has run a turn of its own.
func (m *Manager) SetAgentTask(ctx context.Context, name, task string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	agent, exists := m.agents[name]
	if !exists {
		return fmt.Errorf("agent %s: %w", name, ErrNotFound)
	}
	agent.Task = task
	agent.UpdatedAt = time.Now()
	if err := m.saveState(ctx); err != nil {
		log.Warn("failed to save agent state", "error", err)
	}
	return nil
}

// SetAgentState updates an agent's state from a lifecycle event while
// preserving the agent's task. Lifecycle descriptions ("Turn complete",
// "Session ended", …) belong in the activity/event stream, not in the Task
// field — Task holds what the agent was asked to do, derived from the prompt on
// its last user turn. The task is cleared when the agent stops so a dead agent
// doesn't keep advertising a stale task.
// Returns an error if the transition is invalid per the state machine.
func (m *Manager) SetAgentState(ctx context.Context, name string, state State) error {
	var (
		changed bool
		task    string
	)

	m.mu.Lock()
	agent, exists := m.agents[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("agent %s: %w", name, ErrNotFound)
	}

	if err := ValidateTransition(agent.State, state); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("agent %s: %w", name, err)
	}

	prevState := agent.State
	agent.State = state
	if state == StateStopped {
		agent.Task = ""
	}
	task = agent.Task
	agent.UpdatedAt = time.Now()
	changed = prevState != state

	if err := m.saveState(ctx); err != nil {
		log.Warn("failed to save agent state", "error", err)
	}
	m.mu.Unlock()

	// Notify outside the lock to avoid deadlocks with RLock in notifyStateChange.
	if changed {
		m.notifyStateChange(name, state, task)
	}
	return nil
}

// SetAgentTeam assigns an agent to a team.
func (m *Manager) SetAgentTeam(ctx context.Context, name, team string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, exists := m.agents[name]
	if !exists {
		return fmt.Errorf("agent %s: %w", name, ErrNotFound)
	}

	agent.Team = team
	agent.UpdatedAt = time.Now()

	if err := m.saveState(ctx); err != nil {
		log.Warn("failed to save agent state", "error", err)
	}
	return nil
}

// SendToAgent sends a message/command to an agent's session.
// Sends Enter after the message to submit it.
func (m *Manager) SendToAgent(ctx context.Context, name, message string) error {
	m.mu.RLock()
	be := m.runtimeForAgent(name)
	m.mu.RUnlock()
	return be.SendKeys(ctx, name, message)
}

// CaptureOutput captures recent output from an agent's session.
// Reads from the agent's log file first (includes full history with ANSI).
// Falls back to tmux capture-pane if log file is not available.
func (m *Manager) CaptureOutput(ctx context.Context, name string, lines int) (string, error) {
	m.mu.RLock()
	agent := m.agents[name]
	rt := m.runtimeForAgent(name)
	m.mu.RUnlock()

	// Try log file first
	if agent != nil && agent.LogFile != "" {
		output, err := tailFile(agent.LogFile, lines)
		if err == nil && output != "" {
			return output, nil
		}
		log.Debug("log file read failed, falling back to capture-pane", "agent", name, "error", err)
	}

	// Fall back to tmux capture-pane
	return rt.Capture(ctx, name, lines)
}

// tailFile reads the last N lines from a file.
// It reads only the last 64KB instead of the entire file to avoid large allocations.
func tailFile(path string, lines int) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path from trusted agent state
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck // best-effort close on read-only file

	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := fi.Size()
	if size == 0 {
		return "", nil
	}

	// Read at most the last 64KB — enough for typical tail operations.
	const maxRead = 64 * 1024
	readSize := size
	if readSize > maxRead {
		readSize = maxRead
	}

	buf := make([]byte, readSize)
	_, err = f.ReadAt(buf, size-readSize)
	if err != nil && err != io.EOF {
		return "", err
	}

	// Find last N lines by scanning backward
	count := 0
	pos := len(buf) - 1
	// Skip trailing newline
	if pos >= 0 && buf[pos] == '\n' {
		pos--
	}
	for pos >= 0 {
		if buf[pos] == '\n' {
			count++
			if count >= lines {
				pos++
				break
			}
		}
		pos--
	}
	if pos < 0 {
		pos = 0
	}

	return string(buf[pos:]), nil
}

// FollowOutput streams new log lines in real-time, like tail -f.
// It prints the last N lines first, then polls for new content every 200ms.
// Blocks until the context is canceled.
// Falls back to a one-shot CaptureOutput if no log file exists.
func (m *Manager) FollowOutput(ctx context.Context, name string, lines int, w io.Writer) error {
	m.mu.RLock()
	a := m.agents[name]
	m.mu.RUnlock()

	if a == nil {
		return fmt.Errorf("agent %q: %w", name, ErrNotFound)
	}

	// No log file — fall back to one-shot capture
	if a.LogFile == "" {
		output, err := m.CaptureOutput(ctx, name, lines)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, output)
		return err
	}

	f, err := os.Open(a.LogFile) //nolint:gosec // path from trusted agent state
	if err != nil {
		// Log file doesn't exist yet — fall back to one-shot
		output, captureErr := m.CaptureOutput(ctx, name, lines)
		if captureErr != nil {
			return captureErr
		}
		_, captureErr = io.WriteString(w, output)
		return captureErr
	}
	defer func() { _ = f.Close() }()

	// Print last N lines to start
	initial, tailErr := tailFile(a.LogFile, lines)
	if tailErr == nil && initial != "" {
		if _, writeErr := io.WriteString(w, initial); writeErr != nil {
			return writeErr
		}
	}

	// Seek to end for follow mode
	offset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("seek failed: %w", err)
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			n, readErr := f.ReadAt(buf, offset)
			if n > 0 {
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					return writeErr
				}
				offset += int64(n)
			}
			if readErr != nil && readErr != io.EOF {
				return fmt.Errorf("read failed: %w", readErr)
			}
		}
	}
}

// AttachToAgent returns the command to attach to an agent's session.
func (m *Manager) AttachToAgent(ctx context.Context, name string) error {
	m.mu.RLock()
	be := m.runtimeForAgent(name)
	m.mu.RUnlock()
	cmd := be.AttachCmd(ctx, name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// saveState persists agent state to SQLite.
// SQLite with WAL mode handles concurrency natively — no file locks needed.
// Must be called while holding m.mu.
func (m *Manager) saveState(ctx context.Context) error {
	if m.store == nil {
		return nil
	}
	return m.store.SaveAll(ctx, m.agents)
}

// LoadState loads agent state from SQLite.
// On first run after upgrade, migrates JSON files to SQLite automatically.
func (m *Manager) LoadState() error {
	if m.stateDir == "" {
		return nil
	}

	// Open SQLite store on the single global mycel.db — agents from
	// every repo share the one database. The anchor repo is only the
	// default for new agents.
	dbPath, pathErr := db.GlobalDBPath()
	if pathErr != nil {
		return fmt.Errorf("resolve global db path: %w", pathErr)
	}
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("open agent store: %w", err)
	}
	m.store = store

	// Auto-migrate JSON files if they exist
	if needsMigration(m.stateDir) {
		log.Info("migrating agent state from JSON to SQLite")
		if migErr := migrateJSONToSQLite(store, m.stateDir, m.repoPath); migErr != nil {
			log.Warn("migration had errors", "error", migErr)
		}
	}

	// Load every agent from the global agents table. The manager is
	// single-tenant: the boot repo is only the default for new agents,
	// so agents created against other repos must survive a restart.
	agents, err := store.LoadAll(context.Background())
	if err != nil {
		return fmt.Errorf("load agents: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents = agents
	return nil
}

// RepoCounts returns the number of agents per distinct repo path across
// the whole global agents table (not just this manager's repo). Falls
// back to the in-memory agents when no store is open (tests / standalone
// managers). Errors degrade to the in-memory view — the caller renders a
// list, not a diagnosis.
func (m *Manager) RepoCounts(ctx context.Context) map[string]int {
	m.mu.RLock()
	store := m.store
	m.mu.RUnlock()
	if store != nil {
		if counts, err := store.RepoCounts(ctx); err == nil {
			return counts
		}
	}
	counts := make(map[string]int)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, a := range m.agents {
		if a.Repo != "" {
			counts[a.Repo]++
		}
	}
	return counts
}

// Runtime returns the default runtime backend for session management.
func (m *Manager) Runtime() runtime.Backend {
	return m.runtime()
}

// RuntimeForAgent returns the runtime backend for a specific agent.
func (m *Manager) RuntimeForAgent(name string) runtime.Backend {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.runtimeForAgent(name)
}

// QueryAgentStats returns up to limit recent stats records for the named agent.
func (m *Manager) QueryAgentStats(agentName string, limit int) ([]*AgentStatsRecord, error) {
	if m.store == nil {
		return nil, fmt.Errorf("no store available")
	}
	return m.store.QueryStats(context.Background(), agentName, limit)
}

// RecordAgentStats persists a single AgentStatsRecord to the SQLite store.
// This is used by the background container metrics collector to save Docker
// resource samples so the /api/agents/{name}/stats endpoint returns real data.
func (m *Manager) RecordAgentStats(rec *AgentStatsRecord) error {
	if m.store == nil {
		return fmt.Errorf("no store available")
	}
	return m.store.SaveStats(context.Background(), rec)
}

// Close closes the SQLite store. Call when done with the manager.
func (m *Manager) Close() error {
	if m.store != nil {
		return m.store.Close()
	}
	return nil
}

// RepoPath returns the repo root path for this manager.
func (m *Manager) RepoPath() string {
	return m.repoPath
}

// WorktreePath returns the filesystem path for an agent's worktree directory.
// Returns an empty string if no worktree manager is configured.
func (m *Manager) WorktreePath(agentName string) string {
	if m.worktreeMgr == nil {
		return ""
	}
	return m.worktreeMgr.Path(agentName)
}

// WorktreeDirFor returns an agent's authoritative worktree directory:
// the stored WorktreeDir when set (it may live anywhere the agent's repo
// is), else the manager-computed path. Empty if the agent is unknown and
// no path can be computed.
func (m *Manager) WorktreeDirFor(agentName string) string {
	m.mu.RLock()
	if a, ok := m.agents[agentName]; ok && a.WorktreeDir != "" {
		dir := a.WorktreeDir
		m.mu.RUnlock()
		return dir
	}
	m.mu.RUnlock()
	return m.WorktreePath(agentName)
}

// CreateWorktree creates a git worktree for the given agent name.
// Returns the worktree path, or an error if no worktree manager is configured.
func (m *Manager) CreateWorktree(ctx context.Context, agentName string) (string, error) {
	if m.worktreeMgr == nil {
		return "", fmt.Errorf("worktree manager not configured")
	}
	return m.worktreeMgr.Create(ctx, agentName)
}

// RegisterStopped registers a pre-built Agent record in stopped state without
// starting a session. The caller is responsible for setting all required fields
// (Name, Role, Workspace, Tool, RuntimeBackend, WorktreeDir) before calling this.
// Returns an error if an agent with the same name already exists.
func (m *Manager) RegisterStopped(a *Agent) error {
	if a.Name == "" {
		return fmt.Errorf("agent name is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.agents[a.Name]; exists {
		return fmt.Errorf("agent %q already exists", a.Name)
	}
	if err := m.checkNameAvailable(context.Background(), a.Name); err != nil {
		return err
	}

	now := time.Now()
	a.State = StateStopped
	if a.Repo == "" {
		a.Repo = cleanRepoPath(m.repoPath)
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now

	if a.Children == nil {
		a.Children = []string{}
	}

	m.agents[a.Name] = a
	if err := m.saveState(context.Background()); err != nil {
		delete(m.agents, a.Name)
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}

// cleanRepoPath normalizes a repo path to an absolute cleaned path —
// the canonical form of the global `repo` attribution key.
func cleanRepoPath(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(p)
}

// checkNameAvailable rejects names already taken in the global agents
// table by an agent this manager does not know about. Agent names are
// globally unique across every repo: the name is the primary key of
// the single mycel.db agents table. Soft-deleted rows do not block
// reuse. Best-effort: with no store (standalone managers, some tests)
// the in-memory check in the caller is the only guard.
func (m *Manager) checkNameAvailable(ctx context.Context, name string) error {
	if m.store == nil {
		return nil
	}
	other, err := m.store.Load(ctx, name)
	if err != nil || other == nil || other.DeletedAt != nil {
		return nil //nolint:nilerr // lookup failure must not block spawning
	}
	owner := other.Repo
	if owner == "" {
		owner = other.Workspace
	}
	if owner == "" {
		owner = "another repo"
	}
	return fmt.Errorf("agent name %q is already in use by an agent in %s — agent names are global, pick a different name or delete that agent first", name, owner)
}

// enforceRootSingleton checks if a root agent can be spawned.
// Returns an error if a root already exists and is running.
// Allows respawn if root is stopped or in error state.
func (m *Manager) enforceRootSingleton(_ string) error {
	// Check in-memory state for existing root
	for _, a := range m.agents {
		if a.IsRoot {
			switch a.State {
			case StateStopped, StateError:
				// Terminal state - allow respawn
				log.Debug("root in terminal state, allowing respawn", "state", a.State)
				return nil
			default:
				// Root is active - deny new spawn
				return fmt.Errorf("root agent already exists and is in state %q", a.State)
			}
		}
	}
	return nil
}

// daemonAddrForRuntime returns the daemon server address for the given runtime.
// Docker containers reach the host via host.docker.internal.
// If MYCEL_DAEMON_ADDR is set in the environment, it is used as the base address
// (with host.docker.internal substituted for Docker runtimes).
func daemonAddrForRuntime(rt string) string {
	if addr := os.Getenv("MYCEL_DAEMON_ADDR"); addr != "" {
		// Normalize empty hostname: "http://:8080" → "http://127.0.0.1:8080"
		if u, parseErr := url.Parse(addr); parseErr == nil && u.Hostname() == "" && u.Port() != "" {
			u.Host = net.JoinHostPort("127.0.0.1", u.Port())
			addr = u.String()
		}
		if rt == "docker" {
			// Replace localhost/127.0.0.1 with host.docker.internal for Docker
			addr = strings.ReplaceAll(addr, "127.0.0.1", "host.docker.internal")
			addr = strings.ReplaceAll(addr, "localhost", "host.docker.internal")
		}
		return addr
	}
	if rt == "docker" {
		return "http://host.docker.internal:9374"
	}
	return "http://127.0.0.1:9374"
}

// injectResourceLimits exposes an agent's per-agent Docker CPU/memory caps
// to the runtime backend via MYCEL_CPUS / MYCEL_MEMORY_MB. The Docker
// backend reads these to override the fleet-default --cpus/--memory flags
// for this one container. Zero values are omitted so the backend keeps the
// fleet default. tmux ignores them (limits are not enforced there).
func injectResourceLimits(env map[string]string, cpus float64, memoryMB int64) {
	if cpus > 0 {
		env["MYCEL_CPUS"] = strconv.FormatFloat(cpus, 'f', -1, 64)
	}
	if memoryMB > 0 {
		env["MYCEL_MEMORY_MB"] = strconv.FormatInt(memoryMB, 10)
	}
}

// injectEnv merges environment variables from the agent env file and the
// agent's configured env map, then resolves ${secret:NAME} references.
// Merge order: env file first, then userEnv (explicit config wins over the
// file), with MYCEL_* system vars always protected via mergeUserEnv.
func injectEnv(env map[string]string, repoPath, agentName, envFile string, userEnv map[string]string) {
	// Agent env file
	if envFile != "" {
		parseEnvFile(env, envFile)
	}
	// Per-agent configured env vars
	mergeUserEnv(env, userEnv, agentName)
	// Resolve ${secret:NAME} references in all env values
	resolveSecretRefs(env, repoPath)
}

// mergeUserEnv merges user-configured env vars into env. Keys starting
// with "MYCEL_" are reserved for the system (MYCEL_AGENT_ID, MYCEL_WORKSPACE, …)
// and are skipped with a warning so user config can never clobber them.
func mergeUserEnv(env, userEnv map[string]string, agentName string) {
	for k, v := range userEnv {
		if strings.HasPrefix(k, "MYCEL_") {
			log.Warn("skipping reserved env var from agent config", "agent", agentName, "key", k)
			continue
		}
		env[k] = v
	}
}

// appPromptInstructions generates markdown instructions that tell an
// agent about available platform credentials injected as environment
// variables. Lines are derived generically from each connected app's
// descriptor: every secret field plus every required plain field with a
// configured value gets an env var documented here.
func appPromptInstructions(apps map[string]app.InstanceConfig) string {
	var lines []string
	for name, ic := range apps {
		if !ic.Enabled {
			continue
		}
		plugin, ok := app.Get(ic.App)
		if !ok {
			continue
		}
		d := plugin.Describe()
		for _, f := range d.Fields {
			switch {
			case f.Secret:
				lines = append(lines, fmt.Sprintf("- %s: %s %s.", app.EnvKey(name, f.Key), d.Label, f.Label))
			case f.Required && ic.Config[f.Key] != "":
				lines = append(lines, fmt.Sprintf("- %s: %s %s.", app.EnvKey(name, f.Key), d.Label, f.Label))
			}
		}
	}

	if len(lines) == 0 {
		return ""
	}

	sort.Strings(lines) // deterministic output order

	var sb strings.Builder
	sb.WriteString("\n## Platform Credentials\n\n")
	sb.WriteString("You have access to these platform credentials via environment variables:\n\n")
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteString("\n")
	}
	sb.WriteString("\nYour agent name is available as the `MYCEL_AGENT_ID` environment variable.\n")
	return sb.String()
}

// appendAppPrompt appends platform credential instructions to the agent's
// CLAUDE.md (or provider-equivalent prompt file) if connected apps exist.
func appendAppPrompt(targetDir, toolName string, apps map[string]app.InstanceConfig) {
	instructions := appPromptInstructions(apps)
	if instructions == "" {
		return
	}

	// targetDir is an agent worktree path derived from validated names;
	// reject traversal segments as defense in depth.
	targetDir = filepath.Clean(targetDir)
	if strings.Contains(targetDir, "..") {
		log.Warn("refusing to append app prompt to traversal path", "dir", targetDir)
		return
	}

	adapter := resolveConfigAdapter(toolName)
	promptFile := filepath.Join(targetDir, adapter.PromptFile())
	f, err := os.OpenFile(promptFile, os.O_APPEND|os.O_WRONLY, 0600) //nolint:gosec // controlled agent repo path
	if err != nil {
		log.Debug("cannot append app prompt, prompt file not writable", "path", promptFile, "error", err)
		return
	}
	defer f.Close() //nolint:errcheck
	if _, err := f.WriteString(instructions); err != nil {
		log.Warn("failed to append app prompt instructions", "path", promptFile, "error", err)
	}
}

// injectedPromptFile resolves the prompt file path (CLAUDE.md or the
// provider-equivalent) inside an agent worktree for the given tool.
func injectedPromptFile(targetDir, toolName string) string {
	adapter := resolveConfigAdapter(toolName)
	return filepath.Join(targetDir, adapter.PromptFile())
}

// appendInjectedInstructions appends the mycel-authored injected-instructions
// block to an agent's prompt file. The block carries the authored text plus an
// auto-generated summary of the MCP servers and credential env var NAMES the
// agent has access to. Secret VALUES are never written — only key names.
//
// It is a no-op (returns nil) when no instructions are configured.
func appendInjectedInstructions(ctx context.Context, promptFile string, cfg *home.Config, mcpServers, secretEnvKeys []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if cfg == nil || strings.TrimSpace(cfg.InjectedInstructions) == "" {
		return nil
	}

	// promptFile is derived from a validated agent worktree path; reject
	// traversal segments as defense in depth.
	cleaned := filepath.Clean(promptFile)
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("refusing to write injected instructions to traversal path %q", promptFile)
	}

	var sb strings.Builder
	sb.WriteString("\n## mycel instructions\n\n")
	sb.WriteString(strings.TrimSpace(cfg.InjectedInstructions))
	sb.WriteString("\n\n### Available resources\n")
	sb.WriteString("MCP servers: " + summarizeNames(mcpServers) + "\n")
	sb.WriteString("Credential env vars: " + summarizeNames(secretEnvKeys) + "\n")

	f, err := os.OpenFile(cleaned, os.O_APPEND|os.O_WRONLY, 0600) //nolint:gosec // controlled agent repo path
	if err != nil {
		return fmt.Errorf("open prompt file: %w", err)
	}
	defer f.Close() //nolint:errcheck
	if _, err := f.WriteString(sb.String()); err != nil {
		return fmt.Errorf("write injected instructions: %w", err)
	}
	return nil
}

// summarizeNames renders a sorted, comma-separated list of names, or the
// literal "none" when the list is empty. Used to summarize MCP servers and
// credential env var NAMES for injected instructions — never values.
func summarizeNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}

// injectAppEnv injects non-secret required app config values (feed
// URLs, homeservers, user IDs, ...) into agent environment variables.
// Secret fields are resolved from the vault by injectVaultSecrets.
func injectAppEnv(env map[string]string, apps map[string]app.InstanceConfig) {
	for name, ic := range apps {
		if !ic.Enabled {
			continue
		}
		plugin, ok := app.Get(ic.App)
		if !ok {
			continue
		}
		for _, f := range plugin.Describe().Fields {
			if f.Secret || !f.Required {
				continue
			}
			if v := ic.Config[f.Key]; v != "" {
				env[app.EnvKey(name, f.Key)] = v
			}
		}
	}
}

// wellKnownVaultTokens lists vault secret names that are automatically injected
// as agent env vars when present, without requiring explicit role declaration.
// These are integration/gateway tokens that agents commonly need.
//
//nolint:gochecknoglobals // package-level constant slice — read-only
var wellKnownVaultTokens = []string{
	"SLACK_BOT_TOKEN",
	"SLACK_APP_TOKEN",
	"TELEGRAM_BOT_TOKEN",
	"DISCORD_BOT_TOKEN",
	"WHATSAPP_SESSION",
}

// openLayeredStore opens the global + repo vault layers and returns a
// LayeredStore along with a closer function (never nil when ls != nil).
// Either layer may be missing/unopenable; at least one must succeed or ls is nil.
// The caller is responsible for calling closeFunc() when done.
func openLayeredStore(repoPath, passphrase string) (ls *secret.LayeredStore, closeFunc func()) {
	globalVaultPath, err := home.GlobalSecretsVault()
	if err != nil {
		log.Debug("vault injection: global vault path unavailable", "error", err)
	}

	var globalStore *secret.Store
	if globalVaultPath != "" {
		if gs, e := secret.OpenVaultFile(globalVaultPath, passphrase); e == nil {
			globalStore = gs
		} else {
			log.Debug("vault injection: global vault unavailable", "error", e)
		}
	}

	var repoStore *secret.Store
	if repoPath != "" {
		if h, e := secret.NewStore(repoPath, passphrase); e == nil {
			repoStore = h
		} else {
			log.Debug("vault injection: repo vault unavailable", "error", e)
		}
	}

	if globalStore == nil && repoStore == nil {
		return nil, nil
	}

	ls = secret.NewLayeredStore(globalStore, repoStore)
	closeFunc = func() { _ = ls.Close() }
	return ls, closeFunc
}

// injectVaultSecrets injects vault secrets into the agent env map.
//
// Precedence (highest → lowest):
//  1. Existing value in env (set by agent env-file or injectAppEnv)
//  2. Vault value (global ~/.mycel/secrets.vault + repo <repo>/.mycel/secrets.db, repo wins)
//
// Call AFTER injectEnv + injectAppEnv so that explicitly-set values are
// never overwritten by vault copies.
//
// Role-scoped secrets (roleSecrets from ResolvedRole.Secrets) act as an
// allowlist: vault values are not sprayed across every agent indiscriminately.
// Connected-app credentials (descriptor Secret fields, stored under
// app:<instance>:<key>) and well-known integration tokens (SLACK_BOT_TOKEN
// etc.) are also exported when present as a convenience for agents that
// don't declare them in their role.
//
// GITHUB_PERSONAL_ACCESS_TOKEN and GITHUB_TOKEN in the vault are aliased to
// both GITHUB_TOKEN and GH_TOKEN so git/gh tooling works without manual wiring.
// The connected GitHub app's api_token (app:<instance>:api_token, populated
// by "Sign in with GitHub" or pasted manually) is aliased the same way.
//
// Secret VALUES are never logged.
//
// The returned slice holds the NAMES of env keys populated from the vault
// (never their values) so callers can summarize available credentials to the
// agent without ever exposing the secrets themselves.
func injectVaultSecrets(env map[string]string, repoPath string, roleSecrets []string, apps map[string]app.InstanceConfig) []string {
	passphrase, err := secret.Passphrase()
	if err != nil {
		log.Warn("vault injection skipped: cannot read passphrase", "error", err)
		return nil
	}

	ls, closeLS := openLayeredStore(repoPath, passphrase)
	if ls == nil {
		return nil
	}
	defer closeLS()

	var injected []string

	// injectIfAbsent sets env[key] from the vault secret "name" only when the
	// key is not already present. Values are intentionally not logged.
	injectIfAbsent := func(key, name string) {
		if _, exists := env[key]; exists {
			return // existing value wins
		}
		val, e := ls.GetValue(name)
		if e != nil || val == "" {
			return
		}
		env[key] = val
		injected = append(injected, key)
		log.Debug("injected vault secret into agent env", "key", key)
	}

	// 1. Role-declared secrets — scoped allowlist so each role controls which
	//    vault entries it receives.
	for _, name := range roleSecrets {
		injectIfAbsent(name, name)
	}

	// 1.5. Connected-app credentials — descriptor Secret fields resolve from
	//      the vault under app:<instance>:<key> into conventional env names
	//      (SLACK_BOT_TOKEN, TELEGRAM_BOT_TOKEN_ALERTS, ...).
	//
	//      Special case: the GitHub app's "api_token" field (populated by the
	//      device-flow "Sign in with GitHub" OAuth, or pasted manually) is
	//      additionally aliased to GH_TOKEN/GITHUB_TOKEN so `gh` and git's
	//      credential helper authenticate automatically — same convenience as
	//      the GITHUB_PERSONAL_ACCESS_TOKEN vault alias below (step 3), just
	//      sourced from a connected app instead of a hand-set vault secret.
	//      Scoping matches every other connected-app credential here: any
	//      enabled instance's token is available to all agents (the same
	//      model as SLACK_BOT_TOKEN etc.) — there is no per-agent app
	//      subscription in this codebase to scope against.
	for name, ic := range apps {
		if !ic.Enabled {
			continue
		}
		plugin, ok := app.Get(ic.App)
		if !ok {
			continue
		}
		for _, f := range plugin.Describe().Fields {
			if !f.Secret {
				continue
			}
			injectIfAbsent(app.EnvKey(name, f.Key), app.SecretName(name, f.Key))
			if ic.App == "github" && f.Key == "api_token" {
				injectIfAbsent("GITHUB_TOKEN", app.SecretName(name, f.Key))
				injectIfAbsent("GH_TOKEN", app.SecretName(name, f.Key))
			}
		}
	}

	// 2. Well-known integration tokens — convenience auto-export regardless of
	//    role declaration so "connect once → agents have it" works out of the box.
	for _, name := range wellKnownVaultTokens {
		injectIfAbsent(name, name)
	}

	// 3. GitHub PAT aliases: vault GITHUB_PERSONAL_ACCESS_TOKEN or GITHUB_TOKEN
	//    is exported as both GITHUB_TOKEN and GH_TOKEN so git/gh commands work.
	for _, src := range []string{"GITHUB_PERSONAL_ACCESS_TOKEN", "GITHUB_TOKEN"} {
		val, e := ls.GetValue(src)
		if e != nil || val == "" {
			continue
		}
		if _, exists := env["GITHUB_TOKEN"]; !exists {
			env["GITHUB_TOKEN"] = val
			injected = append(injected, "GITHUB_TOKEN")
			log.Debug("injected vault secret into agent env", "key", "GITHUB_TOKEN")
		}
		if _, exists := env["GH_TOKEN"]; !exists {
			env["GH_TOKEN"] = val
			injected = append(injected, "GH_TOKEN")
			log.Debug("injected vault secret into agent env", "key", "GH_TOKEN")
		}
		break // first source wins; don't double-process
	}

	return injected
}

// resolveRoleSecrets returns the Secrets list for a named role via BFS
// inheritance merge. Returns nil (no error) when the role cannot be loaded
// so callers can proceed with an empty allowlist.
func resolveRoleSecrets(repoPath, roleName string) []string {
	rm, err := home.NewGlobalRoleManager(mycelHomeOrEmpty())
	if err != nil {
		log.Debug("resolveRoleSecrets: cannot open role manager", "error", err)
		return nil
	}
	resolved, err := rm.ResolveRole(roleName)
	if err != nil {
		log.Debug("resolveRoleSecrets: cannot resolve role", "role", roleName, "error", err)
		return nil
	}
	return resolved.Secrets
}

// resolveRoleMCPServers returns the effective MCP server names an agent of the
// named role receives. It returns the resolved role's MCPServers list only —
// no implicit servers are added. Returns nil when the role cannot be loaded.
func resolveRoleMCPServers(repoPath, roleName string) []string {
	rm, err := home.NewGlobalRoleManager(mycelHomeOrEmpty())
	if err != nil {
		log.Debug("resolveRoleMCPServers: cannot open role manager", "error", err)
		return nil
	}
	resolved, err := rm.ResolveRole(roleName)
	if err != nil {
		log.Debug("resolveRoleMCPServers: cannot resolve role", "role", roleName, "error", err)
		return nil
	}
	return resolved.MCPServers
}

// resolveSecretRefs resolves ${secret:NAME} references in env values using the
// layered vault (global + repo). If neither vault can be opened, references
// are left as-is.
func resolveSecretRefs(env map[string]string, repoPath string) {
	// Check if any values contain secret references before opening the store
	hasRefs := false
	for _, v := range env {
		if strings.Contains(v, "${secret:") {
			hasRefs = true
			break
		}
	}
	if !hasRefs {
		return
	}

	passphrase, err := secret.Passphrase()
	if err != nil {
		log.Warn("failed to resolve secret passphrase", "error", err)
		return
	}

	ls, closeLS := openLayeredStore(repoPath, passphrase)
	if ls == nil {
		log.Warn("failed to open secret store for env resolution")
		return
	}
	defer closeLS()

	resolved := ls.ResolveEnv(env)
	for k, v := range resolved {
		env[k] = v
	}
}

// parseEnvFile reads KEY=VALUE lines from a file and merges them into env.
// Lines starting with # and blank lines are skipped.
func parseEnvFile(env map[string]string, path string) {
	data, err := os.ReadFile(path) //nolint:gosec // path provided by caller
	if err != nil {
		log.Warn("failed to read env file", "path", path, "error", err)
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		// The MYCEL_* namespace is reserved for the system — an env file
		// must not clobber MYCEL_AGENT_ID etc. any more than agent config.
		if strings.HasPrefix(key, "MYCEL_") {
			log.Warn("skipping reserved env var from env file", "path", path, "key", key)
			continue
		}
		env[key] = strings.TrimSpace(v)
	}
}
