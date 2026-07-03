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
