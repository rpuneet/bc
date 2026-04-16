package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// BCHome returns the global bc home directory (~/.bc).
// Respects BC_HOME env var override.
func BCHome() (string, error) {
	if env := os.Getenv("BC_HOME"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".bc"), nil
}

// GlobalStateDir returns the state directory for a workspace at
// ~/.bc/workspaces/<workspace-id>/, where the ID is the 12-char sha256
// prefix produced by ComputeWorkspaceID. Matches RegistryEntry.DataDir so
// the migration and the registry agree on a single path.
// Respects BC_STATE_DIR env var override.
func GlobalStateDir(rootDir string) (string, error) {
	if env := os.Getenv("BC_STATE_DIR"); env != "" {
		return env, nil
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}

	bcHome, err := BCHome()
	if err != nil {
		return "", err
	}

	id := ComputeWorkspaceID(absRoot)
	return filepath.Join(bcHome, globalWorkspacesDirName, id), nil
}

// EnsureBCHome creates the global ~/.bc directory structure if it doesn't exist.
func EnsureBCHome() error {
	bcHome, err := BCHome()
	if err != nil {
		return err
	}
	dirs := []string{
		bcHome,
		filepath.Join(bcHome, "workspaces"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	return nil
}
