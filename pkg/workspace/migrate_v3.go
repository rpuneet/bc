package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rpuneet/bc/pkg/log"
)

// MigrateToGlobalState moves workspace state from <project>/.bc/ to
// ~/.bc/workspaces/<id>/. Leaves a .bc-migrated marker in the old location.
// Returns the new state directory path.
func MigrateToGlobalState(rootDir string) (string, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}

	legacyDir := filepath.Join(absRoot, ".bc")
	if _, statErr := os.Stat(legacyDir); statErr != nil {
		return "", fmt.Errorf("no legacy .bc/ directory found in %s", absRoot)
	}

	newDir, err := GlobalStateDir(absRoot)
	if err != nil {
		return "", err
	}

	if homeErr := EnsureBCHome(); homeErr != nil {
		return "", homeErr
	}

	// Create new state directory
	if mkdirErr := os.MkdirAll(newDir, 0750); mkdirErr != nil {
		return "", fmt.Errorf("failed to create %s: %w", newDir, mkdirErr)
	}

	// Copy files from legacy to new location
	entries, err := os.ReadDir(legacyDir)
	if err != nil {
		return "", fmt.Errorf("failed to read legacy dir: %w", err)
	}

	// Only migrate essential state files — skip agent worktrees and
	// Claude session data (can be multi-GB). Agents will be recreated.
	essentialFiles := map[string]bool{
		PreferencesFileName:    true, // preferences.json (M11c+)
		LegacySettingsFileName: true, // settings.json (legacy)
		"bc.db":                true,
		"state.db":             true,
		"cron.db":              true,
		"channels.db":          true,
		"cost.db":              true,
	}
	essentialDirs := map[string]bool{
		"roles": true,
		"logs":  true,
	}

	for _, entry := range entries {
		name := entry.Name()
		src := filepath.Join(legacyDir, name)
		dst := filepath.Join(newDir, name)

		if entry.IsDir() {
			if !essentialDirs[name] {
				log.Debug("migration: skipping directory", "name", name)
				continue
			}
			if cpErr := copyDir(src, dst); cpErr != nil {
				log.Warn("migration: failed to copy directory", "src", src, "error", cpErr)
				continue
			}
		} else {
			if !essentialFiles[name] {
				log.Debug("migration: skipping file", "name", name)
				continue
			}
			if cpErr := migrateFile(src, dst); cpErr != nil {
				log.Warn("migration: failed to copy file", "src", src, "error", cpErr)
				continue
			}
		}
	}

	// Write .bc-migrated marker in the old location
	marker := filepath.Join(legacyDir, ".bc-migrated")
	markerContent := fmt.Sprintf("Workspace state migrated to: %s\n", newDir)
	_ = os.WriteFile(marker, []byte(markerContent), 0644) //nolint:gosec

	log.Info("migrated workspace state", "from", legacyDir, "to", newDir)
	return newDir, nil
}

// NeedsMigration returns true if the workspace has legacy .bc/ state
// that hasn't been migrated to ~/.bc/workspaces/<id>/ yet.
func NeedsMigration(rootDir string) bool {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return false
	}

	legacyDir := filepath.Join(absRoot, ".bc")

	// Legacy exists? (either preferences.json or settings.json)
	hasLegacy := false
	for _, name := range []string{PreferencesFileName, LegacySettingsFileName} {
		if _, err := os.Stat(filepath.Join(legacyDir, name)); err == nil {
			hasLegacy = true
			break
		}
	}
	if !hasLegacy {
		return false
	}

	// Already migrated?
	if _, err := os.Stat(filepath.Join(legacyDir, ".bc-migrated")); err == nil {
		return false
	}

	// Global dir already has a config file? (either name)
	globalDir, gErr := GlobalStateDir(absRoot)
	if gErr != nil {
		return false
	}
	for _, name := range []string{PreferencesFileName, LegacySettingsFileName} {
		if _, err := os.Stat(filepath.Join(globalDir, name)); err == nil {
			return false // already migrated
		}
	}

	return true
}

func migrateFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck

	out, err := os.Create(dst) //nolint:gosec
	if err != nil {
		return err
	}
	defer out.Close() //nolint:errcheck

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0750); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := migrateFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}
