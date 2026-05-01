package agent

import (
	"context"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/provider"
)

func TestToolHealthLoop_ChecksPeriodically(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&stubProvider{name: "test-tool", binary: "test-bin", installed: false})

	m := &Manager{
		agents: map[string]*Agent{
			"a1": {Name: "a1", Tool: "test-tool", State: StateWorking},
			"a2": {Name: "a2", Tool: "test-tool", State: StateIdle},
		},
		providerRegistry: reg,
		defaultTool:      "test-tool",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a very short interval for testing.
	m.StartToolHealthLoop(ctx, 10*time.Millisecond)
	defer m.StopToolHealthLoop()

	// Wait enough time for at least one tick.
	time.Sleep(50 * time.Millisecond)

	// Both active agents should be marked stuck since tool is not installed.
	m.mu.RLock()
	a1State := m.agents["a1"].State
	a2State := m.agents["a2"].State
	m.mu.RUnlock()

	if a1State != StateStuck {
		t.Errorf("expected a1 state stuck, got %s", a1State)
	}
	if a2State != StateStuck {
		t.Errorf("expected a2 state stuck, got %s", a2State)
	}
}

func TestToolHealthLoop_SkipsStoppedAgents(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&stubProvider{name: "test-tool", binary: "missing", installed: false})

	m := &Manager{
		agents: map[string]*Agent{
			"active":  {Name: "active", Tool: "test-tool", State: StateWorking},
			"stopped": {Name: "stopped", Tool: "test-tool", State: StateStopped},
			"errored": {Name: "errored", Tool: "test-tool", State: StateError},
		},
		providerRegistry: reg,
		defaultTool:      "test-tool",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.StartToolHealthLoop(ctx, 10*time.Millisecond)
	defer m.StopToolHealthLoop()

	time.Sleep(50 * time.Millisecond)

	m.mu.RLock()
	activeState := m.agents["active"].State
	stoppedState := m.agents["stopped"].State
	erroredState := m.agents["errored"].State
	m.mu.RUnlock()

	if activeState != StateStuck {
		t.Errorf("expected active agent to be stuck, got %s", activeState)
	}
	if stoppedState != StateStopped {
		t.Errorf("expected stopped agent to remain stopped, got %s", stoppedState)
	}
	if erroredState != StateError {
		t.Errorf("expected errored agent to remain errored, got %s", erroredState)
	}
}

func TestToolHealthLoop_StopsOnContextCancel(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&stubProvider{name: "test-tool", binary: "test-bin", installed: true})

	m := &Manager{
		agents:           map[string]*Agent{"a1": {Name: "a1", Tool: "test-tool", State: StateIdle}},
		providerRegistry: reg,
		defaultTool:      "test-tool",
	}

	ctx, cancel := context.WithCancel(context.Background())

	m.StartToolHealthLoop(ctx, 10*time.Millisecond)

	// Cancel the context — the loop should stop.
	cancel()

	// Give the goroutine time to exit.
	time.Sleep(50 * time.Millisecond)

	// Verify StopToolHealthLoop is safe to call after context cancel.
	m.StopToolHealthLoop()
}

func TestToolHealthLoop_StopMethod(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&stubProvider{name: "test-tool", binary: "test-bin", installed: true})

	m := &Manager{
		agents:           map[string]*Agent{"a1": {Name: "a1", Tool: "test-tool", State: StateIdle}},
		providerRegistry: reg,
		defaultTool:      "test-tool",
	}

	ctx := context.Background()
	m.StartToolHealthLoop(ctx, 10*time.Millisecond)

	// Stop via the explicit method.
	m.StopToolHealthLoop()

	// Give the goroutine time to exit.
	time.Sleep(50 * time.Millisecond)

	// Double-stop should be safe.
	m.StopToolHealthLoop()
}

func TestToolHealthLoop_HandlesErrorsGracefully(t *testing.T) {
	// No provider registry — CheckToolHealth will return an error for each agent.
	m := &Manager{
		agents: map[string]*Agent{
			"a1": {Name: "a1", Tool: "unknown-tool", State: StateWorking},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should not panic even though CheckToolHealth returns errors.
	m.StartToolHealthLoop(ctx, 10*time.Millisecond)
	defer m.StopToolHealthLoop()

	time.Sleep(50 * time.Millisecond)

	// Agent should still be in working state (error was logged, not fatal).
	m.mu.RLock()
	state := m.agents["a1"].State
	m.mu.RUnlock()

	if state != StateWorking {
		t.Errorf("expected agent to remain working after check error, got %s", state)
	}
}
