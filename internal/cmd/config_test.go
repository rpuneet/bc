package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/workspace"
)

func TestConfigShow(t *testing.T) {
	_ = setupTestWorkspace(t)

	stdout, _, err := executeIntegrationCmd("config", "show")
	if err != nil {
		t.Fatalf("config show failed: %v", err)
	}

	// Check that output contains expected sections
	expectedSections := []string{
		"[workspace]",
		"[providers]",
	}

	for _, section := range expectedSections {
		if !strings.Contains(stdout, section) {
			t.Errorf("expected output to contain %s, got:\n%s", section, stdout)
		}
	}
}

func TestConfigShowSection(t *testing.T) {
	_ = setupTestWorkspace(t)

	stdout, _, err := executeIntegrationCmd("config", "show", "providers")
	if err != nil {
		t.Fatalf("config show providers failed: %v", err)
	}

	if !strings.Contains(stdout, "default") {
		t.Errorf("expected output to contain 'default', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "claude") {
		t.Errorf("expected output to contain 'claude', got:\n%s", stdout)
	}
}

func TestConfigShowJSON(t *testing.T) {
	_ = setupTestWorkspace(t)

	stdout, _, err := executeIntegrationCmd("config", "show", "providers", "--json")
	if err != nil {
		t.Fatalf("config show --json failed: %v", err)
	}

	// Parse JSON output
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &data); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	// Check expected fields
	if _, ok := data["Default"]; !ok {
		t.Error("expected 'Default' field in JSON output")
	}
}

func TestConfigGet(t *testing.T) {
	_ = setupTestWorkspace(t)

	tests := []struct {
		key      string
		expected string
	}{
		{"providers.default", "claude"},
		{"workspace.name", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			stdout, _, err := executeIntegrationCmd("config", "get", tt.key)
			if err != nil {
				t.Fatalf("config get %s failed: %v", tt.key, err)
			}

			stdout = strings.TrimSpace(stdout)
			if stdout != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, stdout)
			}
		})
	}
}

func TestConfigGetInvalidKey(t *testing.T) {
	_ = setupTestWorkspace(t)

	_, _, err := executeIntegrationCmd("config", "get", "invalid.key")
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}

	if !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("expected 'unknown config key' error, got: %v", err)
	}
}

func TestConfigSet(t *testing.T) {
	_ = setupTestWorkspace(t)

	// Set workspace.name (safe key that doesn't trigger provider validation issues)
	stdout, _, err := executeIntegrationCmd("config", "set", "workspace.name", "newname")
	if err != nil {
		t.Fatalf("config set workspace.name=newname failed: %v", err)
	}

	if !strings.Contains(stdout, "Set workspace.name") {
		t.Errorf("expected confirmation message, got: %s", stdout)
	}

	// Verify the value was set
	stdout, _, err = executeIntegrationCmd("config", "get", "workspace.name")
	if err != nil {
		t.Fatalf("config get workspace.name failed: %v", err)
	}

	stdout = strings.TrimSpace(stdout)
	if stdout != "newname" {
		t.Errorf("expected %q, got %q", "newname", stdout)
	}
}

func TestConfigSetInvalidValue(t *testing.T) {
	_ = setupTestWorkspace(t)

	tests := []struct {
		key   string
		value string
		desc  string
	}{
		{"nonexistent.key", "value", "unknown key"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_, _, err := executeIntegrationCmd("config", "set", tt.key, tt.value)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.desc)
			}
		})
	}
}

func TestConfigList(t *testing.T) {
	_ = setupTestWorkspace(t)

	stdout, _, err := executeIntegrationCmd("config", "list")
	if err != nil {
		t.Fatalf("config list failed: %v", err)
	}

	expectedKeys := []string{
		"workspace.name",
		"workspace.version",
		"providers.default",
	}

	for _, key := range expectedKeys {
		if !strings.Contains(stdout, key) {
			t.Errorf("expected output to contain key %s", key)
		}
	}
}

func TestConfigListJSON(t *testing.T) {
	_ = setupTestWorkspace(t)

	stdout, _, err := executeIntegrationCmd("config", "list", "--json")
	if err != nil {
		t.Fatalf("config list --json failed: %v", err)
	}

	// Parse JSON output
	var keys []string
	if err := json.Unmarshal([]byte(stdout), &keys); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	if len(keys) == 0 {
		t.Error("expected at least one config key")
	}

	// Check for expected keys
	hasWorkspaceName := false
	for _, key := range keys {
		if key == "workspace.name" {
			hasWorkspaceName = true
			break
		}
	}
	if !hasWorkspaceName {
		t.Error("expected 'workspace.name' in keys list")
	}
}

func TestConfigValidate(t *testing.T) {
	_ = setupTestWorkspace(t)

	stdout, _, err := executeIntegrationCmd("config", "validate")
	if err != nil {
		t.Fatalf("config validate failed: %v", err)
	}

	if !strings.Contains(stdout, "Config is valid") {
		t.Errorf("expected 'Config is valid' message, got: %s", stdout)
	}
}

func TestConfigValidateInvalid(t *testing.T) {
	projectDir := setupTestWorkspace(t)

	// Break the config by setting invalid version
	configPath, cpErr := workspace.ConfigPath(projectDir)
	if cpErr != nil {
		t.Fatal(cpErr)
	}
	if err := os.WriteFile(configPath, []byte(`{"version":99}`), 0600); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeIntegrationCmd("config", "validate")
	if err == nil {
		t.Fatal("expected validation error for invalid config")
	}

	if !strings.Contains(err.Error(), "version") {
		t.Errorf("expected version validation error, got: %v", err)
	}
}

func TestConfigNoWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeIntegrationCmd("config", "show")
	if err == nil {
		t.Fatal("expected error when not in workspace")
	}

	if !strings.Contains(err.Error(), "not in a mycel-adopted repo") {
		t.Errorf("expected 'not in a mycel-adopted repo' error, got: %v", err)
	}
}

func TestConfigCommandStructure(t *testing.T) {
	subcommands := configCmd.Commands()

	expectedCmds := map[string]bool{
		"show":     false,
		"get":      false,
		"set":      false,
		"list":     false,
		"edit":     false,
		"validate": false,
		"reset":    false,
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
