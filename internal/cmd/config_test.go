package cmd

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/home"
)

func TestConfigShow(t *testing.T) {
	_ = setupTestHome(t)

	stdout, _, err := executeIntegrationCmd("config", "show")
	if err != nil {
		t.Fatalf("config show failed: %v", err)
	}

	// Check that output contains expected sections
	expectedSections := []string{
		"[server]",
		"[providers]",
	}

	for _, section := range expectedSections {
		if !strings.Contains(stdout, section) {
			t.Errorf("expected output to contain %s, got:\n%s", section, stdout)
		}
	}
}

func TestConfigShowSection(t *testing.T) {
	_ = setupTestHome(t)

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
	_ = setupTestHome(t)

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
	_ = setupTestHome(t)

	tests := []struct {
		key      string
		expected string
	}{
		{"providers.default", "claude"},
		{"server.host", "127.0.0.1"},
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
	_ = setupTestHome(t)

	_, _, err := executeIntegrationCmd("config", "get", "invalid.key")
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}

	if !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("expected 'unknown config key' error, got: %v", err)
	}
}

func TestConfigSet(t *testing.T) {
	_ = setupTestHome(t)

	// Set user.name (safe key that doesn't trigger provider validation issues)
	stdout, _, err := executeIntegrationCmd("config", "set", "user.name", "newname")
	if err != nil {
		t.Fatalf("config set user.name=newname failed: %v", err)
	}

	if !strings.Contains(stdout, "Set user.name") {
		t.Errorf("expected confirmation message, got: %s", stdout)
	}

	// Verify the value was set
	stdout, _, err = executeIntegrationCmd("config", "get", "user.name")
	if err != nil {
		t.Fatalf("config get user.name failed: %v", err)
	}

	stdout = strings.TrimSpace(stdout)
	if stdout != "newname" {
		t.Errorf("expected %q, got %q", "newname", stdout)
	}
}

func TestConfigSetInvalidValue(t *testing.T) {
	_ = setupTestHome(t)

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
	_ = setupTestHome(t)

	stdout, _, err := executeIntegrationCmd("config", "list")
	if err != nil {
		t.Fatalf("config list failed: %v", err)
	}

	expectedKeys := []string{
		"user.name",
		"server.host",
		"providers.default",
	}

	for _, key := range expectedKeys {
		if !strings.Contains(stdout, key) {
			t.Errorf("expected output to contain key %s", key)
		}
	}
}

func TestConfigListJSON(t *testing.T) {
	_ = setupTestHome(t)

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
	hasProvidersDefault := false
	for _, key := range keys {
		if key == "providers.default" {
			hasProvidersDefault = true
			break
		}
	}
	if !hasProvidersDefault {
		t.Error("expected 'providers.default' in keys list")
	}
}

func TestConfigValidate(t *testing.T) {
	_ = setupTestHome(t)

	stdout, _, err := executeIntegrationCmd("config", "validate")
	if err != nil {
		t.Fatalf("config validate failed: %v", err)
	}

	if !strings.Contains(stdout, "Config is valid") {
		t.Errorf("expected 'Config is valid' message, got: %s", stdout)
	}
}

func TestConfigValidateInvalid(t *testing.T) {
	setupTestHome(t)

	// Break the global config (~/.mycel/prefs.json) with an invalid version
	prefsPath, ppErr := home.PrefsPath()
	if ppErr != nil {
		t.Fatal(ppErr)
	}
	if err := os.WriteFile(prefsPath, []byte(`{"version":99}`), 0600); err != nil {
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

func TestConfigShowIsCWDFree(t *testing.T) {
	// Config is global (~/.mycel/prefs.json) and served by the daemon:
	// `config show` works from any directory when bcd answers.
	tmpDir := t.TempDir() // plain dir, not a git repo
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYCEL_WORKSPACE", "")

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/settings" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}

	stdout, _, err := executeIntegrationCmdT(t, handler, "config", "show")
	if err != nil {
		t.Fatalf("config show must be CWD-free via the daemon, got: %v", err)
	}
	if !strings.Contains(stdout, "[providers]") {
		t.Errorf("expected providers section in output, got: %s", stdout)
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
