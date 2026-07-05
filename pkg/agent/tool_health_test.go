package agent

import (
	"context"
	"testing"

	"github.com/rpuneet/mycel/pkg/provider"
)

// stubProvider is a minimal provider for testing tool health checks.
type stubProvider struct {
	name      string
	binary    string
	installed bool
}

func (s *stubProvider) Name() string                               { return s.name }
func (s *stubProvider) Description() string                        { return "stub" }
func (s *stubProvider) Command() string                            { return s.binary }
func (s *stubProvider) Binary() string                             { return s.binary }
func (s *stubProvider) InstallHint() string                        { return "install " + s.name }
func (s *stubProvider) BuildCommand(_ provider.CommandOpts) string { return s.binary }
func (s *stubProvider) IsInstalled(_ context.Context) bool         { return s.installed }
func (s *stubProvider) Version(_ context.Context) string           { return "1.0" }

func TestCheckToolHealth_Installed(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&stubProvider{name: "test-tool", binary: "test-bin", installed: true})

	m := &Manager{
		agents:           map[string]*Agent{"a1": {Name: "a1", Tool: "test-tool", State: StateWorking}},
		providerRegistry: reg,
		defaultTool:      "test-tool",
	}

	if err := m.CheckToolHealth(context.Background(), "a1"); err != nil {
		t.Fatalf("expected no error for installed tool, got: %v", err)
	}

	if m.agents["a1"].State != StateWorking {
		t.Fatalf("expected state to remain working, got: %s", m.agents["a1"].State)
	}
}

func TestCheckToolHealth_Unavailable(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&stubProvider{name: "test-tool", binary: "missing-bin", installed: false})

	m := &Manager{
		agents:           map[string]*Agent{"a1": {Name: "a1", Tool: "test-tool", State: StateWorking}},
		providerRegistry: reg,
		defaultTool:      "test-tool",
	}

	if err := m.CheckToolHealth(context.Background(), "a1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.agents["a1"].State != StateStuck {
		t.Fatalf("expected state stuck, got: %s", m.agents["a1"].State)
	}

	if m.agents["a1"].Task == "" {
		t.Fatal("expected task to be set with diagnostic info")
	}
}

func TestCheckToolHealth_StoppedAgent(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&stubProvider{name: "test-tool", binary: "missing-bin", installed: false})

	m := &Manager{
		agents:           map[string]*Agent{"a1": {Name: "a1", Tool: "test-tool", State: StateStopped}},
		providerRegistry: reg,
	}

	if err := m.CheckToolHealth(context.Background(), "a1"); err != nil {
		t.Fatalf("expected no error for stopped agent, got: %v", err)
	}

	if m.agents["a1"].State != StateStopped {
		t.Fatal("stopped agent state should not change")
	}
}

func TestCheckToolHealth_NotFound(t *testing.T) {
	m := &Manager{
		agents: make(map[string]*Agent),
	}

	err := m.CheckToolHealth(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestCheckToolHealth_DefaultTool(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&stubProvider{name: "claude", binary: "claude", installed: true})

	m := &Manager{
		agents:           map[string]*Agent{"a1": {Name: "a1", Tool: "", State: StateIdle}},
		providerRegistry: reg,
		defaultTool:      "claude",
	}

	if err := m.CheckToolHealth(context.Background(), "a1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.agents["a1"].State != StateIdle {
		t.Fatalf("expected state to remain idle, got: %s", m.agents["a1"].State)
	}
}

func TestCheckToolHealth_NotifiesStateChange(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&stubProvider{name: "test-tool", binary: "missing-bin", installed: false})

	var notified bool
	m := &Manager{
		agents:           map[string]*Agent{"a1": {Name: "a1", Tool: "test-tool", State: StateWorking}},
		providerRegistry: reg,
		onStateChange: func(name string, state State, task string) {
			notified = true
			if name != "a1" {
				t.Errorf("expected agent name a1, got %s", name)
			}
			if state != StateStuck {
				t.Errorf("expected state stuck, got %s", state)
			}
		},
	}

	if err := m.CheckToolHealth(context.Background(), "a1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !notified {
		t.Fatal("expected state change notification")
	}
}
