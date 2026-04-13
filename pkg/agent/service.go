package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rpuneet/bc/pkg/log"
	"github.com/rpuneet/bc/pkg/names"
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
	Role   string // Filter by role (empty = all)
	Status string // Filter by status/state (empty = all)
}

// CreateOptions holds parameters for creating an agent via the service.
type CreateOptions struct {
	Name    string
	Role    Role
	Tool    string
	EnvFile string
	Runtime string
	Parent  string
	Team    string
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

	// Apply filters
	if opts.Role == "" && opts.Status == "" {
		return agents, nil
	}

	filtered := make([]*Agent, 0, len(agents))
	for _, a := range agents {
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
	a, err := s.manager.SpawnAgentWithOptions(ctx, SpawnOptions{
		Name:      opts.Name,
		Role:      opts.Role,
		Workspace: s.manager.workspacePath,
		ParentID:  opts.Parent,
		Tool:      opts.Tool,
		EnvFile:   opts.EnvFile,
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
		return nil, fmt.Errorf("agent %q not found", name)
	}

	if existing.State != StateStopped && existing.State != StateError {
		// Reconcile: container may have died without bcd noticing
		if !s.manager.runtimeForAgent(name).HasSession(ctx, name) {
			log.Info("reconciling dead agent for restart", "agent", name, "state", existing.State)
			existing.State = StateStopped
			existing.UpdatedAt = time.Now()
			_ = s.manager.saveState()
		} else {
			return nil, fmt.Errorf("agent %q is already running (state: %s)", name, existing.State)
		}
	}

	a, err := s.manager.SpawnAgentWithOptions(ctx, SpawnOptions{
		Name:      name,
		Role:      existing.Role,
		Workspace: s.manager.workspacePath,
		ParentID:  existing.ParentID,
		Tool:      existing.Tool,
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
		return fmt.Errorf("agent %q not found", name)
	}
	if !force && a.State != StateStopped {
		// Reconcile: container may have died without bcd noticing
		if !s.manager.runtimeForAgent(name).HasSession(ctx, name) {
			a.State = StateStopped
			a.UpdatedAt = time.Now()
			_ = s.manager.saveState()
		} else {
			return fmt.Errorf("agent %q must be stopped before deletion (state: %s). Use ?force=true to delete anyway", name, a.State)
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
		return fmt.Errorf("agent %q not found", name)
	}
	// Reconcile stale state: if marked stopped but session is alive, correct it
	if a.State == StateStopped {
		if s.manager.RuntimeForAgent(name).HasSession(ctx, name) {
			a.State = StateIdle
		} else {
			return fmt.Errorf("agent %q is stopped", name)
		}
	}
	return s.manager.SendToAgent(ctx, name, message)
}

// Peek returns recent output from an agent.
func (s *AgentService) Peek(ctx context.Context, name string, lines int) (string, error) {
	a := s.manager.GetAgent(name)
	if a == nil {
		return "", fmt.Errorf("agent %q not found", name)
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

// Get returns a single agent by name.
func (s *AgentService) Get(ctx context.Context, name string) (*Agent, error) {
	a := s.manager.GetAgent(name)
	if a == nil {
		return nil, fmt.Errorf("agent %q not found", name)
	}
	return a, nil
}

func (s *AgentService) publishEvent(eventType string, data map[string]any) {
	if s.events != nil {
		s.events.Publish(eventType, data)
	}
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

// Sessions returns session history for an agent.
func (s *AgentService) Sessions(ctx context.Context, name string) ([]SessionEntry, error) {
	a := s.manager.GetAgent(name)
	if a == nil {
		return nil, fmt.Errorf("agent %q not found", name)
	}

	var entries []SessionEntry

	if a.SessionID != "" {
		entries = append(entries, SessionEntry{ID: a.SessionID, Current: true})
	}

	histDir := filepath.Join(s.manager.stateDir, "agents", name, "session_history")
	files, err := os.ReadDir(histDir)
	if err == nil {
		sort.Slice(files, func(i, j int) bool {
			return files[i].Name() > files[j].Name()
		})
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			data, readErr := os.ReadFile(filepath.Join(histDir, f.Name())) //nolint:gosec // trusted path
			if readErr != nil {
				continue
			}
			id := strings.TrimSpace(string(data))
			if id == "" || id == a.SessionID {
				continue
			}
			fname := strings.TrimSuffix(f.Name(), ".txt")
			ts, parseErr := time.Parse("2006-01-02T15:04:05", fname)
			entry := SessionEntry{ID: id}
			if parseErr == nil {
				entry.Timestamp = ts
			}
			entries = append(entries, entry)
		}
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
