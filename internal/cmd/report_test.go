package cmd

import (
	"os"
	"strings"
	"testing"
)

// --- Report Command Unit Tests ---

func TestReport_NoAgentID(t *testing.T) {
	setupTestWorkspace(t)

	// Clear MYCEL_AGENT_ID env var
	origAgentID := os.Getenv("MYCEL_AGENT_ID")
	_ = os.Unsetenv("MYCEL_AGENT_ID")
	defer func() {
		if origAgentID != "" {
			_ = os.Setenv("MYCEL_AGENT_ID", origAgentID)
		}
	}()

	// Report without MYCEL_AGENT_ID should fail
	_, err := executeCmd("agent", "report", "working", "test message")
	if err == nil {
		t.Error("expected error when MYCEL_AGENT_ID not set")
	}
	if err != nil && !strings.Contains(err.Error(), "this command can only be run by agents in the mycel system") {
		t.Errorf("expected agent-only command error, got: %v", err)
	}
}

func TestReport_InvalidState(t *testing.T) {
	setupTestWorkspace(t)

	// Set MYCEL_AGENT_ID
	origAgentID := os.Getenv("MYCEL_AGENT_ID")
	_ = os.Setenv("MYCEL_AGENT_ID", "test-agent")
	defer func() {
		if origAgentID != "" {
			_ = os.Setenv("MYCEL_AGENT_ID", origAgentID)
		} else {
			_ = os.Unsetenv("MYCEL_AGENT_ID")
		}
	}()

	// Report with invalid state should fail
	_, err := executeCmd("agent", "report", "invalid-state", "test message")
	if err == nil {
		t.Error("expected error for invalid state")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid state") {
		t.Errorf("expected invalid state error, got: %v", err)
	}
}

// --- Report Command Args Tests ---

func TestReportCommand_RequiresState(t *testing.T) {
	// Report requires at least 1 arg (state)
	err := agentReportCmd.Args(agentReportCmd, []string{})
	if err == nil {
		t.Error("expected error for missing state arg")
	}
}

func TestReportCommand_AcceptsStateOnly(t *testing.T) {
	// Report accepts state only
	err := agentReportCmd.Args(agentReportCmd, []string{"working"})
	if err != nil {
		t.Errorf("unexpected error for state-only args: %v", err)
	}
}

func TestReportCommand_AcceptsStateAndMessage(t *testing.T) {
	// Report accepts state + message
	err := agentReportCmd.Args(agentReportCmd, []string{"working", "test", "message"})
	if err != nil {
		t.Errorf("unexpected error for state + message args: %v", err)
	}
}

// --- Enhanced Stuck Report Tests (#675) ---

func TestReportCommand_StuckFlags(t *testing.T) {
	// Test that stuck report flags are defined
	reasonFlag := agentReportCmd.Flags().Lookup("reason")
	if reasonFlag == nil {
		t.Error("--reason flag should be defined")
	}

	reproductionFlag := agentReportCmd.Flags().Lookup("reproduction")
	if reproductionFlag == nil {
		t.Error("--reproduction flag should be defined")
	}

	severityFlag := agentReportCmd.Flags().Lookup("severity")
	if severityFlag == nil {
		t.Error("--severity flag should be defined")
	}

	// Verify default severity
	if severityFlag != nil && severityFlag.DefValue != "medium" {
		t.Errorf("--severity default should be 'medium', got %q", severityFlag.DefValue)
	}
}
