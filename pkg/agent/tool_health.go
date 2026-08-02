package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
)

// CheckToolHealth verifies that the tool binary for the named agent is still
// installed. If the binary is missing, the agent is marked as StateStuck and
// diagnostic information is logged. This is intended to be called from a
// health-check loop or on-demand.
func (m *Manager) CheckToolHealth(ctx context.Context, agentName string) error {
	m.mu.RLock()
	ag, exists := m.agents[agentName]
	if !exists {
		m.mu.RUnlock()
		return fmt.Errorf("agent %q not found", agentName)
	}

	// Only check agents that are in an active state.
	if ag.State == StateStopped || ag.State == StateError {
		m.mu.RUnlock()
		return nil
	}

	toolName := ag.Tool
	m.mu.RUnlock()

	// Resolve the provider for this agent's tool.
	if toolName == "" {
		toolName = m.defaultTool
	}
	if toolName == "" {
		toolName = DefaultProvider
	}

	if m.providerRegistry == nil {
		return fmt.Errorf("no provider registry configured")
	}

	prov, ok := m.providerRegistry.Get(toolName)
	if !ok {
		return fmt.Errorf("unknown provider %q for agent %q", toolName, agentName)
	}

	if prov.IsInstalled(ctx) {
		return nil
	}

	// Tool binary is unavailable — mark agent as stuck.
	binaryName := prov.Binary()
	log.Warn("tool binary unavailable, marking agent stuck",
		"agent", agentName,
		"tool", toolName,
		"binary", binaryName,
	)

	task := fmt.Sprintf("tool unavailable: %s binary not found", toolName)
	if err := m.MarkStuck(ctx, agentName, task); err != nil {
		return fmt.Errorf("failed to mark agent %q as stuck: %w", agentName, err)
	}

	return nil
}

// MarkStuck sets an agent to StateStuck with the given reason recorded as
// its Task, so the UI shows WHY the agent stopped making progress. It
// validates the transition (a no-op if the agent is already stopped/errored)
// and notifies state-change listeners exactly once, on the transition into
// StateStuck. Used by the tool health loop (binary went missing) and the
// cost/stuck guardrail loop (no activity within the template's
// StuckTimeoutMin).
func (m *Manager) MarkStuck(ctx context.Context, name, task string) error {
	var changed bool

	m.mu.Lock()
	ag, exists := m.agents[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("agent %q not found", name)
	}

	if err := ValidateTransition(ag.State, StateStuck); err != nil {
		m.mu.Unlock()
		return err
	}

	prevState := ag.State
	ag.State = StateStuck
	ag.Task = task
	ag.UpdatedAt = time.Now()
	changed = prevState != StateStuck

	if err := m.saveState(ctx); err != nil {
		log.Warn("failed to save agent state after marking stuck", "error", err)
	}
	m.mu.Unlock()

	if changed {
		m.notifyStateChange(name, StateStuck, task)
	}

	return nil
}
