package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

const homeDirName = ".mycel"

// MycelHome returns the global mycel home directory: the MYCEL_HOME
// env var when set (tests, containers), otherwise ~/.mycel.
// Never mutates the filesystem.
func MycelHome() (string, error) {
	if env := os.Getenv("MYCEL_HOME"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, homeDirName), nil
}

// EnsureMycelHome creates the global mycel home directory structure if
// it doesn't already exist. Idempotent.
func EnsureMycelHome() error {
	home, err := MycelHome()
	if err != nil {
		return err
	}
	dirs := []string{
		home,
		filepath.Join(home, globalAgentsDirName),
		filepath.Join(home, globalAppsDirName),
		filepath.Join(home, globalTemplatesDirName),
		filepath.Join(home, globalLogsDirName),
		filepath.Join(home, globalRunDirName),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	return nil
}
