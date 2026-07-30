package cmd

import (
	"os"
	"strings"
	"testing"
)

// --- Input Validation Tests ---

// TestInputValidation_SpecialCharacters tests that special characters in names are rejected
func TestInputValidation_SpecialCharacters(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	tests := []struct {
		name     string
		args     []string
		wantErrs []string // any of these errors is acceptable
	}{
		{
			name:     "agent name with semicolon",
			args:     []string{"agent", "create", "test;agent", "--role", "engineer"},
			wantErrs: []string{"invalid"},
		},
		{
			name:     "agent name with quotes",
			args:     []string{"agent", "create", "test'agent", "--role", "engineer"},
			wantErrs: []string{"invalid"},
		},
		{
			name:     "channel name with special chars",
			args:     []string{"channel", "create", "test@channel"},
			wantErrs: []string{"invalid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := executeIntegrationCmd(tt.args...)
			if err == nil {
				t.Errorf("expected error for %v, got nil", tt.args)
				return
			}
			errLower := strings.ToLower(err.Error())
			found := false
			for _, wantErr := range tt.wantErrs {
				if strings.Contains(errLower, wantErr) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error containing one of %v, got: %v", tt.wantErrs, err)
			}
		})
	}
}

// TestInputValidation_SQLInjection tests that SQL injection patterns are handled safely
func TestInputValidation_SQLInjection(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// These inputs should either be rejected or handled safely (no SQL errors)
	injectionPatterns := []string{
		"'; DROP TABLE agents; --",
		"1 OR 1=1",
		"admin'--",
		"'; DELETE FROM channels; --",
		"UNION SELECT * FROM users",
	}

	for _, pattern := range injectionPatterns {
		t.Run("injection_pattern", func(t *testing.T) {
			// Try to use injection pattern as agent name
			_, _, err := executeIntegrationCmd("agent", "show", pattern)
			// Should either reject as invalid name or return "not found" - never SQL error
			if err != nil {
				errLower := strings.ToLower(err.Error())
				if strings.Contains(errLower, "sql") && !strings.Contains(errLower, "not found") {
					t.Errorf("possible SQL error exposed: %v", err)
				}
			}
		})
	}
}

// TestInputValidation_EmptyInputs tests that empty inputs are properly rejected
func TestInputValidation_EmptyInputs(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "channel send with empty message",
			args:    []string{"channel", "send", "eng", ""},
			wantErr: true,
		},
		{
			name:    "team create with empty name",
			args:    []string{"team", "create", ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := executeIntegrationCmd(tt.args...)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for %v, got nil", tt.args)
			}
		})
	}
}

// TestInputValidation_LongStrings tests handling of very long input strings
func TestInputValidation_LongStrings(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	// Create a very long string (1000+ chars)
	longString := strings.Repeat("a", 1000)

	// These should either work or fail gracefully (no panics, no buffer overflows)
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "long agent name",
			args: []string{"agent", "create", longString, "--role", "engineer"},
		},
		{
			name: "long channel name",
			args: []string{"channel", "create", longString},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify no panic - error is expected
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on long string: %v", r)
				}
			}()
			_, _, _ = executeIntegrationCmd(tt.args...)
		})
	}
}

// --- Non-Existent Resource Tests ---

// TestNonExistentAgent tests operations on non-existent agents
func TestNonExistentAgent(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	tests := []struct {
		name    string
		wantErr string
		args    []string
	}{
		{
			name:    "show non-existent agent",
			args:    []string{"agent", "show", "nonexistent-agent-xyz"},
			wantErr: "not found",
		},
		{
			name:    "stop non-existent agent",
			args:    []string{"agent", "stop", "nonexistent-agent-xyz"},
			wantErr: "not found",
		},
		{
			name:    "attach to non-existent agent",
			args:    []string{"agent", "attach", "nonexistent-agent-xyz"},
			wantErr: "not", // "not found" or "not running"
		},
		{
			name:    "peek non-existent agent",
			args:    []string{"agent", "peek", "nonexistent-agent-xyz"},
			wantErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := executeIntegrationCmd(tt.args...)
			if err == nil {
				t.Errorf("expected error for %v, got nil", tt.args)
				return
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

// TestNonExistentChannel tests operations on non-existent channels
func TestNonExistentChannel(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	tests := []struct {
		name    string
		wantErr string
		args    []string
	}{
		{
			name:    "send to non-existent channel",
			args:    []string{"channel", "send", "nonexistent-channel", "hello"},
			wantErr: "not found",
		},
		{
			name:    "history of non-existent channel",
			args:    []string{"channel", "history", "nonexistent-channel"},
			wantErr: "not found",
		},
		{
			name:    "delete non-existent channel",
			args:    []string{"channel", "delete", "nonexistent-channel"},
			wantErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := executeIntegrationCmd(tt.args...)
			if err == nil {
				t.Errorf("expected error for %v, got nil", tt.args)
				return
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

// Team command removed in CLI restructure (#1916)

// --- State Violation Tests ---

// --- Empty List Tests ---

// TestEmptyLists tests listing resources when none exist
func TestEmptyLists(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "empty agent list",
			args: []string{"agent", "list"},
		},
		{
			name: "empty channel list",
			args: []string{"channel", "list"},
		},
		// team, demon, process commands removed in CLI restructure (#1916)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Empty lists should not error, just return empty
			_, _, err := executeIntegrationCmd(tt.args...)
			if err != nil {
				// Skip known flag leakage issues from other tests in the package
				// (role flags persist across test runs due to global state)
				if strings.Contains(err.Error(), "role@invalid") {
					t.Skip("skipping due to flag leakage from other tests")
				}
				t.Errorf("empty list command %v should not error: %v", tt.args, err)
			}
		})
	}
}

// --- Missing Required Arguments Tests ---

// TestMissingRequiredArgs tests commands with missing required arguments
func TestMissingRequiredArgs(t *testing.T) {
	_, cleanup := setupIntegrationWorkspace(t)
	defer cleanup()

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "agent create without name",
			args: []string{"agent", "create"},
		},
		{
			name: "channel send without channel",
			args: []string{"channel", "send"},
		},
		{
			name: "channel send without message",
			args: []string{"channel", "send", "eng"},
		},
		{
			name: "team create without name",
			args: []string{"team", "create"},
		},
		{
			name: "team add without member",
			args: []string{"team", "add", "myteam"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := executeIntegrationCmd(tt.args...)
			if err == nil {
				t.Errorf("expected error for missing args in %v, got nil", tt.args)
			}
		})
	}
}

// --- Unicode and Special Data Tests ---

// --- No Workspace Tests ---

// TestNoWorkspace tests the commands that still require an enclosing
// mycel-adopted repo. Daemon-first commands (agent list, channel list,
// logs, cost, ...) are CWD-free and covered elsewhere.
func TestNoWorkspace(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir := t.TempDir()
	if err = os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Clear MYCEL_WORKSPACE to test directory-based repo detection
	restoreEnv := clearWorkspaceEnv(t)
	defer restoreEnv()

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "status outside repo",
			args: []string{"status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := executeIntegrationCmd(tt.args...)
			if err == nil {
				t.Errorf("expected workspace error for %v, got nil", tt.args)
				return
			}
			if !strings.Contains(strings.ToLower(err.Error()), "repo") {
				t.Errorf("expected repo-related error, got: %v", err)
			}
		})
	}
}
