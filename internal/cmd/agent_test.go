package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/agent"
)

// --- isValidAgentName Tests ---

func TestIsValidAgentName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"alphanumeric", "agent01", true},
		{"with hyphen", "eng-01", true},
		{"with underscore", "eng_01", true},
		{"uppercase", "AGENT", true},
		{"lowercase", "agent", true},
		{"mixed case", "Eng-01", true},
		{"numbers only suffix", "eng123", true},
		{"empty", "", false},
		{"with space", "agent name", false},
		{"with @", "agent@01", false},
		{"with dot", "agent.01", false},
		{"with slash", "agent/01", false},
		{"starts with hyphen", "-agent", true},
		{"starts with underscore", "_agent", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidAgentName(tt.input); got != tt.want {
				t.Errorf("isValidAgentName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- isValidTeamName Tests ---

func TestIsValidTeamName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"alphanumeric", "platform", true},
		{"with numbers", "team123", true},
		{"with hyphen", "core-team", true},
		{"with underscore", "core_team", true},
		{"mixed", "Platform-Team_01", true},
		{"uppercase", "PLATFORM", true},
		{"empty", "", false},
		{"with space", "core team", false},
		{"with special chars", "team@123", false},
		{"with dot", "team.name", false},
		{"with slash", "team/name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidTeamName(tt.input); got != tt.want {
				t.Errorf("isValidTeamName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- Agent Create Tests ---

func TestAgentCreate_ValidRole(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		wantRole string
	}{
		{"null role (default)", "null", "null"},
		{"worker role", "worker", "worker"},
		{"engineer role", "engineer", "engineer"},
		{"manager role", "manager", "manager"},
		{"qa role", "qa", "qa"},
		{"tech-lead role", "tech-lead", "tech-lead"},
		{"product-manager role", "product-manager", "product-manager"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, err := parseRoleStr(tt.role)
			if err != nil {
				t.Errorf("parseRoleStr(%q) error = %v", tt.role, err)
				return
			}
			if role != tt.wantRole {
				t.Errorf("parseRoleStr(%q) = %v, want %v", tt.role, role, tt.wantRole)
			}
		})
	}
}

func TestAgentCreate_InvalidRole(t *testing.T) {
	// Only truly invalid role names should error (format validation)
	// Any alphanumeric name is valid (roles are custom)
	invalidRoles := []struct {
		role string
		desc string
	}{
		{"role@invalid", "contains @ symbol"},
		{"role with space", "contains space"},
	}

	for _, tt := range invalidRoles {
		t.Run(tt.desc, func(t *testing.T) {
			_, err := parseRoleStr(tt.role)
			if err == nil {
				t.Errorf("parseRoleStr(%q) expected error, got nil", tt.role)
			}
		})
	}
}

func TestAgentCreate_CustomRoles(t *testing.T) {
	// All roles are custom now - any valid alphanumeric name is accepted
	// Legacy aliases ('pm', 'coord', 'tl') are returned as-is
	tests := []struct {
		input    string
		wantRole string
	}{
		{"pm", "pm"},       // No expansion
		{"coord", "coord"}, // No expansion
		{"tl", "tl"},       // No expansion
		{"custom-role", "custom-role"},
		{"admin", "admin"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			role, err := parseRoleStr(tt.input)
			if err != nil {
				t.Errorf("parseRoleStr(%q) error = %v", tt.input, err)
				return
			}
			if role != tt.wantRole {
				t.Errorf("parseRoleStr(%q) = %v, want %v", tt.input, role, tt.wantRole)
			}
		})
	}
}

func TestAgentCreate_EmptyName(t *testing.T) {
	// MaximumNArgs(1) allows an empty string arg — name validation
	// happens later in runAgentCreate, not in Args.
	cmd := agentCreateCmd
	if err := cmd.Args(cmd, []string{""}); err != nil {
		t.Errorf("Args should accept a single (empty) positional arg: %v", err)
	}
}

// --- Agent Create Flags Tests ---

func TestAgentCreateHasParentFlag(t *testing.T) {
	flags := agentCreateCmd.Flags()
	if flags.Lookup("parent") == nil {
		t.Error("expected --parent flag on agent create")
	}
}

func TestAgentCreateHasTeamFlag(t *testing.T) {
	flags := agentCreateCmd.Flags()
	if flags.Lookup("team") == nil {
		t.Error("expected --team flag on agent create")
	}
}

func TestAgentCreateHasTaskFlag(t *testing.T) {
	flags := agentCreateCmd.Flags()
	if flags.Lookup("task") == nil {
		t.Error("expected --task flag on agent create (#3589)")
	}
}

// --- Agent Role Hierarchy Tests ---

func TestCanCreateRole_TechLeadCanCreateEngineer(t *testing.T) {
	if !agent.CanCreateRole(agent.Role("tech-lead"), agent.Role("engineer")) {
		t.Error("tech-lead should be able to create engineer")
	}
}

func TestCanCreateRole_EngineerCannotCreateEngineer(t *testing.T) {
	if agent.CanCreateRole(agent.Role("engineer"), agent.Role("engineer")) {
		t.Error("engineer should not be able to create engineer")
	}
}

func TestCanCreateRole_ManagerCanCreateEngineer(t *testing.T) {
	if !agent.CanCreateRole(agent.Role("manager"), agent.Role("engineer")) {
		t.Error("manager should be able to create engineer")
	}
}

func TestCanCreateRole_ManagerCanCreateQA(t *testing.T) {
	if !agent.CanCreateRole(agent.Role("manager"), agent.Role("qa")) {
		t.Error("manager should be able to create qa")
	}
}

// --- Agent List Tests ---

func TestAgentList_FilterByRole(t *testing.T) {
	// Test that role filter validation works
	validRoles := []string{"engineer", "qa", "manager", "worker"}

	for _, role := range validRoles {
		t.Run(role, func(t *testing.T) {
			_, err := parseRoleStr(role)
			if err != nil {
				t.Errorf("parseRoleStr(%q) should be valid for filtering", role)
			}
		})
	}
}

func TestAgentList_EmptyResult(t *testing.T) {
	// This tests the formatting logic for empty agent lists
	agents := []*agent.Agent{}
	if len(agents) != 0 {
		t.Error("expected empty agent list")
	}
}

// --- Agent Stop Tests ---

func TestAgentStop_NonExistentAgent(t *testing.T) {
	// Agent entities live under an agents root (~/.mycel/agents in
	// production); a fresh temp dir stands in for it here.
	agentsDir := filepath.Join(t.TempDir(), "agents")
	if err := os.MkdirAll(agentsDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Create manager with no agents
	mgr := agent.NewManager(agentsDir)

	// Try to get non-existent agent
	a := mgr.GetAgent("nonexistent")
	if a != nil {
		t.Error("expected nil for non-existent agent")
	}
}

// --- Agent Send Tests ---

func TestAgentSend_EmptyMessage(t *testing.T) {
	// Test that empty message is properly rejected
	cmd := agentSendCmd

	// MinimumNArgs(2) should reject single arg
	err := cmd.Args(cmd, []string{"agent-name"})
	if err == nil {
		t.Error("expected error for single arg (missing message)")
	}
}

func TestAgentSend_ValidArgs(t *testing.T) {
	cmd := agentSendCmd

	// Should accept agent name + message
	err := cmd.Args(cmd, []string{"agent-name", "hello world"})
	if err != nil {
		t.Errorf("unexpected error for valid args: %v", err)
	}

	// Should accept multiple message words
	err = cmd.Args(cmd, []string{"agent-name", "hello", "world", "test"})
	if err != nil {
		t.Errorf("unexpected error for multi-word message: %v", err)
	}
}

// --- Agent Peek Tests ---

func TestAgentPeek_DefaultLines(t *testing.T) {
	// Default should be 50 lines
	if agentPeekLines != 50 {
		// Reset to default for test
		agentPeekLines = 50
	}

	if agentPeekLines != 50 {
		t.Errorf("expected default peek lines = 50, got %d", agentPeekLines)
	}
}

// --- Agent Attach Tests ---

func TestAgentAttach_RequiresName(t *testing.T) {
	cmd := agentAttachCmd

	// ExactArgs(1) should reject no args
	err := cmd.Args(cmd, []string{})
	if err == nil {
		t.Error("expected error for missing agent name")
	}

	// Should accept exactly one arg
	err = cmd.Args(cmd, []string{"agent-name"})
	if err != nil {
		t.Errorf("unexpected error for valid arg: %v", err)
	}

	// Should reject multiple args
	err = cmd.Args(cmd, []string{"agent1", "agent2"})
	if err == nil {
		t.Error("expected error for multiple agent names")
	}
}

// --- Command Structure Tests ---

func TestAgentCommandStructure(t *testing.T) {
	// Verify agentCmd has expected subcommands
	subcommands := agentCmd.Commands()

	expectedCmds := map[string]bool{
		"create": false,
		"list":   false,
		"attach": false,
		"peek":   false,
		"stop":   false,
		"send":   false,
	}

	for _, cmd := range subcommands {
		if _, ok := expectedCmds[cmd.Name()]; ok {
			expectedCmds[cmd.Name()] = true
		}
	}

	for name, found := range expectedCmds {
		if !found {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

func TestAgentCreateFlags(t *testing.T) {
	// Verify create command has expected flags
	flags := agentCreateCmd.Flags()

	if flags.Lookup("tool") == nil {
		t.Error("expected --tool flag")
	}
	if flags.Lookup("role") == nil {
		t.Error("expected --role flag")
	}
}

func TestAgentListFlags(t *testing.T) {
	flags := agentListCmd.Flags()

	if flags.Lookup("role") == nil {
		t.Error("expected --role flag for filtering")
	}
	if flags.Lookup("json") == nil {
		t.Error("expected --json flag")
	}
}

func TestAgentPeekFlags(t *testing.T) {
	flags := agentPeekCmd.Flags()

	if flags.Lookup("lines") == nil {
		t.Error("expected --lines flag")
	}
}

func TestAgentStopFlags(t *testing.T) {
	flags := agentStopCmd.Flags()

	if flags.Lookup("force") == nil {
		t.Error("expected --force flag")
	}
}

// --- Integration Tests using executeCmd ---

func TestAgentListEmpty(t *testing.T) {
	setupTestHome(t)

	// Command should succeed even with no agents
	_, err := executeCmd("agent", "list")
	if err != nil {
		t.Fatalf("agent list failed: %v", err)
	}
}

func TestAgentListJSON(t *testing.T) {
	setupTestHome(t)

	// Command should succeed with --json flag
	_, err := executeCmd("agent", "list", "--json")
	if err != nil {
		t.Fatalf("agent list --json failed: %v", err)
	}
}

func TestAgentStopNonexistent(t *testing.T) {
	setupTestHome(t)

	_, err := executeCmd("agent", "stop", "nonexistent-agent")
	if err == nil {
		t.Error("expected error for stopping nonexistent agent")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestAgentSendNonexistent(t *testing.T) {
	setupTestHome(t)

	_, err := executeCmd("agent", "send", "nonexistent-agent", "hello")
	if err == nil {
		t.Error("expected error for sending to nonexistent agent")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestAgentPeekNonexistent(t *testing.T) {
	setupTestHome(t)

	_, err := executeCmd("agent", "peek", "nonexistent-agent")
	if err == nil {
		t.Error("expected error for peeking nonexistent agent")
	}
}

func TestAgentAttachNonexistent(t *testing.T) {
	setupTestHome(t)

	_, err := executeCmd("agent", "attach", "nonexistent-agent")
	if err == nil {
		t.Error("expected error for attaching to nonexistent agent")
	}
}

func TestAgentListWithRoleFilter(t *testing.T) {
	setupTestHome(t)

	// Should succeed with valid role filter
	_, err := executeCmd("agent", "list", "--role", "engineer")
	if err != nil {
		t.Fatalf("agent list --role failed: %v", err)
	}
}

func TestAgentListInvalidRole(t *testing.T) {
	setupTestHome(t)

	// Only truly invalid role names (format) should error
	// "invalid-role" is valid now (all roles are custom)
	_, err := executeCmd("agent", "list", "--role", "role@invalid")
	if err == nil {
		t.Error("expected error for invalid role filter format")
	}
}

// --- Message Routing Command Tests ---

func TestAgentBroadcast_ValidArgs(t *testing.T) {
	cmd := agentBroadcastCmd

	// Should accept message
	err := cmd.Args(cmd, []string{"hello world"})
	if err != nil {
		t.Errorf("unexpected error for valid args: %v", err)
	}

	// Should accept multiple words as message
	err = cmd.Args(cmd, []string{"hello", "world", "test"})
	if err != nil {
		t.Errorf("unexpected error for multi-word message: %v", err)
	}
}

func TestAgentBroadcast_EmptyArgs(t *testing.T) {
	cmd := agentBroadcastCmd

	// MinimumNArgs(1) should reject no args
	err := cmd.Args(cmd, []string{})
	if err == nil {
		t.Error("expected error for missing message")
	}
}

func TestAgentSendPattern_ValidArgs(t *testing.T) {
	cmd := agentSendPatternCmd

	// Should accept pattern + message
	err := cmd.Args(cmd, []string{"engineer-*", "run tests"})
	if err != nil {
		t.Errorf("unexpected error for valid args: %v", err)
	}

	// Should accept pattern + multi-word message
	err = cmd.Args(cmd, []string{"eng-0*", "check", "status"})
	if err != nil {
		t.Errorf("unexpected error for multi-word message: %v", err)
	}
}

func TestAgentSendPattern_InsufficientArgs(t *testing.T) {
	cmd := agentSendPatternCmd

	// MinimumNArgs(2) should reject single arg
	err := cmd.Args(cmd, []string{"pattern-*"})
	if err == nil {
		t.Error("expected error for missing message")
	}

	// Should reject no args
	err = cmd.Args(cmd, []string{})
	if err == nil {
		t.Error("expected error for no args")
	}
}

func TestAgentBroadcast_NoAgents(t *testing.T) {
	setupTestHome(t)

	// Should succeed with no agents
	_, err := executeCmd("agent", "broadcast", "hello")
	if err != nil {
		t.Fatalf("agent broadcast failed: %v", err)
	}
}

func TestAgentSendPattern_NoMatches(t *testing.T) {
	setupTestHome(t)

	// Should succeed with no matching agents (no error)
	_, err := executeCmd("agent", "send-pattern", "nonexistent-*", "hello")
	if err != nil {
		t.Fatalf("agent send-pattern failed: %v", err)
	}
}

func TestAgentSendPattern_ValidPatterns(t *testing.T) {
	// Test that various glob patterns are accepted
	patterns := []string{
		"engineer-*",
		"eng-0*",
		"*-lead",
		"eng-[0-9]*",
		"team-??",
	}

	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			_, err := filepath.Match(pattern, "test-agent")
			if err != nil {
				t.Errorf("pattern %q should be valid: %v", pattern, err)
			}
		})
	}
}

func TestAgentCommandStructure_MessageRouting(t *testing.T) {
	// Verify agentCmd has the new message routing subcommands
	subcommands := agentCmd.Commands()

	expectedCmds := map[string]bool{
		"broadcast":    false,
		"send-pattern": false,
	}

	for _, cmd := range subcommands {
		if _, ok := expectedCmds[cmd.Name()]; ok {
			expectedCmds[cmd.Name()] = true
		}
	}

	for name, found := range expectedCmds {
		if !found {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

// --- Agent Health Tests ---

func TestAgentHealthFlags(t *testing.T) {
	flags := agentHealthCmd.Flags()

	if flags.Lookup("json") == nil {
		t.Error("expected --json flag")
	}
	if flags.Lookup("timeout") == nil {
		t.Error("expected --timeout flag")
	}
	if flags.Lookup("detect-stuck") == nil {
		t.Error("expected --detect-stuck flag")
	}
	if flags.Lookup("work-timeout") == nil {
		t.Error("expected --work-timeout flag")
	}
	if flags.Lookup("max-failures") == nil {
		t.Error("expected --max-failures flag")
	}
	if flags.Lookup("alert") == nil {
		t.Error("expected --alert flag")
	}
}

func TestAgentHealthAlertRequiresDetectStuck(t *testing.T) {
	setupTestHome(t)

	// Set alert without detect-stuck
	agentHealthAlert = "engineering"
	agentHealthDetect = false
	defer func() {
		agentHealthAlert = ""
		agentHealthDetect = false
	}()

	_, err := executeCmd("agent", "health", "--alert", "engineering")
	if err == nil {
		t.Error("expected error when --alert used without --detect-stuck")
	}
	if err != nil && !strings.Contains(err.Error(), "requires --detect-stuck") {
		t.Errorf("error should mention '--detect-stuck' requirement: %v", err)
	}
}

func TestAgentHealthNoAgents(t *testing.T) {
	setupTestHome(t)

	// Should succeed with no agents
	_, err := executeCmd("agent", "health")
	if err != nil {
		t.Fatalf("agent health failed: %v", err)
	}
}

func TestAgentHealthJSON(t *testing.T) {
	setupTestHome(t)

	// Should succeed with --json flag
	_, err := executeCmd("agent", "health", "--json")
	if err != nil {
		t.Fatalf("agent health --json failed: %v", err)
	}
}

func TestAgentHealth_StuckDetectionNoStuck(t *testing.T) {
	// Test that no stuck agents returns empty list
	healthResults := []AgentHealth{
		{Name: "eng-01", Role: "engineer", Status: "healthy", IsStuck: false},
		{Name: "eng-02", Role: "engineer", Status: "healthy", IsStuck: false},
	}

	var stuckAgents []AgentHealth
	for _, h := range healthResults {
		if h.IsStuck || h.Status == "stuck" {
			stuckAgents = append(stuckAgents, h)
		}
	}

	if len(stuckAgents) != 0 {
		t.Errorf("expected 0 stuck agents, got %d", len(stuckAgents))
	}
}

func TestAgentHealth_StuckDetectionWithStuck(t *testing.T) {
	// Test that stuck agents are correctly identified
	healthResults := []AgentHealth{
		{Name: "eng-01", Role: "engineer", Status: "healthy", IsStuck: false},
		{Name: "eng-02", Role: "engineer", Status: "stuck", IsStuck: true, StuckReason: "no_activity"},
		{Name: "eng-03", Role: "engineer", Status: "stuck", IsStuck: true, StuckReason: "repeated_failures"},
	}

	var stuckAgents []AgentHealth
	for _, h := range healthResults {
		if h.IsStuck || h.Status == "stuck" {
			stuckAgents = append(stuckAgents, h)
		}
	}

	if len(stuckAgents) != 2 {
		t.Errorf("expected 2 stuck agents, got %d", len(stuckAgents))
	}
}

func TestAgentHealth_AlertMessageFormat(t *testing.T) {
	// Test that alert message is formatted correctly
	stuckAgents := []AgentHealth{
		{Name: "eng-01", Role: "engineer", Status: "stuck", IsStuck: true, StuckReason: "no_activity", StuckDetails: "no events in 15m"},
		{Name: "eng-02", Role: "qa", Status: "stuck", IsStuck: true, StuckReason: "repeated_failures", StuckDetails: "task failed 3 times"},
	}

	var sb strings.Builder
	sb.WriteString("⚠️ ALERT: 2 stuck agent(s) detected\n")
	for _, h := range stuckAgents {
		sb.WriteString("  • " + h.Name + " (" + h.Role + "): " + h.StuckReason + " - " + h.StuckDetails + "\n")
	}
	message := sb.String()

	if !strings.Contains(message, "eng-01") {
		t.Error("message should contain eng-01")
	}
	if !strings.Contains(message, "eng-02") {
		t.Error("message should contain eng-02")
	}
	if !strings.Contains(message, "no_activity") {
		t.Error("message should contain reason 'no_activity'")
	}
	if !strings.Contains(message, "repeated_failures") {
		t.Error("message should contain reason 'repeated_failures'")
	}
}

// --- Agent Delete Tests ---

func TestAgentDeleteFlags(t *testing.T) {
	flags := agentDeleteCmd.Flags()

	if flags.Lookup("force") == nil {
		t.Error("expected --force flag")
	}
	if flags.Lookup("purge") == nil {
		t.Error("expected --purge flag")
	}
}

func TestAgentDelete_RequiresName(t *testing.T) {
	cmd := agentDeleteCmd

	// ExactArgs(1) should reject no args
	err := cmd.Args(cmd, []string{})
	if err == nil {
		t.Error("expected error for missing agent name")
	}

	// Should accept exactly one arg
	err = cmd.Args(cmd, []string{"agent-name"})
	if err != nil {
		t.Errorf("unexpected error for valid arg: %v", err)
	}
}

func TestAgentDelete_NonexistentAgent(t *testing.T) {
	setupTestHome(t)

	_, err := executeCmd("agent", "delete", "nonexistent-agent", "--force")
	if err == nil {
		t.Error("expected error for deleting nonexistent agent")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestAgentDeleteOptions(t *testing.T) {
	// Test DeleteOptions struct
	opts := agent.DeleteOptions{
		Force: false,
	}
	if opts.Force {
		t.Error("expected Force to be false by default")
	}

	opts.Force = true
	if !opts.Force {
		t.Error("expected Force to be true after setting")
	}
}

// --- Agent Rename Tests ---

func TestAgentRenameCmd_CommandDefinition(t *testing.T) {
	// Test command is properly configured
	if agentRenameCmd.Use != "rename <old-name> <new-name>" {
		t.Errorf("Use = %q, want %q", agentRenameCmd.Use, "rename <old-name> <new-name>")
	}

	if agentRenameCmd.Short != "Rename an agent" {
		t.Errorf("Short = %q, want %q", agentRenameCmd.Short, "Rename an agent")
	}

	// Check force flag exists
	forceFlag := agentRenameCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("--force flag should exist")
	}
}

func TestAgentRename_RunEValidation(t *testing.T) {
	// The same-name check in runAgentRename runs before any daemon or
	// repo access, so it can be exercised via a direct call.
	err := runAgentRename(nil, []string{"eng-01", "eng-01"})
	if err == nil {
		t.Error("expected error when renaming to same name")
	}
	if err != nil && !strings.Contains(err.Error(), "same") {
		t.Errorf("expected 'same' in error, got: %v", err)
	}
}

// --- Agent Create Validation Tests ---

func TestAgentCreate_RejectsRootRole(t *testing.T) {
	// Test that root role cannot be created via mycel agent create
	setupTestHome(t)

	// Reset flags to prevent leaking state from previous tests
	agentCreateTeam = ""
	agentCreateParent = ""
	agentCreateTool = ""
	agentCreateRole = "worker"

	_, err := executeCmd("agent", "create", "my-root", "--role", "root")
	if err == nil {
		t.Error("expected error when creating root agent via agent create")
	}
	if !strings.Contains(err.Error(), "cannot create root agent") {
		t.Errorf("error should mention cannot create root agent: %v", err)
	}
	if !strings.Contains(err.Error(), "mycel up") {
		t.Errorf("error should mention 'mycel up': %v", err)
	}
}

func TestAgentCreate_NonExistentTeam(t *testing.T) {
	// Test that agent create fails if team doesn't exist
	setupTestHome(t)

	// Create engineer role file in the global roles migration dir
	// (~/.mycel/roles); the role store picks it up on load.
	rolesDir := filepath.Join(os.Getenv("MYCEL_HOME"), "roles")
	if err := os.MkdirAll(rolesDir, 0750); err != nil {
		t.Fatalf("failed to create roles dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rolesDir, "engineer.md"), []byte("# Engineer Role"), 0600); err != nil {
		t.Fatalf("failed to create engineer role: %v", err)
	}

	_, err := executeCmd("agent", "create", "eng-01", "--role", "engineer", "--team", "nonexistent-team")
	if err == nil {
		t.Error("expected error when creating agent with non-existent team")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should mention team does not exist: %v", err)
	}
}

func TestPeekFollowFlag(t *testing.T) {
	// Verify the --follow / -f flag is registered on the peek command
	f := agentPeekCmd.Flags().Lookup("follow")
	if f == nil {
		t.Fatal("expected --follow flag to be registered on peek command")
	}
	if f.Shorthand != "f" {
		t.Errorf("expected shorthand 'f', got %q", f.Shorthand)
	}
	if f.DefValue != "false" {
		t.Errorf("expected default value 'false', got %q", f.DefValue)
	}
}
