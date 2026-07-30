package workspace

// global.go — path helpers for user-global assets (~/.mycel/...).
//
// Phase M8 of the multi-tenant refactor promotes templates, secrets, MCP
// trust, and costs from per-workspace `.bc/` directories to a single
// user-scoped `~/.mycel/` tree. These helpers centralize the path resolution
// so tests can swap MYCEL_HOME and production code stays consistent.

import (
	"fmt"
	"os"
	"path/filepath"
)

// Subdirectories and files relative to MycelHome().
const (
	globalTemplatesDirName  = "templates"
	globalSecretsFileName   = "secrets.vault"
	globalMCPFileName       = "mcps.json"
	globalCostsFileName     = "costs.db"
	globalToolsFileName     = "tools.json"
	globalWorkspacesDirName = "workspaces"
	globalDaemonPidName     = "daemon.pid"
	globalDaemonLogName     = "daemon.log"
	globalDaemonAddrName    = "daemon.addr"
)

// DataDir returns the per-workspace runtime directory for a given
// workspace ID.
//
// Returns ~/.mycel/workspaces/<id>/ (respecting MYCEL_HOME). Pass the ID from
// ComputeWorkspaceID(absRootDir).
//
// This path holds every piece of runtime state for the workspace:
// preferences.json, state.db, agents/, logs/ — nothing lives
// under the project directory anymore.
func DataDir(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("workspace id is empty")
	}
	home, err := MycelHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, globalWorkspacesDirName, id), nil
}

// GlobalTemplatesDir returns the user-global templates directory
// (~/.mycel/templates/). Templates here apply across all workspaces; each
// workspace may override a template by placing a file with the same name
// under its state dir's templates/ directory
// (~/.mycel/workspaces/<id>/templates/).
func GlobalTemplatesDir() (string, error) {
	return globalPath(globalTemplatesDirName)
}

// GlobalSecretsVault returns the path to the user-global secrets vault
// (~/.mycel/secrets.vault). This is a SQLite database holding the user's
// encrypted key/value secrets shared across workspaces.
func GlobalSecretsVault() (string, error) {
	return globalPath(globalSecretsFileName)
}

// GlobalMCPConfig returns the path to the user-global MCP trust config
// (~/.mycel/mcps.json). Servers listed here are available to every
// workspace unless the workspace overrides them locally.
func GlobalMCPConfig() (string, error) {
	return globalPath(globalMCPFileName)
}

// GlobalCostsDB returns the path to the user-global cost ledger
// (~/.mycel/costs.db). Every cost record is tagged with a repo path so
// cross-workspace analytics work without data duplication.
func GlobalCostsDB() (string, error) {
	return globalPath(globalCostsFileName)
}

// GlobalToolsConfig returns the path to the user-global CLI tools
// registry (~/.mycel/tools.json). Tools here describe machine-level
// dependencies (claude, bun, docker helpers, etc.) — there is no
// per-workspace override for this file.
func GlobalToolsConfig() (string, error) {
	return globalPath(globalToolsFileName)
}

// DaemonPidPath returns the path to the user-global bcd pid file
// (~/.mycel/daemon.pid). The bcd daemon is user-scoped — a single process
// serves every workspace — so its pid lives outside any per-workspace
// directory.
func DaemonPidPath() (string, error) {
	return globalPath(globalDaemonPidName)
}

// DaemonLogPath returns the path to the user-global bcd log file
// (~/.mycel/daemon.log). Same rationale as DaemonPidPath: one bcd, one log.
func DaemonLogPath() (string, error) {
	return globalPath(globalDaemonLogName)
}

// DaemonAddrPath returns the path to the user-global bcd address file
// (~/.mycel/daemon.addr). `mycel up` writes the currently-listening address
// (scheme + host:port, e.g. "http://127.0.0.1:8080") so the CLI and
// agents can locate the daemon without requiring MYCEL_DAEMON_ADDR to be
// set when the daemon runs on a non-default port.
func DaemonAddrPath() (string, error) {
	return globalPath(globalDaemonAddrName)
}

// EnsureGlobalDir makes sure ~/.mycel/ exists with 0750 permissions. It is
// idempotent and safe to call from any process path that needs to write
// a global asset. Returns the resolved MycelHome path for convenience.
func EnsureGlobalDir() (string, error) {
	home, err := MycelHome()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(home, 0750); err != nil {
		return "", fmt.Errorf("create bc home %s: %w", home, err)
	}
	return home, nil
}

// globalPath joins MycelHome() with name. It does NOT create the parent; use
// EnsureGlobalDir when writing. Returns an error only if MycelHome cannot
// be resolved (HOME unset and MYCEL_HOME unset).
func globalPath(name string) (string, error) {
	home, err := MycelHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, name), nil
}
