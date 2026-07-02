package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/rpuneet/mycel/pkg/log"
)

// The state directory used to live under ~/.bc for historical reasons
// (the project was called bc before the mycel rename). Every new install
// now uses ~/.mycel. Callers can invoke MigrateLegacyHome() once at
// startup to rename an old ~/.bc/ tree into ~/.mycel/.
const (
	legacyHomeDirName = ".bc"
	homeDirName       = ".mycel"
)

// warnBCHomeOnce / warnBCStateDirOnce keep the "deprecated env var"
// WARN to at most one line per process. Both MycelHome() and
// GlobalStateDir() are called from bcd request handlers, so without the
// guard the log would be flooded on every request when a deployment
// still exports the legacy env var.
var (
	warnBCHomeOnce     sync.Once
	warnBCStateDirOnce sync.Once
)

// MycelHome returns the global mycel home directory.
//
// Resolution order:
//  1. MYCEL_HOME env var (canonical, post-rename).
//  2. BC_HOME env var (deprecated but honored so bc-era scripts and
//     tests keep working; a one-line WARN is logged).
//  3. ~/.mycel/ when it exists on disk.
//  4. ~/.bc/ when it exists but ~/.mycel/ doesn't — a bc-era install
//     that hasn't run the migration yet.
//  5. ~/.mycel/ as the default when nothing exists yet.
//
// This function never mutates the filesystem. Use MigrateLegacyHome()
// at CLI startup to rename an existing ~/.bc/ to ~/.mycel/.
func MycelHome() (string, error) {
	if env := os.Getenv("MYCEL_HOME"); env != "" {
		return env, nil
	}
	if env := os.Getenv("BC_HOME"); env != "" {
		warnBCHomeOnce.Do(func() {
			log.Warn("BC_HOME is deprecated; use MYCEL_HOME", "value", env)
		})
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	target := filepath.Join(home, homeDirName)
	if _, statErr := os.Stat(target); statErr == nil {
		return target, nil
	}

	legacy := filepath.Join(home, legacyHomeDirName)
	if _, statErr := os.Stat(legacy); statErr == nil {
		return legacy, nil
	}

	return target, nil
}

// BCHome is the pre-rename spelling of MycelHome. Retained so existing
// call sites keep compiling; new code should use MycelHome directly.
//
// Deprecated: use MycelHome.
func BCHome() (string, error) {
	return MycelHome()
}

// MigrateLegacyHome renames ~/.bc/ to ~/.mycel/ when the legacy tree
// exists and the canonical one doesn't. Idempotent, safe to call
// multiple times, safe to call when nothing needs migrating. Returns
// (migrated, err) — migrated is true only when a rename actually
// happened, so callers can print a one-time notice.
//
// Callers must have resolved the "real" user HOME (i.e. not overridden
// via MYCEL_HOME/BC_HOME) before calling this — the migration is a
// no-op when the resolved home already looks like something other
// than "$HOME/.bc" or "$HOME/.mycel".
func MigrateLegacyHome() (bool, error) {
	if os.Getenv("MYCEL_HOME") != "" || os.Getenv("BC_HOME") != "" {
		return false, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	target := filepath.Join(home, homeDirName)
	legacy := filepath.Join(home, legacyHomeDirName)

	if _, statErr := os.Stat(target); statErr == nil {
		return false, nil // canonical already exists
	}
	if _, statErr := os.Stat(legacy); statErr != nil {
		return false, nil // nothing to migrate
	}
	if renameErr := os.Rename(legacy, target); renameErr != nil {
		return false, fmt.Errorf("rename %s → %s: %w", legacy, target, renameErr)
	}
	log.Info("migrated legacy ~/.bc to ~/.mycel", "from", legacy, "to", target)
	return true, nil
}

// GlobalStateDir returns the state directory for a workspace at
// <MycelHome>/workspaces/<workspace-id>/, where the ID is the 12-char
// sha256 prefix produced by ComputeWorkspaceID. Matches
// RegistryEntry.DataDir so migration and registry agree on a single
// path.
//
// Respects MYCEL_STATE_DIR (canonical) and BC_STATE_DIR (deprecated).
func GlobalStateDir(rootDir string) (string, error) {
	if env := os.Getenv("MYCEL_STATE_DIR"); env != "" {
		return env, nil
	}
	if env := os.Getenv("BC_STATE_DIR"); env != "" {
		warnBCStateDirOnce.Do(func() {
			log.Warn("BC_STATE_DIR is deprecated; use MYCEL_STATE_DIR", "value", env)
		})
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

// EnsureBCHome is the pre-rename spelling of EnsureMycelHome.
//
// Deprecated: use EnsureMycelHome.
func EnsureBCHome() error {
	return EnsureMycelHome()
}
