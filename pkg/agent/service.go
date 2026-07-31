package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/names"
)

// EventPublisher is the interface for publishing agent lifecycle events.
type EventPublisher interface {
	Publish(eventType string, data map[string]any)
}

// CostQuerier is the interface for querying agent cost data.
type CostQuerier interface {
	AgentCostSummary(agentID string) (*CostSummary, error)
}

// CostSummary holds cost breakdown for an agent.
type CostSummary struct {
	AgentID      string  `json:"agent_id"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	RequestCount int64   `json:"request_count"`
}

// ListOptions configures agent listing.
type ListOptions struct {
	Role            string // Filter by role (empty = all)
	Status          string // Filter by status/state (empty = all)
	IncludeArchived bool   // When true, archived agents are included alongside live ones
	OnlyArchived    bool   // When true, only archived agents are returned (overrides IncludeArchived)
}

// CreateOptions holds parameters for creating an agent via the service.
type CreateOptions struct {
	// Env holds user-configured environment variables for the agent.
	// Values may contain ${secret:NAME} references; they are stored
	// verbatim and resolved against the vault at spawn time.
	Env     map[string]string
	Name    string
	Role    Role
	Tool    string
	Model   string // provider model identifier; empty uses the provider default
	EnvFile string
	Runtime string
	Parent  string
	Team    string
	// Repo is the absolute path of the git repo the agent is bound to.
	// Empty means "the repo the daemon was booted against" (the default repo).
	Repo string
}

// StartOptions configures agent start behavior.
type StartOptions struct {
	Runtime  string // Runtime backend override
	ResumeID string // Explicit session ID to resume
}

// SessionEntry represents a single session history record.
type SessionEntry struct {
	Timestamp time.Time `json:"timestamp,omitempty"`
	ID        string    `json:"id"`
	Current   bool      `json:"current,omitempty"`
}

// SendResult holds the result of a broadcast/role/pattern send operation.
type SendResult struct {
	Matched []string `json:"matched"`
	Sent    int      `json:"sent"`
	Skipped int      `json:"skipped"`
	Failed  int      `json:"failed"`
}

// AgentService provides the application-level API for agent management.
// It wraps Manager with event publishing and cost queries.
// This is the boundary that the daemon (issue #1938) will use.
type AgentService struct {
	manager *Manager
	events  EventPublisher
	costs   CostQuerier
	// hookStore persists ingested hook events (see IngestHookEvent).
	hookStore HookEventAppender
	// onHookEvent is invoked after each successfully ingested hook event
	// with the SSE-shaped payload (see SetOnHookEvent).
	onHookEvent func(agentName string, ts time.Time, payload map[string]any)
}

// NewAgentService creates a new agent service wrapping the given manager.
// It registers a state-change callback on the manager so that ongoing
// state transitions (hook events) are published as SSE events.
func NewAgentService(mgr *Manager, events EventPublisher, costs CostQuerier) *AgentService {
	svc := &AgentService{
		manager: mgr,
		events:  events,
		costs:   costs,
	}

	// Wire the manager's state-change callback to publish SSE events.
	mgr.SetOnStateChange(func(name string, state State, task string) {
		svc.publishEvent("agent.state_changed", map[string]any{
			"name":  name,
			"state": string(state),
			"task":  task,
		})
	})

	return svc
}

// Manager returns the underlying agent manager.
func (s *AgentService) Manager() *Manager {
	return s.manager
}

// List returns agents matching the given options.
func (s *AgentService) List(ctx context.Context, opts ListOptions) ([]*Agent, error) {
	agents := s.manager.ListAgents()

	// Fast path: no filters set and default archive behavior (hide).
	if opts.Role == "" && opts.Status == "" && !opts.IncludeArchived && !opts.OnlyArchived {
		filtered := make([]*Agent, 0, len(agents))
		for _, a := range agents {
			if a.ArchivedAt != nil {
				continue
			}
			filtered = append(filtered, a)
		}
		return filtered, nil
	}

	filtered := make([]*Agent, 0, len(agents))
	for _, a := range agents {
		archived := a.ArchivedAt != nil
		switch {
		case opts.OnlyArchived && !archived:
			continue
		case !opts.OnlyArchived && !opts.IncludeArchived && archived:
			continue
		}
		if opts.Role != "" && string(a.Role) != opts.Role {
			continue
		}
		if opts.Status != "" && !matchesStatus(a.State, opts.Status) {
			continue
		}
		filtered = append(filtered, a)
	}
	return filtered, nil
}

// Archive marks the named agent as archived, hiding it from default
// List() results. Idempotent. Errors when the agent doesn't exist, or
// when the agent is still running — archiving a live agent leaves the
// runtime in a confusing state, so callers must stop it first.
func (s *AgentService) Archive(ctx context.Context, name string) error {
	a := s.manager.GetAgent(name)
	if a == nil {
		return fmt.Errorf("agent %q: %w", name, ErrNotFound)
	}
	if a.ArchivedAt == nil && isRunningState(a.State) {
		return fmt.Errorf("cannot archive agent %q while running (state=%s); stop it first: %w", name, a.State, ErrAlreadyRunning)
	}
	return s.manager.SetArchived(ctx, name, true)
}

// isRunningState reports whether a state represents an agent that is
// actively running (either idle/waiting, starting, or working).
func isRunningState(state State) bool {
	return state == StateIdle || state == StateStarting || state == StateWorking
}

// Unarchive clears the archived flag on the named agent.
// Idempotent. Errors when the agent doesn't exist.
func (s *AgentService) Unarchive(ctx context.Context, name string) error {
	return s.manager.SetArchived(ctx, name, false)
}

// matchesStatus checks if an agent state matches a status filter.
// Maps the detailed internal states to the simplified 4-state model.
func matchesStatus(state State, status string) bool {
	switch status {
	case "running":
		return state == StateIdle || state == StateWorking || state == StateStarting
	case "stopped":
		return state == StateStopped
	case "error":
		return state == StateError
	case "starting":
		return state == StateStarting
	default:
		// Allow matching by exact internal state name too
		return string(state) == status
	}
}

// Create creates a new agent.
func (s *AgentService) Create(ctx context.Context, opts CreateOptions) (*Agent, error) {
	repo := opts.Repo
	if repo == "" {
		repo = s.manager.repoPath
	}
	a, err := s.manager.SpawnAgentWithOptions(ctx, SpawnOptions{
		Name:      opts.Name,
		Role:      opts.Role,
		Workspace: repo,
		ParentID:  opts.Parent,
		Tool:      opts.Tool,
		Model:     opts.Model,
		EnvFile:   opts.EnvFile,
		Env:       opts.Env,
		Runtime:   opts.Runtime,
		Team:      opts.Team,
	})
	if err != nil {
		return nil, err
	}

	s.publishEvent("agent.created", map[string]any{
		"name": a.Name,
		"role": string(a.Role),
		"tool": a.Tool,
	})

	return a, nil
}

// Start starts a stopped agent, optionally with a fresh session.
func (s *AgentService) Start(ctx context.Context, name string, opts StartOptions) (*Agent, error) {
	existing := s.manager.GetAgent(name)
	if existing == nil {
		return nil, fmt.Errorf("agent %q: %w", name, ErrNotFound)
	}

	if existing.State != StateStopped && existing.State != StateError {
		// Reconcile: container may have died without the daemon noticing
		if !s.manager.runtimeForAgent(name).HasSession(ctx, name) {
			log.Info("reconciling dead agent for restart", "agent", name, "state", existing.State)
			existing.State = StateStopped
			existing.UpdatedAt = time.Now()
			_ = s.manager.saveState(ctx)
		} else {
			return nil, fmt.Errorf("agent %q is already running (state: %s): %w", name, existing.State, ErrAlreadyRunning)
		}
	}

	// Respawn in the repo the agent is bound to, not the boot repo.
	repo := existing.Repo
	if repo == "" {
		repo = s.manager.repoPath
	}
	a, err := s.manager.SpawnAgentWithOptions(ctx, SpawnOptions{
		Name:      name,
		Role:      existing.Role,
		Workspace: repo,
		ParentID:  existing.ParentID,
		Tool:      existing.Tool,
		Model:     existing.Model,
		EnvFile:   existing.EnvFile,
		Runtime:   opts.Runtime,
		SessionID: opts.ResumeID,
	})
	if err != nil {
		return nil, err
	}

	s.publishEvent("agent.started", map[string]any{
		"name":       a.Name,
		"session_id": a.SessionID,
	})

	return a, nil
}

// Stop stops a running agent.
func (s *AgentService) Stop(ctx context.Context, name string) error {
	if err := s.manager.StopAgent(ctx, name); err != nil {
		return err
	}

	s.publishEvent("agent.stopped", map[string]any{
		"name":   name,
		"reason": "user_request",
	})

	return nil
}

// Delete permanently removes an agent. Agent must be stopped first unless force is true.
func (s *AgentService) Delete(ctx context.Context, name string, force bool) error {
	a := s.manager.GetAgent(name)
	if a == nil {
		return fmt.Errorf("agent %q: %w", name, ErrNotFound)
	}
	if !force && a.State != StateStopped {
		// Reconcile: container may have died without the daemon noticing
		if !s.manager.runtimeForAgent(name).HasSession(ctx, name) {
			a.State = StateStopped
			a.UpdatedAt = time.Now()
			_ = s.manager.saveState(ctx)
		} else {
			return fmt.Errorf("agent %q must be stopped before deletion (state: %s). Use ?force=true to delete anyway: %w", name, a.State, ErrAlreadyRunning)
		}
	}

	// Force: stop first if still running
	if force && a.State != StateStopped {
		if err := s.manager.StopAgent(ctx, name); err != nil {
			log.Warn("force delete: failed to stop agent", "agent", name, "error", err)
		}
	}

	if err := s.manager.DeleteAgent(ctx, name); err != nil {
		return err
	}

	s.publishEvent("agent.deleted", map[string]any{
		"name": name,
	})

	return nil
}

// Send sends a message to a running agent.
func (s *AgentService) Send(ctx context.Context, name, message string) error {
	a := s.manager.GetAgent(name)
	if a == nil {
		return fmt.Errorf("agent %q: %w", name, ErrNotFound)
	}
	// Reconcile stale state (#3175). Two wedges are common:
	//
	//   - marked stopped but the tmux session is actually alive → idle
	//   - stuck at "starting" because the transition hook was missed
	//     (crash before writing state, hook dropped, etc.) → idle
	//
	// In both cases we probe the runtime for a live session. If one
	// exists we correct the visible state so the dashboard stops lying;
	// if not we return the honest "not running" error.
	if a.State == StateStopped || a.State == StateStarting {
		if s.manager.RuntimeForAgent(name).HasSession(ctx, name) {
			// GetAgent returned a copy — reset via UpdateAgentState so
			// the manager's authoritative state actually moves.
			if uErr := s.manager.UpdateAgentState(ctx, name, StateIdle, a.Task); uErr != nil {
				log.Warn("reconcile to idle failed", "agent", name, "from", a.State, "error", uErr)
			}
		} else if a.State == StateStopped {
			return fmt.Errorf("agent %q is stopped: %w", name, ErrNotRunning)
		} else {
			return fmt.Errorf("agent %q is still starting: %w", name, ErrNotRunning)
		}
	}
	return s.manager.SendToAgent(ctx, name, message)
}

// SendAll broadcasts a message to all running agents.
func (s *AgentService) SendAll(ctx context.Context, message string) (int, error) {
	agents := s.manager.ListAgents()
	sent := 0
	for _, a := range agents {
		if a.State == StateStopped || a.State == StateError {
			continue
		}
		if err := s.manager.SendToAgent(ctx, a.Name, message); err != nil {
			log.Warn("send-all: failed", "agent", a.Name, "error", err)
			continue
		}
		sent++
	}
	return sent, nil
}

// Peek returns recent output from an agent.
func (s *AgentService) Peek(ctx context.Context, name string, lines int) (string, error) {
	a := s.manager.GetAgent(name)
	if a == nil {
		return "", fmt.Errorf("agent %q: %w", name, ErrNotFound)
	}
	return s.manager.CaptureOutput(ctx, name, lines)
}

// Cost returns the cost summary for an agent.
func (s *AgentService) Cost(ctx context.Context, name string) (*CostSummary, error) {
	if s.costs == nil {
		return nil, fmt.Errorf("cost tracking not configured")
	}
	return s.costs.AgentCostSummary(name)
}

// Broadcast sends a message to all running agents.
// Returns the number of agents the message was sent to.
func (s *AgentService) Broadcast(ctx context.Context, message string) (int, error) {
	agents := s.manager.ListAgents()
	sent := 0
	for _, a := range agents {
		if a.State == StateStopped || a.State == StateError {
			continue
		}
		if err := s.manager.SendToAgent(ctx, a.Name, message); err != nil {
			log.Warn("broadcast: failed to send to agent", "agent", a.Name, "error", err)
			continue
		}
		sent++
	}
	return sent, nil
}

// SetEnv replaces an agent's configured environment variables. Values
// with ${secret:NAME} references are stored verbatim; they resolve at
// spawn, so edits apply on the agent's next restart.
func (s *AgentService) SetEnv(ctx context.Context, name string, env map[string]string) error {
	if err := s.manager.SetAgentEnv(ctx, name, env); err != nil {
		return err
	}
	// Publish keys only — env values may hold sensitive material.
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	s.publishEvent("agent.env_updated", map[string]any{
		"name": name,
		"keys": keys,
	})
	return nil
}

// Get returns a single agent by name.
func (s *AgentService) Get(ctx context.Context, name string) (*Agent, error) {
	a := s.manager.GetAgent(name)
	if a == nil {
		return nil, fmt.Errorf("agent %q: %w", name, ErrNotFound)
	}
	return a, nil
}

func (s *AgentService) publishEvent(eventType string, data map[string]any) {
	if s.events != nil {
		s.events.Publish(eventType, data)
	}
}

// SyncSessions reconciles in-memory agent state with actual runtime sessions.
// For each agent that is not already stopped or in error, it checks whether
// the underlying tmux/docker session still exists. If the session is gone it
// marks the agent as stopped.
// Returns the total number of agents inspected (synced) and the number that
// were transitioned to stopped (stopped).
func (s *AgentService) SyncSessions(ctx context.Context) (synced, stopped int) {
	agents := s.manager.ListAgents()
	for _, a := range agents {
		if a.State == StateStopped || a.State == StateError {
			continue
		}
		synced++
		rt := s.manager.RuntimeForAgent(a.Name)
		if rt.HasSession(ctx, a.Name) {
			continue
		}
		// Session gone — mark stopped.
		if err := s.manager.UpdateAgentState(ctx, a.Name, StateStopped, ""); err != nil {
			log.Warn("sync: failed to update agent state", "agent", a.Name, "error", err)
			continue
		}
		stopped++
		s.publishEvent("agent.state_changed", map[string]any{
			"name":   a.Name,
			"state":  string(StateStopped),
			"reason": "session_gone",
		})
	}
	return synced, stopped
}

// StopAll stops all running agents. Returns count of agents stopped.
func (s *AgentService) StopAll(ctx context.Context) (int, error) {
	agents := s.manager.ListAgents()
	count := 0
	for _, a := range agents {
		if a.State != StateStopped && a.State != StateError {
			count++
		}
	}
	if err := s.manager.StopAll(ctx); err != nil {
		return 0, err
	}
	s.publishEvent("agents.stopped_all", map[string]any{"count": count})
	return count, nil
}

// Rename renames an agent.
func (s *AgentService) Rename(ctx context.Context, oldName, newName string) error {
	if err := s.manager.RenameAgent(ctx, oldName, newName); err != nil {
		return err
	}
	s.publishEvent("agent.renamed", map[string]any{
		"old_name": oldName,
		"new_name": newName,
	})
	return nil
}

// Sessions returns session history for an agent. Every agent (root or not)
// has its own worktree; sessions are looked up uniformly by WorktreeDir
// encoded to the Claude CLI projects format (abs path slashes → dashes)
// under ~/.claude/projects/<encoded>/*.jsonl.
func (s *AgentService) Sessions(_ context.Context, name string) ([]SessionEntry, error) {
	a := s.manager.GetAgent(name)
	if a == nil {
		return nil, fmt.Errorf("agent %q: %w", name, ErrNotFound)
	}

	var entries []SessionEntry
	seen := make(map[string]bool)
	if a.SessionID != "" {
		entries = append(entries, SessionEntry{ID: a.SessionID, Current: true})
		seen[a.SessionID] = true
	}

	if a.WorktreeDir == "" {
		return entries, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return entries, nil
	}
	// Claude CLI encodes the abs worktree path by replacing both
	// '/' and '.' with '-', e.g. '/Users/p/.mycel/x' → '-Users-p--bc-x'.
	encoded := strings.ReplaceAll(a.WorktreeDir, "/", "-")
	encoded = strings.ReplaceAll(encoded, ".", "-")
	projDir := filepath.Join(home, ".claude", "projects", encoded)
	files, err := os.ReadDir(projDir)
	if err != nil {
		return entries, nil
	}
	type fileEntry struct {
		mod time.Time
		id  string
	}
	var found []fileEntry
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(f.Name(), ".jsonl")
		if seen[id] {
			continue
		}
		seen[id] = true
		info, infoErr := f.Info()
		if infoErr != nil {
			continue
		}
		found = append(found, fileEntry{id: id, mod: info.ModTime()})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].mod.After(found[j].mod) })
	for _, fe := range found {
		entries = append(entries, SessionEntry{ID: fe.id, Timestamp: fe.mod})
	}
	return entries, nil
}

// SendToRole sends a message to all running agents with the given role.
func (s *AgentService) SendToRole(ctx context.Context, role, message string) (SendResult, error) {
	agents := s.manager.ListAgents()
	result := SendResult{}
	for _, a := range agents {
		if string(a.Role) != role {
			continue
		}
		result.Matched = append(result.Matched, a.Name)
		if a.State == StateStopped || a.State == StateError {
			result.Skipped++
			continue
		}
		if err := s.manager.SendToAgent(ctx, a.Name, message); err != nil {
			log.Warn("send-role: failed to send", "agent", a.Name, "error", err)
			result.Failed++
			continue
		}
		result.Sent++
	}
	return result, nil
}

// SendToPattern sends a message to all agents whose names match the given glob pattern.
func (s *AgentService) SendToPattern(ctx context.Context, pattern, message string) (SendResult, error) {
	agents := s.manager.ListAgents()
	result := SendResult{}
	for _, a := range agents {
		match, matchErr := filepath.Match(pattern, a.Name)
		if matchErr != nil {
			return result, fmt.Errorf("invalid pattern %q: %w", pattern, matchErr)
		}
		if !match {
			continue
		}
		result.Matched = append(result.Matched, a.Name)
		if a.State == StateStopped || a.State == StateError {
			result.Skipped++
			continue
		}
		if err := s.manager.SendToAgent(ctx, a.Name, message); err != nil {
			log.Warn("send-pattern: failed to send", "agent", a.Name, "error", err)
			result.Failed++
			continue
		}
		result.Sent++
	}
	return result, nil
}

// GenerateName generates a unique agent name not already in use.
func (s *AgentService) GenerateName(ctx context.Context) (string, error) {
	agents := s.manager.ListAgents()
	existing := make([]string, 0, len(agents))
	for _, a := range agents {
		existing = append(existing, a.Name)
	}
	return names.GenerateUniqueFromList(existing, 20)
}

// ForkAgent creates a new stopped agent by copying the source agent's worktree
// config files (CLAUDE.md, .mcp.json). The forked agent starts in stopped state.
func (s *AgentService) ForkAgent(ctx context.Context, sourceName, newName string) (*Agent, error) {
	src := s.manager.GetAgent(sourceName)
	if src == nil {
		return nil, fmt.Errorf("source agent %q: %w", sourceName, ErrNotFound)
	}

	if !IsValidAgentName(newName) {
		return nil, fmt.Errorf("agent name %q is invalid: use letters, numbers, dash, underscore (max %d chars)", newName, MaxAgentNameLength)
	}

	if existing := s.manager.GetAgent(newName); existing != nil {
		return nil, fmt.Errorf("agent %q already exists", newName)
	}

	// Create git worktree for the new agent.
	wtDir, err := s.manager.CreateWorktree(ctx, newName)
	if err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}

	// Copy CLAUDE.md from source worktree to new worktree.
	if src.WorktreeDir != "" {
		srcClaude := filepath.Join(src.WorktreeDir, "CLAUDE.md")
		if data, readErr := os.ReadFile(srcClaude); readErr == nil { //nolint:gosec // trusted path
			dstClaude := filepath.Join(wtDir, "CLAUDE.md")
			if writeErr := os.WriteFile(dstClaude, data, 0600); writeErr != nil {
				return nil, fmt.Errorf("fork: copy CLAUDE.md: %w", writeErr)
			}
		}

		// Copy .mcp.json from source worktree to new worktree.
		srcMCP := filepath.Join(src.WorktreeDir, ".mcp.json")
		if data, readErr := os.ReadFile(srcMCP); readErr == nil { //nolint:gosec // trusted path
			dstMCP := filepath.Join(wtDir, ".mcp.json")
			if writeErr := os.WriteFile(dstMCP, data, 0600); writeErr != nil {
				return nil, fmt.Errorf("fork: copy .mcp.json: %w", writeErr)
			}
		}
	}

	now := time.Now()
	newAgent := &Agent{
		ID:             newName,
		Name:           newName,
		Role:           src.Role,
		State:          StateStopped,
		Workspace:      src.Workspace,
		Session:        newName,
		Tool:           src.Tool,
		RuntimeBackend: src.RuntimeBackend,
		WorktreeDir:    wtDir,
		ParentID:       "", // forked agents are independent — no parent
		Children:       []string{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.manager.RegisterStopped(newAgent); err != nil {
		return nil, fmt.Errorf("register forked agent: %w", err)
	}

	s.publishEvent("agent.forked", map[string]any{
		"source": sourceName,
		"name":   newName,
	})

	return newAgent, nil
}
