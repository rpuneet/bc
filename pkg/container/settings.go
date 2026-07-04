package container

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// requiredClaudeSettings are fields that MUST be present to prevent
// Claude Code from showing interactive prompts that block Docker agents.
var requiredClaudeSettings = map[string]any{
	"theme":                             "dark",
	"skipDangerousModePermissionPrompt": true,
	"autoUpdaterStatus":                 "disabled",
}

// SeedClaudeSettings ensures required settings exist in settings.json.
// If the file doesn't exist, creates it. If it exists, merges in any
// missing required fields without overwriting user customizations.
func SeedClaudeSettings(volumeDir string) error {
	// volumeDir derives from agent state paths; reject anything that
	// escaped upstream validation before touching the filesystem.
	volumeDir = filepath.Clean(volumeDir)
	if volumeDir == "" || volumeDir == "." || strings.Contains(volumeDir, "..") {
		return fmt.Errorf("invalid volume dir %q", volumeDir)
	}
	settingsPath := filepath.Join(volumeDir, "settings.json")

	// Load existing settings if present
	existing := map[string]any{}
	if data, err := os.ReadFile(settingsPath); err == nil { //nolint:gosec // trusted path
		_ = json.Unmarshal(data, &existing)
	}

	// Merge required fields — only set if missing
	changed := false
	for k, v := range requiredClaudeSettings {
		if _, ok := existing[k]; !ok {
			existing[k] = v
			changed = true
		}
	}

	if !changed && len(existing) > 0 {
		return nil // all required fields already present
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(settingsPath, data, 0600)
}

// SeedClaudeTrust marks projectPath as trusted in the claude.json at
// claudeJSONPath so Claude Code skips the interactive "trust this folder"
// prompt that hangs headless agents. It merges
//
//	{"projects": {"<projectPath>": {"hasTrustDialogAccepted": true}}}
//
// into the file — creating it when missing and never clobbering other
// keys (auth, oauthAccount, other projects). For Docker agents the
// projectPath must be the container-side path (/workspace); for tmux
// agents it is the host worktree path.
func SeedClaudeTrust(claudeJSONPath, projectPath string) error {
	claudeJSONPath = filepath.Clean(claudeJSONPath)
	if claudeJSONPath == "" || claudeJSONPath == "." || strings.Contains(claudeJSONPath, "..") {
		return fmt.Errorf("invalid claude.json path %q", claudeJSONPath)
	}
	if projectPath == "" {
		return fmt.Errorf("project path is required")
	}

	root := map[string]any{}
	if data, err := os.ReadFile(claudeJSONPath); err == nil { //nolint:gosec // trusted path
		_ = json.Unmarshal(data, &root)
	}

	projects, _ := root["projects"].(map[string]any)
	if projects == nil {
		projects = map[string]any{}
	}
	entry, _ := projects[projectPath].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
	}
	if accepted, _ := entry["hasTrustDialogAccepted"].(bool); accepted {
		return nil // already trusted — leave the file untouched
	}
	entry["hasTrustDialogAccepted"] = true
	projects[projectPath] = entry
	root["projects"] = projects

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(claudeJSONPath), 0750); err != nil {
		return err
	}
	return os.WriteFile(claudeJSONPath, data, 0600)
}
