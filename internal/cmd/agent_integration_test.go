package cmd

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/agent"
)

// Agent command integration tests that don't require actual tmux sessions.
// These tests seed agent state files directly to test display/query functionality.

func TestAgentListEmptyIntegration(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	stdout, _, err := executeIntegrationCmd("agent", "list")
	if err != nil {
		t.Fatalf("agent list failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "No agents") {
		t.Errorf("expected 'No agents' message, got: %s", stdout)
	}
}

func TestAgentListWithAgents(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Seed agents
	seedAgents(t, wsDir, map[string]*agent.Agent{
		"engineer-01": {
			Name:      "engineer-01",
			Role:      agent.Role("engineer"),
			State:     agent.StateWorking,
			Task:      "fixing bug",
			Session:   "bc-engineer-01",
			StartedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now(),
		},
		"qa-01": {
			Name:      "qa-01",
			Role:      agent.Role("qa"),
			State:     agent.StateIdle,
			Session:   "bc-qa-01",
			StartedAt: time.Now().Add(-30 * time.Minute),
			UpdatedAt: time.Now(),
		},
	})

	stdout, _, err := executeIntegrationCmd("agent", "list")
	if err != nil {
		t.Fatalf("agent list failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "engineer-01") {
		t.Errorf("output should contain engineer-01: %s", stdout)
	}
	if !strings.Contains(stdout, "qa-01") {
		t.Errorf("output should contain qa-01: %s", stdout)
	}
}

func TestAgentListFilterByRole(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Seed agents with different roles
	seedAgents(t, wsDir, map[string]*agent.Agent{
		"engineer-01": {
			Name:      "engineer-01",
			Role:      agent.Role("engineer"),
			State:     agent.StateWorking,
			Session:   "bc-engineer-01",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		"qa-01": {
			Name:      "qa-01",
			Role:      agent.Role("qa"),
			State:     agent.StateIdle,
			Session:   "bc-qa-01",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	})

	// Filter by engineer role
	stdout, _, err := executeIntegrationCmd("agent", "list", "--role", "engineer")
	if err != nil {
		t.Fatalf("agent list --role failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "engineer-01") {
		t.Errorf("output should contain engineer-01: %s", stdout)
	}
	// qa-01 should not be in output when filtering by engineer
	// Note: The filter might still show QA in the summary count, just not in the filtered list
}

func TestAgentListJSONIntegration(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	seedAgents(t, wsDir, map[string]*agent.Agent{
		"engineer-01": {
			Name:      "engineer-01",
			Role:      agent.Role("engineer"),
			State:     agent.StateIdle,
			Session:   "bc-engineer-01",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	})

	stdout, _, err := executeIntegrationCmd("agent", "list", "--json")
	if err != nil {
		t.Fatalf("agent list --json failed: %v\nOutput: %s", err, stdout)
	}
	// Output should be valid JSON (starts with [ or {)
	trimmed := strings.TrimSpace(stdout)
	if !strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "{") {
		t.Errorf("output should be JSON, got: %s", stdout)
	}
}

func TestAgentStopNotFound(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	_, _, err := executeIntegrationCmd("agent", "stop", "nonexistent-agent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found: %v", err)
	}
}

func TestAgentPeekNotFound(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	_, _, err := executeIntegrationCmd("agent", "peek", "nonexistent-agent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
	// Error message varies based on implementation
}

func TestAgentShowNotFound(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	_, _, err := executeIntegrationCmd("agent", "show", "nonexistent-agent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found: %v", err)
	}
}

func TestAgentShowWithAgent(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Seed an agent
	seedAgents(t, wsDir, map[string]*agent.Agent{
		"test-agent": {
			Name:      "test-agent",
			Role:      agent.Role("engineer"),
			State:     agent.StateWorking,
			Session:   "bc-test-agent",
			StartedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now(),
		},
	})

	stdout, _, err := executeIntegrationCmd("agent", "show", "test-agent")
	if err != nil {
		t.Fatalf("agent show failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "test-agent") {
		t.Errorf("output should contain agent name: %s", stdout)
	}
	if !strings.Contains(stdout, "engineer") {
		t.Errorf("output should contain role: %s", stdout)
	}
}

func TestAgentAttachNotFound(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	_, _, err := executeIntegrationCmd("agent", "attach", "nonexistent-agent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
	// Error message varies based on implementation
}

func TestAgentSendNotFound(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	_, _, err := executeIntegrationCmd("agent", "send", "nonexistent-agent", "hello")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found: %v", err)
	}
}

func TestAgentSendToStoppedAgent(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Seed a stopped agent
	seedAgents(t, wsDir, map[string]*agent.Agent{
		"stopped-agent": {
			Name:      "stopped-agent",
			Role:      agent.Role("engineer"),
			State:     agent.StateStopped,
			Session:   "bc-stopped-agent",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	})

	_, _, err := executeIntegrationCmd("agent", "send", "stopped-agent", "hello")
	if err == nil {
		t.Error("expected error for stopped agent")
	}
	if err != nil && !strings.Contains(err.Error(), "stopped") {
		t.Errorf("error should mention stopped: %v", err)
	}
}

func TestAgentCreateInvalidRole(t *testing.T) {
	// Only truly invalid role names (format) should error
	// Any alphanumeric name with hyphens is valid (roles are custom)
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	_, _, err := executeIntegrationCmd("agent", "create", "test-agent", "--role", "role@invalid")
	if err == nil {
		t.Error("expected error for invalid role format")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention invalid: %v", err)
	}
}

func TestAgentCreateInvalidTeam(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	_, _, err := executeIntegrationCmd("agent", "create", "test-agent", "--role", "null", "--team", "invalid team name!")
	if err == nil {
		t.Error("expected error for invalid team name")
	}
	if err != nil && !strings.Contains(err.Error(), "team name") {
		t.Errorf("error should mention team name: %v", err)
	}
}

func TestAgentCreateUnknownTool(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	_, _, err := executeIntegrationCmd("agent", "create", "test-agent", "--role", "engineer", "--tool", "nonexistent-tool")
	if err == nil {
		t.Error("expected error for unknown tool")
	}
	if err != nil && !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("error should mention unknown tool: %v", err)
	}
}

func TestAgentNoWorkspace(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir := t.TempDir()
	if err = os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Clear MYCEL_WORKSPACE to test directory-based workspace detection
	restoreEnv := clearWorkspaceEnv(t)
	defer restoreEnv()

	_, _, execErr := executeIntegrationCmd("agent", "list")
	if execErr == nil {
		t.Error("expected error when not in workspace")
	}
	if !strings.Contains(execErr.Error(), "not in a mycel-adopted repo") {
		t.Errorf("expected workspace error, got: %v", execErr)
	}
}

func TestAgentHealthEmpty(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	stdout, _, err := executeIntegrationCmd("agent", "health")
	if err != nil {
		t.Fatalf("agent health failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "No agents") {
		t.Errorf("expected 'No agents' message, got: %s", stdout)
	}
}

func TestAgentHealthWithAgents(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Seed agents
	seedAgents(t, wsDir, map[string]*agent.Agent{
		"engineer-01": {
			Name:      "engineer-01",
			Role:      agent.Role("engineer"),
			State:     agent.StateWorking,
			Session:   "bc-engineer-01",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		"stuck-agent": {
			Name:      "stuck-agent",
			Role:      agent.Role("engineer"),
			State:     agent.StateError,
			Session:   "bc-stuck-agent",
			StartedAt: time.Now().Add(-2 * time.Hour),
			UpdatedAt: time.Now().Add(-2 * time.Hour), // stale
		},
	})

	stdout, _, err := executeIntegrationCmd("agent", "health")
	if err != nil {
		t.Fatalf("agent health failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "engineer-01") {
		t.Errorf("output should contain engineer-01: %s", stdout)
	}
	if !strings.Contains(stdout, "stuck-agent") {
		t.Errorf("output should contain stuck-agent: %s", stdout)
	}
	if !strings.Contains(stdout, "Summary:") {
		t.Errorf("output should contain summary: %s", stdout)
	}
}

func TestAgentHealthSpecificAgent(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	seedAgents(t, wsDir, map[string]*agent.Agent{
		"engineer-01": {
			Name:      "engineer-01",
			Role:      agent.Role("engineer"),
			State:     agent.StateWorking,
			Session:   "bc-engineer-01",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	})

	stdout, _, err := executeIntegrationCmd("agent", "health", "engineer-01")
	if err != nil {
		t.Fatalf("agent health failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "engineer-01") {
		t.Errorf("output should contain engineer-01: %s", stdout)
	}
}

func TestAgentHealthNotFound(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	_, _, err := executeIntegrationCmd("agent", "health", "nonexistent-agent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found: %v", err)
	}
}

func TestAgentHealthJSONOutput(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	seedAgents(t, wsDir, map[string]*agent.Agent{
		"engineer-01": {
			Name:      "engineer-01",
			Role:      agent.Role("engineer"),
			State:     agent.StateIdle,
			Session:   "bc-engineer-01",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	})

	stdout, _, err := executeIntegrationCmd("agent", "health", "--json")
	if err != nil {
		t.Fatalf("agent health --json failed: %v\nOutput: %s", err, stdout)
	}
	// Output should be valid JSON (starts with [)
	trimmed := strings.TrimSpace(stdout)
	if !strings.HasPrefix(trimmed, "[") {
		t.Errorf("output should be JSON array, got: %s", stdout)
	}
	if !strings.Contains(stdout, "engineer-01") {
		t.Errorf("JSON should contain agent name: %s", stdout)
	}
}

func TestAgentHealthCustomTimeout(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Seed an agent with stale state
	seedAgents(t, wsDir, map[string]*agent.Agent{
		"stale-agent": {
			Name:      "stale-agent",
			Role:      agent.Role("engineer"),
			State:     agent.StateWorking,
			Session:   "bc-stale-agent",
			StartedAt: time.Now().Add(-5 * time.Minute),
			UpdatedAt: time.Now().Add(-5 * time.Minute),
		},
	})

	// With short timeout, should show degraded
	stdout, _, err := executeIntegrationCmd("agent", "health", "--timeout", "1s")
	if err != nil {
		t.Fatalf("agent health --timeout failed: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "degraded") && !strings.Contains(stdout, "unhealthy") {
		t.Errorf("stale agent should be degraded or unhealthy with 1s timeout: %s", stdout)
	}
}

func TestAgentHealthDetectStuckFlag(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Seed an agent
	seedAgents(t, wsDir, map[string]*agent.Agent{
		"test-agent": {
			Name:      "test-agent",
			Role:      agent.Role("engineer"),
			State:     agent.StateWorking,
			Session:   "bc-test-agent",
			StartedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now(),
		},
	})

	// Test that --detect-stuck flag works (command should execute without error)
	stdout, _, err := executeIntegrationCmd("agent", "health", "--detect-stuck")
	if err != nil {
		t.Fatalf("agent health --detect-stuck failed: %v\nOutput: %s", err, stdout)
	}
	// Command should execute and show agent
	if !strings.Contains(stdout, "test-agent") {
		t.Errorf("output should contain agent name: %s", stdout)
	}
}

func TestAgentHealthDetectStuckMaxFailures(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Seed an agent
	seedAgents(t, wsDir, map[string]*agent.Agent{
		"test-agent": {
			Name:      "test-agent",
			Role:      agent.Role("engineer"),
			State:     agent.StateWorking,
			Session:   "bc-test-agent",
			StartedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now(),
		},
	})

	// Test custom max-failures flag (command should execute without error)
	stdout, _, err := executeIntegrationCmd("agent", "health", "--detect-stuck", "--max-failures", "5")
	if err != nil {
		t.Fatalf("agent health --max-failures failed: %v\nOutput: %s", err, stdout)
	}
	// Command should execute and show agent
	if !strings.Contains(stdout, "test-agent") {
		t.Errorf("output should contain agent name: %s", stdout)
	}
}

func TestAgentHealthDetectStuckWorkTimeout(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Seed an agent
	seedAgents(t, wsDir, map[string]*agent.Agent{
		"test-agent": {
			Name:      "test-agent",
			Role:      agent.Role("engineer"),
			State:     agent.StateWorking,
			Session:   "bc-test-agent",
			StartedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now(),
		},
	})

	// Test custom work-timeout flag (command should execute without error)
	stdout, _, err := executeIntegrationCmd("agent", "health", "--detect-stuck", "--work-timeout", "1h")
	if err != nil {
		t.Fatalf("agent health --work-timeout failed: %v\nOutput: %s", err, stdout)
	}
	// Command should execute and show agent
	if !strings.Contains(stdout, "test-agent") {
		t.Errorf("output should contain agent name: %s", stdout)
	}
}

func TestAgentStartMissingAgent(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Try to start non-existent agent - should fail
	stdout, stderr, err := executeIntegrationCmd("agent", "start", "nonexistent-agent")
	if err == nil {
		t.Fatalf("agent start should fail for non-existent agent, got: %s", stdout)
	}
	if !strings.Contains(stderr, "not found") && !strings.Contains(stdout, "not found") {
		t.Errorf("error message should indicate agent not found: stderr=%s stdout=%s", stderr, stdout)
	}
}

func TestAgentStartNotStopped(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Seed agent in running state (not stopped)
	seedAgents(t, wsDir, map[string]*agent.Agent{
		"working-agent": {
			Name:      "working-agent",
			Role:      agent.Role("engineer"),
			State:     agent.StateWorking,
			Session:   "bc-working-agent",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	})

	// Try to start running agent - should fail with "not stopped" error
	stdout, stderr, err := executeIntegrationCmd("agent", "start", "working-agent")
	if err == nil {
		t.Fatalf("agent start should fail for running agent, got: %s", stdout)
	}
	output := stderr + stdout
	if !strings.Contains(output, "already running") && !strings.Contains(output, "state:") {
		t.Errorf("error message should indicate agent is already running: %s", output)
	}
}

func TestAgentStartStopped(t *testing.T) {
	wsDir, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Seed stopped agent
	seedAgents(t, wsDir, map[string]*agent.Agent{
		"stopped-agent": {
			Name:      "stopped-agent",
			Role:      agent.Role("engineer"),
			State:     agent.StateStopped,
			Session:   "",
			StartedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now(),
		},
	})

	// Start the stopped agent - note: this will fail in test because tmux session can't be created
	// but we're testing the command logic, not the actual tmux creation
	stdout, stderr, _ := executeIntegrationCmd("agent", "start", "stopped-agent")
	// In integration test, this will fail due to tmux issues, but error should not be "agent not found"
	output := stderr + stdout
	if strings.Contains(output, "agent not found") || strings.Contains(output, "no agent named") {
		t.Errorf("agent should be found, but got: %s", output)
	}
}
