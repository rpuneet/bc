package workspace

import (
	"crypto/sha256"
	"encoding/hex"
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

// GlobalStateDir returns the state directory for a workspace at
// <MycelHome>/workspaces/<workspace-id>/, where the ID is the 12-char
// sha256 prefix produced by ComputeWorkspaceID.
//
// Respects MYCEL_STATE_DIR (env override for tests/containers).
func GlobalStateDir(rootDir string) (string, error) {
	if env := os.Getenv("MYCEL_STATE_DIR"); env != "" {
		return env, nil
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}

	home, err := MycelHome()
	if err != nil {
		return "", err
	}

	id := ComputeWorkspaceID(absRoot)
	return filepath.Join(home, globalWorkspacesDirName, id), nil
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
		filepath.Join(home, "workspaces"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	return nil
}

// workspaceIDLength is the number of hex characters in a workspace ID.
// sha256(path)[:12] gives 12 hex chars (48 bits) which is collision-safe
// for practical numbers of repos.
const workspaceIDLength = 12

// ComputeWorkspaceID returns the stable 12-char hex ID for an absolute path.
// It is the first workspaceIDLength hex chars of sha256(absPath). Empty path
// returns an empty string.
func ComputeWorkspaceID(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:])[:workspaceIDLength]
}
