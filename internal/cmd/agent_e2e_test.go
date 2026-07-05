package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/agent"
)

// resetAgentFlags resets agent command flags between tests
func resetAgentFlags() {
	agentCreateTool = ""
	agentCreateRole = "worker"
	agentCreateParent = ""
	agentCreateTeam = ""
	agentListRole = ""
	agentListJSON = false
	agentPeekLines = 50
	agentStopForce = false
}

// --- Agent Lifecycle E2E Tests ---

func TestAgentLifecycle_ListEmpty(t *testing.T) {
	setupTestWorkspace(t)
	resetAgentFlags()
	defer resetAgentFlags()

	// Agent list uses fmt.Printf, so output not captured by cobra buffer
	// Just verify the command succeeds
	_, err := executeCmd("agent", "list")
	if err != nil {
		t.Fatalf("agent list error: %v", err)
	}
}

func TestAgentLifecycle_ListWithRoleFilter(t *testing.T) {
	setupTestWorkspace(t)
	resetAgentFlags()
	defer resetAgentFlags()

	// Filter by role should work even with no agents
	// Agent list uses fmt.Printf, so output not captured
	_, err := executeCmd("agent", "list", "--role", "engineer")
	if err != nil {
		t.Fatalf("agent list --role error: %v", err)
	}
}

func TestAgentLifecycle_ListInvalidRole(t *testing.T) {
	setupTestWorkspace(t)
	resetAgentFlags()
	defer resetAgentFlags()

	// Use a truly invalid role name (contains @) to trigger validation error
	_, err := executeCmd("agent", "list", "--role", "invalid@role")
	if err == nil {
		t.Error("expected error for invalid role filter")
	}
}

func TestAgentLifecycle_ListPositionalArg(t *testing.T) {
	setupTestWorkspace(t)
	resetAgentFlags()
	defer resetAgentFlags()

	// Positional args should error with helpful message
	_, err := executeCmd("agent", "list", "engineer")
	if err == nil {
		t.Error("expected error for positional argument")
	}
	if !strings.Contains(err.Error(), "unexpected argument") {
		t.Errorf("expected 'unexpected argument' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--role") {
		t.Errorf("error should suggest --role flag, got: %v", err)
	}
}

func TestAgentLifecycle_CreateNoWorkspace(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	// Clear MYCEL_WORKSPACE env var so workspace detection falls back to directory walking
	origBCWorkspace := os.Getenv("MYCEL_WORKSPACE")
	_ = os.Unsetenv("MYCEL_WORKSPACE")
	defer func() {
		if origBCWorkspace != "" {
			_ = os.Setenv("MYCEL_WORKSPACE", origBCWorkspace)
		}
	}()

	resetAgentFlags()
	defer resetAgentFlags()

	// Include --role flag so validation passes and we reach workspace check
	_, err := executeCmd("agent", "create", "test-agent", "--role", "engineer")
	if err == nil {
		t.Fatal("expected error for missing workspace")
	}
	if !strings.Contains(err.Error(), "not in a mycel-adopted repo") {
		t.Errorf("expected workspace error, got: %v", err)
	}
}

func TestAgentLifecycle_StopNotFound(t *testing.T) {
	setupTestWorkspace(t)
	resetAgentFlags()
	defer resetAgentFlags()

	_, err := executeCmd("agent", "stop", "nonexistent-agent")
	if err == nil {
		t.Error("expected error for non-existent agent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestAgentLifecycle_PeekNotFound(t *testing.T) {
	setupTestWorkspace(t)
	resetAgentFlags()
	defer resetAgentFlags()

	_, err := executeCmd("agent", "peek", "nonexistent-agent")
	if err == nil {
		t.Error("expected error for non-existent agent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestAgentLifecycle_SendNotFound(t *testing.T) {
	setupTestWorkspace(t)
	resetAgentFlags()
	defer resetAgentFlags()

	_, err := executeCmd("agent", "send", "nonexistent-agent", "hello")
	if err == nil {
		t.Error("expected error for non-existent agent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestAgentLifecycle_AttachNotRunning(t *testing.T) {
	setupTestWorkspace(t)
	resetAgentFlags()
	defer resetAgentFlags()

	_, err := executeCmd("agent", "attach", "nonexistent-agent")
	if err == nil {
		t.Error("expected error for non-existent agent")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("expected 'not running' error, got: %v", err)
	}
}

// --- Agent State Workflow Tests ---

func TestAgentStateWorkflow_ManagerOperations(t *testing.T) {
	wsDir := setupTestWorkspace(t)
	resetAgentFlags()
	defer resetAgentFlags()

	// Create workspace manager
	mgr := agent.NewWorkspaceManager(filepath.Join(wsDir, ".bc", "agents"), wsDir)
	if err := mgr.LoadState(); err != nil {
		t.Logf("initial load (expected to be empty): %v", err)
	}

	// List agents - should be empty
	agents := mgr.ListAgents()
	if len(agents) != 0 {
		t.Errorf("expected no agents initially, got %d", len(agents))
	}

	// Running count should be 0
	if count := mgr.RunningCount(); count != 0 {
		t.Errorf("expected running count 0, got %d", count)
	}

	// Agent count should be 0
	if count := mgr.AgentCount(); count != 0 {
		t.Errorf("expected agent count 0, got %d", count)
	}
}

func TestAgentStateWorkflow_GetNonExistentAgent(t *testing.T) {
	wsDir := setupTestWorkspace(t)

	mgr := agent.NewWorkspaceManager(filepath.Join(wsDir, ".bc", "agents"), wsDir)
	_ = mgr.LoadState()

	// GetAgent should return nil for non-existent
	a := mgr.GetAgent("does-not-exist")
	if a != nil {
		t.Error("expected nil for non-existent agent")
	}
}

// --- Channel Communication Workflow Tests ---
// Note: Channel commands use fmt.Printf which doesn't go through cobra's test buffer
// So we test that commands succeed/fail appropriately, not output content

func TestChannelWorkflow_CreateAndList(t *testing.T) {
	setupTestWorkspace(t)

	ch := uniqueChannelName(t, "")
	// Create a channel (should succeed without error)
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("channel create error: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	// List channels (should succeed)
	_, err = executeCmd("channel", "list")
	if err != nil {
		t.Fatalf("channel list error: %v", err)
	}
}

func TestChannelWorkflow_AddRemoveMembers(t *testing.T) {
	setupTestWorkspace(t)

	ch := uniqueChannelName(t, "")
	// Create channel via command
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	// Add member (should succeed)
	_, err = executeCmd("channel", "add", ch, "agent-01")
	if err != nil {
		t.Fatalf("channel add error: %v", err)
	}

	// Remove member (should succeed)
	_, err = executeCmd("channel", "remove", ch, "agent-01")
	if err != nil {
		t.Fatalf("channel remove error: %v", err)
	}
}

func TestChannelWorkflow_JoinWithoutAgentID(t *testing.T) {
	setupTestWorkspace(t)

	ch := uniqueChannelName(t, "join")
	// Create channel via command
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	// Use t.Setenv with empty string to unset (t.Setenv handles cleanup)
	t.Setenv("MYCEL_AGENT_ID", "")

	// Try to join - should fail without MYCEL_AGENT_ID (now has user-friendly error message)
	_, err = executeCmd("channel", "join", ch)
	if err == nil {
		t.Error("expected error for join without agent context")
	}
	// Check for user-friendly error message that explains the issue and how to fix it
	if err != nil && !strings.Contains(err.Error(), "can only be run by agents") && !strings.Contains(err.Error(), "MYCEL_AGENT_ID") {
		t.Errorf("expected agent-only error message, got: %v", err)
	}
}

func TestChannelWorkflow_JoinWithAgentID(t *testing.T) {
	setupTestWorkspace(t)

	ch := uniqueChannelName(t, "join")
	// Create channel via command
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	// Set MYCEL_AGENT_ID using t.Setenv (handles cleanup automatically)
	t.Setenv("MYCEL_AGENT_ID", "test-agent-01")

	// Join channel (should succeed)
	_, err = executeCmd("channel", "join", ch)
	if err != nil {
		t.Fatalf("channel join error: %v", err)
	}
}

func TestChannelWorkflow_LeaveChannel(t *testing.T) {
	setupTestWorkspace(t)

	ch := uniqueChannelName(t, "leave")
	// Create channel and add member via commands
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	_, err = executeCmd("channel", "add", ch, "leaving-agent")
	if err != nil {
		t.Fatalf("failed to add member: %v", err)
	}

	// Set MYCEL_AGENT_ID using t.Setenv (handles cleanup automatically)
	t.Setenv("MYCEL_AGENT_ID", "leaving-agent")

	// Leave channel (should succeed)
	_, err = executeCmd("channel", "leave", ch)
	if err != nil {
		t.Fatalf("channel leave error: %v", err)
	}
}

func TestChannelWorkflow_SendToEmptyChannel(t *testing.T) {
	setupTestWorkspace(t)

	ch := uniqueChannelName(t, "empty")
	// Create empty channel via command
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	// Send to empty channel (should succeed but do nothing)
	_, err = executeCmd("channel", "send", ch, "hello world")
	if err != nil {
		t.Fatalf("channel send error: %v", err)
	}
}

func TestChannelWorkflow_DeleteChannel(t *testing.T) {
	setupTestWorkspace(t)

	ch := uniqueChannelName(t, "del")
	// Create channel via command
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	// Delete channel (should succeed)
	_, err = executeCmd("channel", "delete", ch)
	if err != nil {
		t.Fatalf("channel delete error: %v", err)
	}
}

func TestChannelWorkflow_DeleteNonExistent(t *testing.T) {
	setupTestWorkspace(t)

	_, err := executeCmd("channel", "delete", "nonexistent-e2e-wf-xyz")
	if err == nil {
		t.Error("expected error deleting non-existent channel")
	}
}

func TestChannelWorkflow_HistoryEmpty(t *testing.T) {
	setupTestWorkspace(t)

	ch := uniqueChannelName(t, "hist")
	// Create channel via command
	_, err := executeCmd("channel", "create", ch)
	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = executeCmd("channel", "delete", ch)
	})

	// Check empty history (should succeed)
	_, err = executeCmd("channel", "history", ch)
	if err != nil {
		t.Fatalf("channel history error: %v", err)
	}
}

func TestChannelWorkflow_HistoryNonExistent(t *testing.T) {
	setupTestWorkspace(t)

	_, err := executeCmd("channel", "history", "nonexistent-e2e-wf-xyz")
	if err == nil {
		t.Error("expected error for non-existent channel history")
	}
}

// --- Agent Team Workflow Tests ---

func TestAgentTeamWorkflow_InvalidTeamName(t *testing.T) {
	invalidNames := []string{
		"team with spaces",
		"team@special",
		"team.dot",
		"",
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			if isValidTeamName(name) {
				t.Errorf("expected %q to be invalid team name", name)
			}
		})
	}
}

func TestAgentTeamWorkflow_ValidTeamName(t *testing.T) {
	validNames := []string{
		"engineering",
		"qa-team",
		"team_01",
		"Platform-2024",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			if !isValidTeamName(name) {
				t.Errorf("expected %q to be valid team name", name)
			}
		})
	}
}

// --- Role Hierarchy Workflow Tests ---

func TestRoleHierarchy_CanCreate(t *testing.T) {
	// Test role hierarchy based on actual RoleHierarchy map:
	// Manager: TechLead, Engineer, QA
	// TechLead: Engineer only
	// Engineer, QA, Worker: cannot create children
	tests := []struct {
		name      string
		parent    agent.Role
		child     agent.Role
		canCreate bool
	}{
		{"manager creates engineer", agent.Role("manager"), agent.Role("engineer"), true},
		{"manager creates qa", agent.Role("manager"), agent.Role("qa"), true},
		{"manager creates tech-lead", agent.Role("manager"), agent.Role("tech-lead"), true},
		{"manager cannot create worker", agent.Role("manager"), agent.Role("worker"), false},
		{"tech-lead creates engineer", agent.Role("tech-lead"), agent.Role("engineer"), true},
		{"tech-lead cannot create qa", agent.Role("tech-lead"), agent.Role("qa"), false},
		{"engineer cannot create engineer", agent.Role("engineer"), agent.Role("engineer"), false},
		{"worker cannot create anything", agent.Role("worker"), agent.Role("worker"), false},
		{"qa cannot create engineer", agent.Role("qa"), agent.Role("engineer"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.CanCreateRole(tt.parent, tt.child)
			if result != tt.canCreate {
				t.Errorf("CanCreateRole(%v, %v) = %v, want %v",
					tt.parent, tt.child, result, tt.canCreate)
			}
		})
	}
}

// --- Command Argument Validation Tests ---

func TestAgentCmdArgs_CreateAcceptsOptionalName(t *testing.T) {
	// create accepts 0 or 1 args
	err := agentCreateCmd.Args(agentCreateCmd, []string{})
	if err != nil {
		t.Errorf("expected create to accept 0 args, got: %v", err)
	}

	err = agentCreateCmd.Args(agentCreateCmd, []string{"my-agent"})
	if err != nil {
		t.Errorf("expected create to accept 1 arg, got: %v", err)
	}

	err = agentCreateCmd.Args(agentCreateCmd, []string{"a", "b"})
	if err == nil {
		t.Error("expected create to reject 2 args")
	}
}

func TestAgentCmdArgs_StopRequiresName(t *testing.T) {
	err := agentStopCmd.Args(agentStopCmd, []string{})
	if err == nil {
		t.Error("expected stop to require agent name")
	}

	err = agentStopCmd.Args(agentStopCmd, []string{"my-agent"})
	if err != nil {
		t.Errorf("expected stop to accept 1 arg, got: %v", err)
	}
}

func TestAgentCmdArgs_SendRequiresNameAndMessage(t *testing.T) {
	err := agentSendCmd.Args(agentSendCmd, []string{})
	if err == nil {
		t.Error("expected send to require args")
	}

	err = agentSendCmd.Args(agentSendCmd, []string{"agent"})
	if err == nil {
		t.Error("expected send to require message")
	}

	err = agentSendCmd.Args(agentSendCmd, []string{"agent", "message"})
	if err != nil {
		t.Errorf("expected send to accept name and message, got: %v", err)
	}

	// Multiple words in message should work
	err = agentSendCmd.Args(agentSendCmd, []string{"agent", "hello", "world"})
	if err != nil {
		t.Errorf("expected send to accept multi-word message, got: %v", err)
	}
}

// --- No Workspace Error Tests ---

func TestNoWorkspace_AgentList(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	// Clear MYCEL_WORKSPACE env var to test directory-based workspace detection
	restoreEnv := clearWorkspaceEnv(t)
	defer restoreEnv()

	_, err := executeCmd("agent", "list")
	if err == nil {
		t.Fatal("expected error for missing workspace")
	}
	if !strings.Contains(err.Error(), "not in a mycel-adopted repo") {
		t.Errorf("expected workspace error, got: %v", err)
	}
}

func TestNoWorkspace_AgentStop(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	restoreEnv := clearWorkspaceEnv(t)
	defer restoreEnv()

	_, err := executeCmd("agent", "stop", "any-agent")
	if err == nil {
		t.Fatal("expected error for missing workspace")
	}
	if !strings.Contains(err.Error(), "not in a mycel-adopted repo") {
		t.Errorf("expected workspace error, got: %v", err)
	}
}

func TestNoWorkspace_AgentPeek(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	restoreEnv := clearWorkspaceEnv(t)
	defer restoreEnv()

	_, err := executeCmd("agent", "peek", "any-agent")
	if err == nil {
		t.Fatal("expected error for missing workspace")
	}
	if !strings.Contains(err.Error(), "not in a mycel-adopted repo") {
		t.Errorf("expected workspace error, got: %v", err)
	}
}

func TestNoWorkspace_AgentSend(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	restoreEnv := clearWorkspaceEnv(t)
	defer restoreEnv()

	_, err := executeCmd("agent", "send", "any-agent", "message")
	if err == nil {
		t.Fatal("expected error for missing workspace")
	}
	if !strings.Contains(err.Error(), "not in a mycel-adopted repo") {
		t.Errorf("expected workspace error, got: %v", err)
	}
}

func TestNoWorkspace_ChannelCreate(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	restoreEnv := clearWorkspaceEnv(t)
	defer restoreEnv()

	_, err := executeCmd("channel", "create", "test")
	if err == nil {
		t.Fatal("expected error for missing workspace")
	}
	if !strings.Contains(err.Error(), "not in a mycel-adopted repo") {
		t.Errorf("expected workspace error, got: %v", err)
	}
}
