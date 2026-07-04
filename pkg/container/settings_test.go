package container

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSeedClaudeSettings_CreatesFile(t *testing.T) {
	dir := t.TempDir()

	if err := SeedClaudeSettings(dir); err != nil {
		t.Fatalf("SeedClaudeSettings() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "settings.json")) //nolint:gosec // test uses temp dir
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	if settings["theme"] != "dark" {
		t.Errorf("theme = %v, want %q", settings["theme"], "dark")
	}
	if settings["skipDangerousModePermissionPrompt"] != true {
		t.Errorf("skipDangerousModePermissionPrompt = %v, want true", settings["skipDangerousModePermissionPrompt"])
	}
}

func TestSeedClaudeSettings_PreservesExisting(t *testing.T) {
	dir := t.TempDir()

	existing := []byte(`{"theme":"light","custom":"value"}`)
	settingsPath := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settingsPath, existing, 0600); err != nil { //nolint:gosec // test uses temp dir
		t.Fatalf("failed to write existing settings: %v", err)
	}

	if err := SeedClaudeSettings(dir); err != nil {
		t.Fatalf("SeedClaudeSettings() error = %v", err)
	}

	data, err := os.ReadFile(settingsPath) //nolint:gosec // test uses temp dir
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	// Existing user values must be preserved
	if settings["theme"] != "light" {
		t.Errorf("theme = %v, want %q (should preserve user value)", settings["theme"], "light")
	}
	if settings["custom"] != "value" {
		t.Errorf("custom = %v, want %q (should preserve user value)", settings["custom"], "value")
	}
}

func readClaudeJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test path
	if err != nil {
		t.Fatalf("read claude.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal claude.json: %v", err)
	}
	return m
}

func trustedEntry(t *testing.T, root map[string]any, project string) map[string]any {
	t.Helper()
	projects, ok := root["projects"].(map[string]any)
	if !ok {
		t.Fatalf("projects key missing: %v", root)
	}
	entry, ok := projects[project].(map[string]any)
	if !ok {
		t.Fatalf("project %q missing: %v", project, projects)
	}
	return entry
}

// Docker runtime: the trusted key must be the container-side path.
func TestSeedClaudeTrust_CreatesFileWithContainerPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claude.json")

	if err := SeedClaudeTrust(path, "/workspace"); err != nil {
		t.Fatalf("SeedClaudeTrust() error = %v", err)
	}

	entry := trustedEntry(t, readClaudeJSON(t, path), "/workspace")
	if accepted, _ := entry["hasTrustDialogAccepted"].(bool); !accepted {
		t.Error("hasTrustDialogAccepted not true for /workspace")
	}
}

// Tmux runtime: the trusted key is the host worktree path, and existing
// keys (auth, other projects) must survive the merge.
func TestSeedClaudeTrust_MergesWithoutClobbering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claude.json")
	seed := `{
		"oauthAccount": {"email": "user@example.com"},
		"projects": {
			"/existing": {"hasTrustDialogAccepted": true, "history": ["x"]}
		}
	}`
	if err := os.WriteFile(path, []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}

	worktree := "/home/user/.mycel/worktrees/eng-01"
	if err := SeedClaudeTrust(path, worktree); err != nil {
		t.Fatalf("SeedClaudeTrust() error = %v", err)
	}

	root := readClaudeJSON(t, path)
	if _, ok := root["oauthAccount"].(map[string]any); !ok {
		t.Error("oauthAccount was clobbered")
	}
	existing := trustedEntry(t, root, "/existing")
	if _, ok := existing["history"]; !ok {
		t.Error("existing project entry was clobbered")
	}
	entry := trustedEntry(t, root, worktree)
	if accepted, _ := entry["hasTrustDialogAccepted"].(bool); !accepted {
		t.Errorf("hasTrustDialogAccepted not true for %q", worktree)
	}
}

func TestSeedClaudeTrust_IdempotentWhenAlreadyTrusted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claude.json")
	seed := `{"projects": {"/workspace": {"hasTrustDialogAccepted": true}}, "custom": 1}`
	if err := os.WriteFile(path, []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}

	if err := SeedClaudeTrust(path, "/workspace"); err != nil {
		t.Fatalf("SeedClaudeTrust() error = %v", err)
	}

	// Already trusted — the file must be left byte-for-byte untouched.
	data, err := os.ReadFile(path) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != seed {
		t.Errorf("file rewritten despite already-trusted entry: %s", data)
	}
}

func TestSeedClaudeTrust_RejectsInvalidInputs(t *testing.T) {
	if err := SeedClaudeTrust("", "/workspace"); err == nil {
		t.Error("accepted empty claude.json path")
	}
	if err := SeedClaudeTrust(filepath.Join(t.TempDir(), "claude.json"), ""); err == nil {
		t.Error("accepted empty project path")
	}
}
