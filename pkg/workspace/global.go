package workspace

// global.go — path helpers for user-global assets (~/.bc/...).
//
// Phase M8 of the multi-tenant refactor promotes templates, secrets, MCP
// trust, and costs from per-workspace `.bc/` directories to a single
// user-scoped `~/.bc/` tree. These helpers centralize the path resolution
// so tests can swap BC_HOME and production code stays consistent.

import (
	"fmt"
	"os"
	"path/filepath"
)

// Subdirectories and files relative to BCHome().
const (
	globalTemplatesDirName = "templates"
	globalSecretsFileName  = "secrets.vault"
	globalMCPFileName      = "mcps.json"
	globalCostsFileName    = "costs.db"
	globalToolsFileName    = "tools.json"
)

// GlobalTemplatesDir returns the user-global templates directory
// (~/.bc/templates/). Templates here apply across all workspaces; each
// workspace may override a template by placing a file with the same name
// under <ws>/.bc/templates/.
func GlobalTemplatesDir() (string, error) {
	return globalPath(globalTemplatesDirName)
}

// GlobalSecretsVault returns the path to the user-global secrets vault
// (~/.bc/secrets.vault). This is a SQLite database holding the user's
// encrypted key/value secrets shared across workspaces.
func GlobalSecretsVault() (string, error) {
	return globalPath(globalSecretsFileName)
}

// GlobalMCPConfig returns the path to the user-global MCP trust config
// (~/.bc/mcps.json). Servers listed here are available to every
// workspace unless the workspace overrides them locally.
func GlobalMCPConfig() (string, error) {
	return globalPath(globalMCPFileName)
}

// GlobalCostsDB returns the path to the user-global cost ledger
// (~/.bc/costs.db). Every cost record is tagged with a workspace_id so
// cross-workspace analytics work without data duplication.
func GlobalCostsDB() (string, error) {
	return globalPath(globalCostsFileName)
}

// GlobalToolsConfig returns the path to the user-global CLI tools
// registry (~/.bc/tools.json). Tools here describe machine-level
// dependencies (claude, bun, docker helpers, etc.) — there is no
// per-workspace override for this file.
func GlobalToolsConfig() (string, error) {
	return globalPath(globalToolsFileName)
}

// EnsureGlobalDir makes sure ~/.bc/ exists with 0750 permissions. It is
// idempotent and safe to call from any process path that needs to write
// a global asset. Returns the resolved BCHome path for convenience.
func EnsureGlobalDir() (string, error) {
	home, err := BCHome()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(home, 0750); err != nil {
		return "", fmt.Errorf("create bc home %s: %w", home, err)
	}
	return home, nil
}

// globalPath joins BCHome() with name. It does NOT create the parent; use
// EnsureGlobalDir when writing. Returns an error only if BCHome cannot
// be resolved (HOME unset and BC_HOME unset).
func globalPath(name string) (string, error) {
	home, err := BCHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, name), nil
}
