package agent

import (
	"context"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
)

// DefaultToolHealthInterval is the default interval between tool health checks.
const DefaultToolHealthInterval = 30 * time.Second

// StartToolHealthLoop launches a background goroutine that periodically checks
// tool availability for all running agents. It calls CheckToolHealth for each
// agent on every tick, logging state transitions (tool became unavailable or
// recovered). The loop respects context cancellation for clean shutdown.
func (m *Manager) StartToolHealthLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultToolHealthInterval
	}

	m.toolHealthMu.Lock()
	// If a loop is already running, stop it first.
	if m.toolHealthCancel != nil {
		m.toolHealthCancel()
	}
	loopCtx, cancel := context.WithCancel(ctx)
	m.toolHealthCancel = cancel
	m.toolHealthMu.Unlock()

	go m.runToolHealthLoop(loopCtx, interval)
}

// StopToolHealthLoop cancels the background tool health check goroutine.
func (m *Manager) StopToolHealthLoop() {
	m.toolHealthMu.Lock()
	defer m.toolHealthMu.Unlock()

	if m.toolHealthCancel != nil {
		m.toolHealthCancel()
		m.toolHealthCancel = nil
	}
}

// runToolHealthLoop is the internal loop that ticks at the given interval and
// checks each running agent's tool availability.
func (m *Manager) runToolHealthLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Info("tool health loop started", "interval", interval)

	for {
		select {
		case <-ctx.Done():
			log.Info("tool health loop stopped")
			return
		case <-ticker.C:
			m.checkAllAgentTools(ctx)
		}
	}
}

// checkAllAgentTools iterates over all agents and checks tool health for those
// in an active state (not stopped, not errored).
func (m *Manager) checkAllAgentTools(ctx context.Context) {
	agents := m.ListAgents()

	for _, a := range agents {
		// Skip agents that are not in an active state.
		if a.State == StateStopped || a.State == StateError {
			continue
		}

		if err := m.CheckToolHealth(ctx, a.Name); err != nil {
			log.Debug("tool health check failed",
				"agent", a.Name,
				"error", err,
			)
		}
	}
}
