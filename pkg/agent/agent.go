// Package agent provides agent lifecycle management for bc.
//
// An agent is an AI assistant running in an isolated tmux session with its own
// git worktree. Agents have roles (engineer, manager, etc.) that determine
// their capabilities and permissions.
//
// # Basic Usage
//
// Create an agent manager:
//
//	mgr := agent.NewWorkspaceManager(".bc/agents", "/path/to/workspace")
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
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/bc/pkg/container"
	"github.com/rpuneet/bc/pkg/db"
	"github.com/rpuneet/bc/pkg/log"
	"github.com/rpuneet/bc/pkg/names"
	"github.com/rpuneet/bc/pkg/provider"
	"github.com/rpuneet/bc/pkg/runtime"
	"github.com/rpuneet/bc/pkg/secret"
	"github.com/rpuneet/bc/pkg/tmux"
	"github.com/rpuneet/bc/pkg/workspace"
	"github.com/rpuneet/bc/pkg/worktree"
)

// MaxAgentNameLength is the maximum allowed length for an agent name.
const MaxAgentNameLength = 64

// Default configuration constants.
const (
	// DefaultSessionPrefix is the tmux session name prefix for bc agents.
	DefaultSessionPrefix = "bc-"

	// DefaultProvider is the default AI provider for new agents.
	DefaultProvider = "claude"

	// DefaultMaxLogBytes is the maximum log file size in bytes before lazy truncation.
	DefaultMaxLogBytes = 10 * 1024 * 1024 // 10MB
)

// IsValidAgentName validates that agent names contain only alphanumeric characters, hyphens, and underscores,
// and are at most MaxAgentNameLength characters long.
// This ensures agent names are safe for use in file paths, shell environments, and tmux sessions.
func IsValidAgentName(name string) bool {
	if name == "" {
		return false
	}
	if len(name) > MaxAgentNameLength {
		return false
	}
	for _, c := range name {
		isLower := c >= 'a' && c <= 'z'
		isUpper := c >= 'A' && c <= 'Z'
		isDigit := c >= '0' && c <= '9'
		isAllowed := isLower || isUpper || isDigit || c == '-' || c == '_'
		if !isAllowed {
			return false
		}
	}
	return true
}

// Role defines the type of agent.
type Role string

const (
	// RoleRoot is the only hardcoded role - a singleton root agent.
	// All other roles are defined in workspace .bc/roles/*.md files.
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
	PermModifyConfig Permission = "can_modify_config" // Can modify workspace config
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
// workspace .bc/roles/*.md files via RoleManager.
// Only the root role has hardcoded capabilities.
var RoleCapabilities = map[Role][]Capability{
	RoleRoot: {CapCreateAgents, CapAssignWork, CapCreateEpics, CapReviewWork}, // Root can do everything
}

var RoleHierarchy = map[Role][]Role{
	// Root can create any role defined in workspace (checked at runtime)
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
// Custom roles loaded from .bc/roles/*.md return level 1 by default.
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
// UpdateAgentState, which is called by bc report.
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
	UpdatedAt      time.Time    `json:"updated_at"`
	StartedAt      time.Time    `json:"started_at"`
	CreatedAt      time.Time    `json:"created_at"`
	StoppedAt      *time.Time   `json:"stopped_at,omitempty"`
	DeletedAt      *time.Time   `json:"deleted_at,omitempty"`
	RolePrompt     *AgentMemory `json:"memory,omitempty"`
	Workspace      string       `json:"workspace"`
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Task           string       `json:"task,omitempty"`
	Session        string       `json:"session"`
	SessionID      string       `json:"session_id,omitempty"` // For session resume (#1939)
	Tool           string       `json:"tool,omitempty"`
	ParentID       string       `json:"parent_id,omitempty"`
	HookedWork     string       `json:"hooked_work,omitempty"`
	WorktreeDir    string       `json:"worktree_dir,omitempty"`
	LogFile        string       `json:"log_file,omitempty"`
	Team           string       `json:"team,omitempty"`
	RecoveredFrom  string       `json:"recovered_from,omitempty"`
	EnvFile        string       `json:"env_file,omitempty"`
	RuntimeBackend string       `json:"runtime_backend,omitempty"`
	LastCrashTime  *time.Time   `json:"last_crash_time,omitempty"`
	Role           Role         `json:"role"`
	State          State        `json:"state"`
	Children       []string     `json:"children,omitempty"`
	CrashCount     int          `json:"crash_count,omitempty"`
	IsRoot         bool         `json:"is_root,omitempty"`
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

// LoadRoleMemory loads role-specific prompt content from .bc/roles/<role>.md.
// For the root role, loads from .bc/prompts/root.md for backward compatibility.
// Returns nil AgentMemory if the file doesn't exist.
func LoadRoleMemory(workspacePath string, role Role) *AgentMemory {
	// For root role, try backward compatible location first
	if role == RoleRoot {
		rootPromptPath := filepath.Join(workspacePath, "prompts", "root.md")
		//nolint:gosec // path constructed from trusted workspace root
		if data, err := os.ReadFile(rootPromptPath); err == nil {
			return &AgentMemory{
				RolePrompt: string(data),
				LoadedAt:   time.Now(),
			}
		}
	}

	// Load role from .bc/roles/<role>.md using RoleManager
	stateDir := filepath.Join(workspacePath, ".bc")
	rm := workspace.NewRoleManager(stateDir)
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
type Manager struct {
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

	// roleManager validates role existence (shared with workspace)
	roleManager *workspace.RoleManager

	defaultBackend string // "tmux" or "docker"
	stateDir       string

	// Agent command (e.g., "claude" or "claude --dangerously-skip-permissions")
	agentCmd string

	// defaultTool is the provider name for the default agentCmd (for BuildCommand)
	defaultTool string

	// Workspace path for env vars
	workspacePath string

	// BootstrapDelay is the time to wait before sending bootstrap prompts.
	// If zero, DefaultBootstrapDelay is used.
	BootstrapDelay time.Duration

	// maxLogBytes is the maximum log file size before truncation.
	// Defaults to DefaultMaxLogBytes; overridden by ApplyWorkspaceConfig.
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
func (m *Manager) SetRoleManager(rm *workspace.RoleManager) {
	m.roleManager = rm
}

// ApplyWorkspaceConfig applies workspace-level configuration overrides to the manager.
// This should be called after creating a manager to pick up workspace-specific settings.
func (m *Manager) ApplyWorkspaceConfig(cfg *workspace.Config) {
	if cfg == nil {
		return
	}
	if cfg.Logs.MaxBytes > 0 {
		m.maxLogBytes = cfg.Logs.MaxBytes
	}
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

// NewManager creates a new agent manager with workspace-scoped tmux sessions.
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

// NewWorkspaceManager creates an agent manager scoped to a workspace.
// Session names will be unique per workspace to avoid collisions.
func NewWorkspaceManager(stateDir, workspacePath string) *Manager {
	cmd, tool := defaultAgentCmd()
	tmuxBe := runtime.NewTmuxBackend(tmux.NewWorkspaceManager(DefaultSessionPrefix, workspacePath))
	return &Manager{
		agents:           make(map[string]*Agent),
		agentLocks:       make(map[string]*sync.Mutex),
		backends:         map[string]runtime.Backend{"tmux": tmuxBe},
		defaultBackend:   "tmux",
		providerRegistry: provider.DefaultRegistry,
		stateDir:         stateDir,
		agentCmd:         cmd,
		defaultTool:      tool,
		workspacePath:    workspacePath,
		maxLogBytes:      DefaultMaxLogBytes,
		worktreeMgr:      worktree.NewManager(workspacePath),
	}
}

// NewWorkspaceManagerWithRuntime creates an agent manager with a specific runtime backend.
// rtName should be "docker" or "tmux".
func NewWorkspaceManagerWithRuntime(stateDir, workspacePath string, rt runtime.Backend, rtName string) *Manager {
	cmd, tool := defaultAgentCmd()
	bes := map[string]runtime.Backend{rtName: rt}
	// Always register a tmux backend so agents with RuntimeBackend="tmux" work
	if rtName != "tmux" {
		bes["tmux"] = runtime.NewTmuxBackend(tmux.NewWorkspaceManager(DefaultSessionPrefix, workspacePath))
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
		workspacePath:    workspacePath,
		maxLogBytes:      DefaultMaxLogBytes,
		worktreeMgr:      worktree.NewManager(workspacePath),
	}
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

// getAgentCommand looks up the command for a tool from the manager's provider registry.
// SessionID takes priority over the resume flag when non-empty.
func (m *Manager) getAgentCommand(toolName, agentName string, resume bool, sessionID string) (string, bool) {
	if m.providerRegistry != nil {
		if p, ok := m.providerRegistry.Get(toolName); ok {
			return p.BuildCommand(provider.CommandOpts{
				AgentName: agentName,
				Resume:    resume,
				SessionID: sessionID,
			}), true
		}
	}
	return "", false
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
// checking workspace ProvidersConfig first, then falling back to global config.
// This enables per-workspace tool customization.
func GetAgentCommandFromConfig(toolName string, wsCfg *workspace.Config) (string, bool) {
	// Check workspace ProvidersConfig first
	if wsCfg != nil {
		if p := wsCfg.GetProvider(toolName); p != nil && p.Command != "" {
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
	Name      string
	Role      Role
	Workspace string
	ParentID  string
	Tool      string
	EnvFile   string
	Runtime   string // override runtime backend ("tmux" or "docker"); empty uses manager default
	Team      string // optional team assignment
	SessionID string // Explicit session ID to resume (overrides stored session_id)
}

// SpawnAgent creates and starts a new agent.
// Idempotent: if the agent already exists and its tmux session is alive, reuse it.
func (m *Manager) SpawnAgent(ctx context.Context, name string, role Role, workspace string) (*Agent, error) {
	return m.SpawnAgentWithOptions(ctx, SpawnOptions{Name: name, Role: role, Workspace: workspace})
}

// SpawnAgentWithTool creates and starts a new agent with a specific tool.
// If tool is empty, uses the manager's default agent command.
func (m *Manager) SpawnAgentWithTool(ctx context.Context, name string, role Role, workspace string, tool string) (*Agent, error) {
	return m.SpawnAgentWithOptions(ctx, SpawnOptions{Name: name, Role: role, Workspace: workspace, Tool: tool})
}

// SpawnAgentWithParent creates and starts a new agent with a parent relationship.
// Idempotent: if the agent already exists and its tmux session is alive, reuse it.
func (m *Manager) SpawnAgentWithParent(ctx context.Context, name string, role Role, workspace string, parentID string) (*Agent, error) {
	return m.SpawnAgentWithOptions(ctx, SpawnOptions{Name: name, Role: role, Workspace: workspace, ParentID: parentID})
}

// SpawnAgentWithOptions creates and starts a new agent with all options.
// If tool is empty, uses the manager's default agent command.
// Idempotent: if the agent already exists and its tmux session is alive, reuse it.
func (m *Manager) SpawnAgentWithOptions(ctx context.Context, opts SpawnOptions) (*Agent, error) {
	name := opts.Name
	role := opts.Role
	wsPath := opts.Workspace
	parentID := opts.ParentID

	m.mu.Lock()

	// Auto-generate name if empty
	if name == "" {
		existing := make(map[string]bool, len(m.agents))
		for n := range m.agents {
			existing[n] = true
		}
		generated, genErr := names.GenerateUnique(existing, 100)
		if genErr != nil {
			m.mu.Unlock()
			return nil, fmt.Errorf("failed to generate agent name: %w", genErr)
		}
		name = generated
		opts.Name = name
	}

	log.Debug("spawning agent", "name", name, "role", role, "workspace", wsPath, "parentID", parentID, "tool", opts.Tool)

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
	// (e.g., standalone agent manager without workspace). Built-in roles
	// like "root" are always valid.
	if role != RoleRoot && m.roleManager != nil {
		if !m.roleManager.HasRole(string(role)) {
			m.mu.Unlock()
			return nil, fmt.Errorf("role %q does not exist; create it via the API or in .bc/roles/%s.md", role, role)
		}
	}

	// Enforce root singleton constraint
	if role == RoleRoot {
		if err := m.enforceRootSingleton(wsPath); err != nil {
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
			if err := m.saveState(); err != nil {
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
	wsPath := opts.Workspace

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

	// Check if existing worktree can be reused for resume (tmux only)
	agentRuntime := existing.RuntimeBackend
	if m.worktreeMgr.Exists(name) && agentRuntime == "tmux" {
		// Worktree exists — check for active session conflict
		for beName, be := range m.backends {
			if be.HasSession(ctx, name) {
				m.mu.Unlock()
				return nil, fmt.Errorf("worktree for %s is already in use by active session on %s backend", name, beName)
			}
		}
		// Worktree exists with no active session — enable resume
		if !resume && sessionID == "" {
			resume = true
			log.Debug("worktree exists, will use --continue", "agent", name)
		}
	}

	toolName := existing.Tool
	if toolName == "" {
		toolName = m.defaultTool
	}
	agentCmd := m.agentCmd
	if toolName != "" {
		if cmd, ok := m.getAgentCommand(toolName, name, resume, sessionID); ok {
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

	env := map[string]string{
		"BC_AGENT_ID":      name,
		"BC_AGENT_ROLE":    string(existing.Role),
		"BC_WORKSPACE":     wsPath,
		"BC_AGENT_RUNTIME": agentRuntime,
		"BC_BCD_ADDR":      bcdAddrForRuntime(agentRuntime),
		"BC_WORKTREE_NAME": m.worktreeMgr.Name(name),
	}
	if toolName != "" {
		env["BC_AGENT_TOOL"] = toolName
	}
	if existing.ParentID != "" {
		env["BC_PARENT_ID"] = existing.ParentID
	}
	// Pass through BC_API_KEY from the host environment so agents inside
	// containers can authenticate back to bcd when --api-key is enabled.
	if apiKey := os.Getenv("BC_API_KEY"); apiKey != "" {
		env["BC_API_KEY"] = apiKey
	}
	injectEnv(env, wsPath, toolName, existing.EnvFile)

	rt := m.runtimeForAgent(name)
	m.mu.Unlock()

	// Phase 2: per-agent lock — slow I/O (create session, pipe-pane)
	agentLock := m.getAgentLock(name)
	agentLock.Lock()

	// Ensure worktree exists and is valid (may have been cleaned up, moved,
	// or corrupted by runtime changes like Docker→localhost migration).
	wtDir := existing.WorktreeDir
	needsRecreate := wtDir == "" || !m.worktreeMgr.Exists(name)

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
		_ = m.worktreeMgr.Remove(ctx, name) //nolint:errcheck
		var wtErr error
		wtDir, wtErr = m.worktreeMgr.Create(ctx, name)
		if wtErr != nil {
			agentLock.Unlock()
			return nil, fmt.Errorf("failed to create worktree for agent %s: %w", name, wtErr)
		}
		existing.WorktreeDir = wtDir
		log.Info("worktree recreated", "agent", name, "path", wtDir)
	}

	// Write hook settings and role files to worktree (regenerate on every start
	// so config changes like MCP URLs take effect without manual intervention).
	if err := WriteWorkspaceHookSettings(wtDir); err != nil {
		log.Error("failed to write hook settings", "dir", wtDir, "error", err)
	}
	if setupErr := SetupAgentFromRoleWithRuntime(wsPath, name, string(existing.Role), wtDir, agentRuntime, existing.Tool); setupErr != nil {
		log.Warn("role setup failed on restart", "agent", name, "error", setupErr)
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
		existing.LogFile = m.setupLogPipe(ctx, name, wsPath)
	}

	if existing.State == StateStopped || existing.State == StateError {
		existing.State = StateStarting
	}
	existing.UpdatedAt = time.Now()

	agentLock.Unlock()

	// Phase 3: global lock — persist state
	m.mu.Lock()
	if err := m.saveState(); err != nil {
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
	wsPath := opts.Workspace
	parentID := opts.ParentID
	tool := opts.Tool

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
		if cmd, ok := m.getAgentCommand(effectiveTool, name, false, ""); ok {
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
		Workspace:      wsPath,
		Session:        name,
		Tool:           effectiveTool,
		ParentID:       parentID,
		Team:           opts.Team,
		EnvFile:        opts.EnvFile,
		RuntimeBackend: agentRuntime,
		Children:       []string{},
		IsRoot:         role == RoleRoot,
		CreatedAt:      now,
		StartedAt:      now,
		UpdatedAt:      now,
	}

	// Register agent early so runtimeForAgent can resolve the correct backend
	m.agents[name] = agent

	// Build env vars so the spawned process sees them immediately
	env := map[string]string{
		"BC_AGENT_ID":      name,
		"BC_AGENT_ROLE":    string(role),
		"BC_WORKSPACE":     wsPath,
		"BC_AGENT_RUNTIME": agentRuntime,
		"BC_BCD_ADDR":      bcdAddrForRuntime(agentRuntime),
		"BC_WORKTREE_NAME": m.worktreeMgr.Name(name),
	}
	if effectiveTool != "" {
		env["BC_AGENT_TOOL"] = effectiveTool
	}
	if parentID != "" {
		env["BC_PARENT_ID"] = parentID
	}
	// Pass through BC_API_KEY from the host environment so agents inside
	// containers can authenticate back to bcd when --api-key is enabled.
	if apiKey := os.Getenv("BC_API_KEY"); apiKey != "" {
		env["BC_API_KEY"] = apiKey
	}
	injectEnv(env, wsPath, effectiveTool, opts.EnvFile)

	rt := m.runtimeForAgent(name)
	m.mu.Unlock()

	// Phase 2: per-agent lock — slow I/O (create session, role setup, log pipe)
	agentLock := m.getAgentLock(name)
	agentLock.Lock()

	// Create worktree for this agent
	wtDir, wtErr := m.worktreeMgr.Create(ctx, name)
	if wtErr != nil {
		agentLock.Unlock()
		m.mu.Lock()
		delete(m.agents, name)
		m.mu.Unlock()
		return nil, fmt.Errorf("create worktree: %w", wtErr)
	}
	agent.WorktreeDir = wtDir

	// Ensure Claude home dir exists
	if claudeErr := m.worktreeMgr.EnsureClaudeDir(name); claudeErr != nil {
		log.Warn("failed to ensure Claude dir", "agent", name, "error", claudeErr)
	}

	// Write hook settings to the worktree
	if err := WriteWorkspaceHookSettings(wtDir); err != nil {
		log.Warn("failed to write hook settings", "dir", wtDir, "error", err)
	}

	// Write role files (prompt, MCP, rules, etc.) to the worktree using provider adapter
	if setupErr := SetupAgentFromRoleWithRuntime(wsPath, name, string(role), wtDir, agentRuntime, effectiveTool); setupErr != nil {
		log.Warn("role setup failed", "agent", name, "error", setupErr)
		agent.Task = fmt.Sprintf("role setup failed: %v", setupErr)
	}

	// Validate required tools before starting — fail fast with clear errors.
	if toolErrs := validateAgentTools(wsPath, string(role)); len(toolErrs) > 0 {
		for _, te := range toolErrs {
			log.Warn("tool validation failed", "agent", name, "error", te)
		}
		agent.Task = fmt.Sprintf("tool validation: %d issue(s)", len(toolErrs))
		// Non-fatal: agent starts but issues are logged for visibility
	}

	// Create session IN the worktree directory
	if err := rt.CreateSessionWithEnv(ctx, name, wtDir, agentCmd, env); err != nil {
		agentLock.Unlock()
		m.mu.Lock()
		delete(m.agents, name)
		m.mu.Unlock()
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Start log streaming via pipe-pane
	agent.LogFile = m.setupLogPipe(ctx, name, wsPath)

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
	if err := m.saveState(); err != nil {
		log.Warn("failed to save agent state", "error", err)
	}
	m.mu.Unlock()

	return agent, nil
}

// setupLogPipe creates the logs directory and starts pipe-pane for the agent.
// Returns the log file path.
func (m *Manager) setupLogPipe(ctx context.Context, name, workspace string) string {
	logsDir := filepath.Join(workspace, ".bc", "logs")
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		log.Warn("failed to create logs dir", "error", err)
		return ""
	}

	logPath := filepath.Join(logsDir, name+".log")

	// Truncate if over max size
	truncateLogFile(logPath, m.maxLogBytes)

	if err := m.runtimeForAgent(name).PipePane(ctx, name, logPath); err != nil {
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

	info, err := os.Stat(path)
	if err != nil || info.Size() <= maxBytes {
		return
	}

	data, err := os.ReadFile(path) //nolint:gosec // path constructed from trusted workspace root
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

	if err := os.WriteFile(path, data[half:], 0600); err != nil { //nolint:gosec // path constructed from trusted workspace root
		log.Warn("failed to truncate log", "path", path, "error", err)
	}
}

// SpawnChildAgent creates a child agent under a parent agent.
// Validates that the parent has permission to create the child role.
func (m *Manager) SpawnChildAgent(ctx context.Context, parentID, childName string, childRole Role, workspace string) (*Agent, error) {
	return m.SpawnAgentWithOptions(ctx, SpawnOptions{Name: childName, Role: childRole, Workspace: workspace, ParentID: parentID})
}

// SpawnChildAgentWithTool creates a child agent under a parent agent with a specific tool.
// Validates that the parent has permission to create the child role.
func (m *Manager) SpawnChildAgentWithTool(ctx context.Context, parentID, childName string, childRole Role, workspace, tool string) (*Agent, error) {
	return m.SpawnAgentWithOptions(ctx, SpawnOptions{Name: childName, Role: childRole, Workspace: workspace, ParentID: parentID, Tool: tool})
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
	// Claude Code writes transcripts to .bc/agents/<name>/claude/projects/*/<uuid>.jsonl
	// where the UUID IS the session ID.
	if id := findSessionIDFromTranscripts(m.stateDir, ag.Name); id != "" {
		log.Debug("captured session ID from JSONL transcript", "agent", ag.Name, "session_id", id)
		return id
	}

	return ""
}

// findSessionIDFromTranscripts scans the agent's Claude projects directory
// for the most recent .jsonl transcript and extracts the session ID from
// the filename (which is the UUID session ID).
func findSessionIDFromTranscripts(stateDir, agentName string) string {
	projectsDir := filepath.Join(stateDir, "agents", agentName, "claude", "projects")
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

// writeSessionIDFile persists the session ID to a plain-text file and archives
// it in the session history directory alongside a timestamp.
// Permissions are 0600 (session IDs may grant conversation access).
func writeSessionIDFile(stateDir, agentName, sessionID string) {
	agentDir := filepath.Join(stateDir, "agents", agentName)
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
		return fmt.Errorf("agent %s not found", name)
	}
	rt := m.runtimeForAgent(name)
	stateDir := m.stateDir
	m.mu.RUnlock()

	// Phase 2: per-agent lock — slow I/O (capture session ID, kill session)
	agentLock := m.getAgentLock(name)
	agentLock.Lock()

	// Capture session ID from output before killing the session.
	if sessionID := m.captureSessionIDForAgent(ctx, agent, rt); sessionID != "" {
		agent.SessionID = sessionID
		writeSessionIDFile(stateDir, name, sessionID)
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
	if err := m.saveState(); err != nil {
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
	if err := m.saveState(); err != nil {
		log.Warn("failed to save agent state after tree stop", "error", err)
	}
	m.mu.Unlock()

	return nil
}

// collectAgentTree collects all agents in a tree depth-first. Must be called with m.mu held.
func (m *Manager) collectAgentTree(name string) ([]agentTreeEntry, error) {
	agent, exists := m.agents[name]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", name)
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

// DeleteAgent permanently removes an agent from the workspace.
func (m *Manager) DeleteAgent(ctx context.Context, name string) error {
	return m.DeleteAgentWithOptions(ctx, name, DeleteOptions{})
}

// DeleteAgentWithOptions permanently removes an agent with configurable options.
// Cleans up all resources: container, volume, worktree, git branch, log file,
// agent state directory, channel memberships, and child agent references.
// Partial failures are logged but do not abort the deletion.
func (m *Manager) DeleteAgentWithOptions(ctx context.Context, name string, opts DeleteOptions) error {
	log.Debug("deleting agent", "name", name)

	// Phase 1: global lock — validate agent exists, snapshot references
	m.mu.RLock()
	agent, exists := m.agents[name]
	if !exists {
		m.mu.RUnlock()
		return fmt.Errorf("agent %s not found", name)
	}
	rt := m.runtimeForAgent(name)
	workspacePath := m.workspacePath
	stateDir := m.stateDir
	logFile := agent.LogFile
	m.mu.RUnlock()

	// Phase 2: per-agent lock — slow I/O (kill session, remove container, git cleanup)
	agentLock := m.getAgentLock(name)
	agentLock.Lock()

	// 1. Stop the container/session
	_ = rt.KillSession(ctx, name) //nolint:errcheck // may already be stopped

	// 2. Remove the container entirely (for Docker agents)
	if cb, ok := rt.(*container.Backend); ok {
		_ = cb.RemoveSession(ctx, name) //nolint:errcheck // may not exist
	}

	// 3. Remove persistent volume (.bc/volumes/<name>/)
	volumeDir := filepath.Join(workspacePath, ".bc", "volumes", name)
	if err := os.RemoveAll(volumeDir); err != nil {
		log.Warn("delete: failed to remove agent volume", "agent", name, "error", err)
	}

	// 4. Remove git worktree
	if err := m.worktreeMgr.Remove(ctx, name); err != nil {
		log.Warn("failed to remove worktree", "agent", name, "error", err)
	}

	// 5. Remove log file
	if logFile != "" {
		if err := os.Remove(logFile); err != nil && !os.IsNotExist(err) {
			log.Warn("delete: failed to remove log file", "agent", name, "path", logFile, "error", err)
		}
	}

	// 6. Remove agent state directory (.bc/agents/<name>/ — auth, session history, etc.)
	agentStateDir := filepath.Join(stateDir, "agents", name)
	if err := os.RemoveAll(agentStateDir); err != nil {
		log.Warn("delete: failed to remove agent state dir", "agent", name, "path", agentStateDir, "error", err)
	}

	agentLock.Unlock()

	// Phase 3: global lock — update maps, orphan children, persist
	m.mu.Lock()

	// 7. Update children's ParentID to "" (orphan them cleanly)
	for _, childName := range agent.Children {
		if child, ok := m.agents[childName]; ok {
			child.ParentID = ""
			child.UpdatedAt = time.Now()
		}
	}

	// 8. Remove from parent's children list
	m.removeFromParent(name)

	// 9. Soft-delete in SQLite first (set deleted_at) so the agent won't be
	// resurrected by LoadAll even if bcd crashes before the hard delete.
	if m.store != nil {
		if err := m.store.SoftDelete(name); err != nil {
			log.Warn("delete: failed to soft-delete agent in store", "agent", name, "error", err)
		}
	}

	// 10. Delete from state map and clean up per-agent lock
	delete(m.agents, name)
	delete(m.agentLocks, name)

	if err := m.saveState(); err != nil {
		log.Warn("delete: failed to save state", "agent", name, "error", err)
	}

	// 11. Hard-delete the row from SQLite. The soft-delete above already
	// prevents resurrection; this removes the row entirely for cleanliness.
	if m.store != nil {
		if err := m.store.Delete(name); err != nil {
			log.Warn("delete: failed to remove agent from store", "agent", name, "error", err)
		}
	}
	m.mu.Unlock()

	log.Debug("agent fully deleted", "agent", name, "volume", volumeDir)
	return nil
}

// RenameAgent renames an agent from oldName to newName.
func (m *Manager) RenameAgent(ctx context.Context, oldName, newName string) error {
	if !IsValidAgentName(newName) {
		return fmt.Errorf("agent name %q is invalid: use letters, numbers, dash, underscore (max %d chars)", newName, MaxAgentNameLength)
	}

	// Phase 1: validate under global lock, snapshot agent
	m.mu.Lock()
	agent, exists := m.agents[oldName]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("agent %s not found", oldName)
	}
	if _, newExists := m.agents[newName]; newExists {
		m.mu.Unlock()
		return fmt.Errorf("agent %s already exists", newName)
	}
	// Agent must be stopped — rename while running is unsafe
	if agent.State != StateStopped && agent.State != StateError {
		m.mu.Unlock()
		return fmt.Errorf("agent %q must be stopped before renaming (state: %s)", oldName, agent.State)
	}
	rt := m.runtimeForAgent(oldName)
	m.mu.Unlock()

	// Phase 2: slow I/O under per-agent lock
	agentLock := m.getAgentLock(oldName)
	agentLock.Lock()

	log.Debug("renaming agent", "oldName", oldName, "newName", newName)

	// Rename runtime session (tmux rename-session / docker rename)
	if err := rt.RenameSession(ctx, oldName, newName); err != nil {
		log.Warn("rename: failed to rename runtime session", "error", err)
		// Non-fatal — session may already be dead (agent is stopped)
	}

	// Move worktree directory to preserve content instead of Remove+Create
	oldPath := m.worktreeMgr.Path(oldName)
	newPath := m.worktreeMgr.Path(newName)
	var newWorktreeDir string
	if err := os.MkdirAll(filepath.Dir(newPath), 0750); err != nil {
		log.Warn("rename: failed to create new agent dir", "error", err)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		log.Warn("rename: failed to move worktree", "error", err)
		// Fall back to create new
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

	// Regenerate .mcp.json with the new agent name so MCP SSE URLs
	// point to /_mcp/{newName}/sse instead of /_mcp/{oldName}/sse.
	if newWorktreeDir != "" && agent.Role != "" {
		wsPath := m.workspacePath
		agentRuntime := agent.RuntimeBackend
		if agentRuntime == "" {
			agentRuntime = "tmux"
		}
		if setupErr := SetupAgentFromRoleWithRuntime(wsPath, newName, string(agent.Role), newWorktreeDir, agentRuntime, agent.Tool); setupErr != nil {
			log.Warn("rename: failed to regenerate role files", "agent", newName, "error", setupErr)
		}
	}

	// Rename log file
	oldLogDir := filepath.Join(m.workspacePath, ".bc", "logs")
	oldLogFile := filepath.Join(oldLogDir, oldName+".log")
	newLogFile := filepath.Join(oldLogDir, newName+".log")
	if err := os.Rename(oldLogFile, newLogFile); err != nil && !os.IsNotExist(err) {
		log.Warn("rename: failed to rename log file", "error", err)
	}

	// Rename agent state directory
	oldStateDir := filepath.Join(m.stateDir, "agents", oldName)
	newStateDir := filepath.Join(m.stateDir, "agents", newName)
	if err := os.Rename(oldStateDir, newStateDir); err != nil && !os.IsNotExist(err) {
		log.Warn("rename: failed to rename state dir", "error", err)
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

	if err := m.saveState(); err != nil {
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

	if err := m.saveState(); err != nil {
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

// UpdateAgentState updates an agent's state and task.
// Returns an error if the transition is invalid per the state machine.
func (m *Manager) UpdateAgentState(name string, state State, task string) error {
	var changed bool

	m.mu.Lock()
	agent, exists := m.agents[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("agent %s not found", name)
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

	if err := m.saveState(); err != nil {
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
func (m *Manager) SetAgentTeam(name, team string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, exists := m.agents[name]
	if !exists {
		return fmt.Errorf("agent %s not found", name)
	}

	agent.Team = team
	agent.UpdatedAt = time.Now()

	if err := m.saveState(); err != nil {
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
	return m.runtimeForAgent(name).Capture(ctx, name, lines)
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
		return fmt.Errorf("agent %q not found", name)
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
func (m *Manager) saveState() error {
	if m.store == nil {
		return nil
	}
	return m.store.SaveAll(m.agents)
}

// LoadState loads agent state from SQLite.
// On first run after upgrade, migrates JSON files to SQLite automatically.
func (m *Manager) LoadState() error {
	if m.stateDir == "" {
		return nil
	}

	// Open SQLite store — use the unified bc.db when workspace path is known,
	// otherwise fall back to state.db in the agents dir (tests / standalone).
	var dbPath string
	if m.workspacePath != "" {
		dbPath = db.BCDBPath(m.workspacePath)
	} else {
		dbPath = filepath.Join(m.stateDir, "state.db")
	}
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("open agent store: %w", err)
	}
	m.store = store

	// Auto-migrate JSON files if they exist
	if needsMigration(m.stateDir) {
		log.Info("migrating agent state from JSON to SQLite")
		if migErr := migrateJSONToSQLite(store, m.stateDir, m.workspacePath); migErr != nil {
			log.Warn("migration had errors", "error", migErr)
		}
	}

	// Load all agents from SQLite
	agents, err := store.LoadAll()
	if err != nil {
		return fmt.Errorf("load agents: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents = agents
	return nil
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

// Tmux returns the underlying tmux manager if the default backend is tmux.
// Deprecated: Use Runtime() instead. This is kept for backward compatibility.
func (m *Manager) Tmux() *tmux.Manager {
	if tb, ok := m.runtime().(*runtime.TmuxBackend); ok {
		return tb.TmuxManager()
	}
	return nil
}

// QueryAgentStats returns up to limit recent stats records for the named agent.
func (m *Manager) QueryAgentStats(agentName string, limit int) ([]*AgentStatsRecord, error) {
	if m.store == nil {
		return nil, fmt.Errorf("no store available")
	}
	return m.store.QueryStats(agentName, limit)
}

// RecordAgentStats persists a single AgentStatsRecord to the SQLite store.
// This is used by the background container metrics collector to save Docker
// resource samples so the /api/agents/{name}/stats endpoint returns real data.
func (m *Manager) RecordAgentStats(rec *AgentStatsRecord) error {
	if m.store == nil {
		return fmt.Errorf("no store available")
	}
	return m.store.SaveStats(rec)
}

// Close closes the SQLite store. Call when done with the manager.
func (m *Manager) Close() error {
	if m.store != nil {
		return m.store.Close()
	}
	return nil
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

// bcdAddrForRuntime returns the bcd server address for the given runtime.
// Docker containers reach the host via host.docker.internal.
// If BC_BCD_ADDR is set in the environment, it is used as the base address
// (with host.docker.internal substituted for Docker runtimes).
func bcdAddrForRuntime(rt string) string {
	if addr := os.Getenv("BC_BCD_ADDR"); addr != "" {
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

// injectEnv merges environment variables from the agent env file
// and resolves ${secret:NAME} references.
func injectEnv(env map[string]string, workspacePath, _, envFile string) {
	// Agent env file
	if envFile != "" {
		parseEnvFile(env, envFile)
	}
	// Resolve ${secret:NAME} references in all env values
	resolveSecretRefs(env, workspacePath)
}

// resolveSecretRefs resolves ${secret:NAME} references in env values using the
// workspace secret store. If the store cannot be opened, references are left as-is.
func resolveSecretRefs(env map[string]string, workspacePath string) {
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

	store, err := secret.NewStore(workspacePath, passphrase)
	if err != nil {
		log.Warn("failed to open secret store for env resolution", "error", err)
		return
	}
	defer func() { _ = store.Close() }()

	resolved := store.ResolveEnv(env)
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
		env[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
}
