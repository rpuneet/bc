// Package workspace manages mycel-adopted repos and their state.
//
// A Workspace represents a git repo whose configuration and agent
// state live under ~/.mycel/workspaces/<id>/ (preferences.json, state
// databases, agents/, logs/). The repo itself stays pristine.
//
// # Basic Usage
//
// Find the enclosing adopted repo:
//
//	ws, err := workspace.Find(".")
//	if err != nil {
//	    log.Fatal("not in a mycel-adopted repo")
//	}
//	fmt.Println("Repo:", ws.Name())
//
// Adopt a new repo:
//
//	ws, err := workspace.Init("/path/to/repo")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Load an already-adopted repo:
//
//	ws, err := workspace.Load("/path/to/repo")
//	if err != nil {
//	    log.Fatal(err)
//	}
package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/log"
)

// Workspace represents an active workspace.
//
// The Workspace maintains two independent directories:
//
//   - RootDir:  the project (a pristine git repo bc points at but never
//     writes runtime state into).
//   - DataDir:  the per-workspace runtime directory
//     (~/.mycel/workspaces/<id>/) containing preferences.json, state
//     databases, agents/, logs/, etc.
type Workspace struct {
	Config      *Config      // JSON config
	RoleManager *RoleManager // Role file manager
	RootDir     string       // Project root directory (pristine git repo)
	DataDir     string       // Runtime state dir (~/.mycel/workspaces/<id>/)
	stateDir    string       // Resolved state dir (normally == DataDir)
}

// Init initializes a new workspace. State is stored under ~/.mycel/workspaces/<id>/.
func Init(rootDir string) (*Workspace, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	// Verify this is a git repository — bc requires git for agent worktrees.
	gitDir := filepath.Join(absRoot, ".git")
	if _, statErr := os.Stat(gitDir); os.IsNotExist(statErr) {
		return nil, fmt.Errorf("not a git repository: %s\nRun 'git init' first, then 'mycel up'", absRoot)
	}

	if homeErr := EnsureMycelHome(); homeErr != nil {
		return nil, homeErr
	}

	stateDir, err := GlobalStateDir(absRoot)
	if err != nil {
		return nil, fmt.Errorf("cannot determine state directory: %w", err)
	}

	dirs := []string{
		stateDir,
		filepath.Join(stateDir, "agents"),
		filepath.Join(stateDir, "roles"),
		filepath.Join(stateDir, "channels"),
		filepath.Join(stateDir, "prompts"),
	}
	for _, dir := range dirs {
		if err = os.MkdirAll(dir, 0750); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	if cpErr := copyDefaultPrompts(absRoot, stateDir); cpErr != nil {
		log.Warn("failed to copy default prompts", "error", cpErr)
	}

	cfg := DefaultConfig()

	configPath := filepath.Join(stateDir, PreferencesFileName)
	if saveErr := cfg.Save(configPath); saveErr != nil {
		return nil, fmt.Errorf("failed to save config: %w", saveErr)
	}

	rm, closeStore, err := initRoleManager(stateDir, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to init role manager: %w", err)
	}
	_ = closeStore // store stays open for workspace lifetime

	// No registry: the state dir at ~/.mycel/workspaces/<id>/ (keyed by
	// ComputeWorkspaceID of the repo path) is the only marker. Find()
	// re-derives it by walking up from cwd and hashing each candidate.

	return &Workspace{
		RootDir:     absRoot,
		DataDir:     stateDir,
		stateDir:    stateDir,
		Config:      &cfg,
		RoleManager: rm,
	}, nil
}

// Load loads a workspace from a directory.
func Load(rootDir string) (*Workspace, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	// Verify this is a git repository.
	gitDir := filepath.Join(absRoot, ".git")
	if _, statErr := os.Stat(gitDir); os.IsNotExist(statErr) {
		return nil, fmt.Errorf("not a git repository: %s", absRoot)
	}

	// State lives in the global state dir (~/.mycel/workspaces/<id>/).
	stateDir, stateDirErr := GlobalStateDir(absRoot)
	if stateDirErr != nil {
		return nil, fmt.Errorf("cannot determine state directory: %w", stateDirErr)
	}

	// Config is <stateDir>/preferences.json — the only config file bc reads.
	jsonPath := filepath.Join(stateDir, PreferencesFileName)
	if _, statErr := os.Stat(jsonPath); statErr != nil {
		return nil, fmt.Errorf("not a mycel-adopted repo (no %s found in %s); run 'mycel up' from your repo (or add one in the web UI)",
			PreferencesFileName, stateDir)
	}

	cfg, loadErr := LoadConfig(jsonPath)
	if loadErr != nil {
		return nil, fmt.Errorf("failed to load workspace config: %w", loadErr)
	}
	log.Info("config: "+PreferencesFileName, "path", jsonPath)

	cfg.FillDefaults()

	if valErr := cfg.Validate(); valErr != nil {
		return nil, fmt.Errorf("invalid %s: %w", PreferencesFileName, valErr)
	}

	rm, closeStore, err := loadRoleManager(stateDir, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load roles: %w", err)
	}
	_ = closeStore // store stays open for workspace lifetime

	return &Workspace{
		RootDir:     absRoot,
		DataDir:     stateDir,
		stateDir:    stateDir,
		Config:      cfg,
		RoleManager: rm,
	}, nil
}

// Find searches for a mycel-adopted repo starting from dir and going
// up. A directory qualifies when its global state dir
// (~/.mycel/workspaces/<ComputeWorkspaceID(dir)>/preferences.json)
// exists — i.e. the repo was adopted by `mycel up`. There is no
// registry and no in-repo marker: the walk re-derives the state dir by
// hashing each candidate path.
func Find(dir string) (*Workspace, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	current := absDir
	for {
		// Global state dir: the repo was adopted by `mycel up`.
		id := ComputeWorkspaceID(current)
		if stateDir, sdErr := DataDir(id); sdErr == nil {
			prefs := filepath.Join(stateDir, PreferencesFileName)
			if _, statErr := os.Stat(prefs); statErr == nil {
				return Load(current)
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil, fmt.Errorf("no mycel-adopted repo found (searched from %s to root)", absDir)
		}
		current = parent
	}
}

// Save saves the workspace configuration to preferences.json.
func (w *Workspace) Save() error {
	configPath := filepath.Join(w.StateDir(), PreferencesFileName)
	return w.Config.Save(configPath)
}

// StateDir returns the resolved state directory path
// (~/.mycel/workspaces/<id>/).
func (w *Workspace) StateDir() string {
	if w.stateDir != "" {
		return w.stateDir
	}
	if w.DataDir != "" {
		return w.DataDir
	}
	return filepath.Join(w.RootDir, ".bc")
}

// SettingsFile returns the absolute path of the workspace preferences
// file: <StateDir>/preferences.json. The path is returned whether or not
// the file exists yet, so callers may safely write to it.
func (w *Workspace) SettingsFile() string {
	return filepath.Join(w.StateDir(), PreferencesFileName)
}

// AgentsDir returns the agents state directory.
func (w *Workspace) AgentsDir() string {
	return filepath.Join(w.StateDir(), "agents")
}

// LogsDir returns the logs directory.
func (w *Workspace) LogsDir() string {
	if w.Config != nil && w.Config.Logs.Path != "" {
		return filepath.Join(w.RootDir, w.Config.Logs.Path)
	}
	return filepath.Join(w.StateDir(), "logs")
}

// RolesDir returns the roles directory path.
func (w *Workspace) RolesDir() string {
	return filepath.Join(w.StateDir(), "roles")
}

// ChannelsDir returns the channels directory path.
func (w *Workspace) ChannelsDir() string {
	return filepath.Join(w.StateDir(), "channels")
}

// EnsureDirs creates all required directories.
func (w *Workspace) EnsureDirs() error {
	dirs := []string{
		w.StateDir(),
		w.AgentsDir(),
		w.LogsDir(),
		w.RolesDir(),
		w.ChannelsDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}

	return nil
}

// GetRole returns a role by name, loading it if necessary.
func (w *Workspace) GetRole(name string) (*Role, error) {
	if w.RoleManager == nil {
		return nil, fmt.Errorf("role manager not initialized")
	}

	if role, ok := w.RoleManager.GetRole(name); ok {
		return role, nil
	}

	return w.RoleManager.LoadRole(name)
}

// GetRolePrompt returns the prompt content for a role.
func (w *Workspace) GetRolePrompt(name string) string {
	role, err := w.GetRole(name)
	if err != nil {
		return ""
	}
	return role.Prompt
}

// openRoleStore creates a RoleStore on the single global database
// (<MycelHome>/mycel.db, or the configured TimescaleDB). Roles from
// every workspace share the one roles table; the connection is
// borrowed from pkg/db and Close on the store is a no-op.
func openRoleStore(cfg *Config) (*RoleStore, error) {
	wsDB, driver, err := db.Global(cfg.DBStorageSettings())
	if err != nil {
		return nil, fmt.Errorf("role store: open global db: %w", err)
	}
	return NewRoleStoreFromDB(wsDB.DB, driver)
}

// NewGlobalRoleManager creates a role manager backed by the single global
// database, where roles for every workspace live. stateDir is only used to
// migrate any legacy filesystem roles under <stateDir>/roles into the store.
// Package-level helpers that resolve roles without a *Workspace must use
// this constructor — building a manager on a per-workspace bc.db reads the
// wrong store and reports every role as missing.
func NewGlobalRoleManager(stateDir string) (*RoleManager, error) {
	store, err := openRoleStore(nil)
	if err != nil {
		return nil, err
	}
	_, _ = store.MigrateFromFiles(filepath.Join(stateDir, "roles")) //nolint:errcheck // best-effort
	return NewRoleManagerWithStore(stateDir, store), nil
}

// initRoleManager creates a role manager with SQL store for workspace Init.
// It creates the store, migrates defaults, and migrates any legacy filesystem
// roles. Returns the manager plus a close function for the store.
func initRoleManager(stateDir string, cfg *Config) (*RoleManager, func() error, error) {
	store, err := openRoleStore(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("open role store: %w", err)
	}

	// Migrate defaults into store
	if migrateErr := store.MigrateDefaults(); migrateErr != nil {
		log.Warn("failed to migrate default roles to store", "error", migrateErr)
	}

	// Also migrate any existing filesystem files
	rolesDir := filepath.Join(stateDir, "roles")
	if _, migrateErr := store.MigrateFromFiles(rolesDir); migrateErr != nil {
		log.Warn("failed to migrate role files to store", "error", migrateErr)
	}

	rm := NewRoleManagerWithStore(stateDir, store)

	// Ensure base and root roles exist in the store
	if _, ensureErr := rm.EnsureDefaultRoot(); ensureErr != nil {
		log.Warn("failed to ensure default root role", "error", ensureErr)
	}
	if _, ensureErr := rm.EnsureDefaultRoles(); ensureErr != nil {
		log.Warn("failed to ensure default roles", "error", ensureErr)
	}

	return rm, store.Close, nil
}

// loadRoleManager creates a role manager with SQL store for workspace Load.
// It opens the store, migrates any new filesystem files, and loads all roles
// into the cache.
func loadRoleManager(stateDir string, cfg *Config) (*RoleManager, func() error, error) {
	store, err := openRoleStore(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("open role store: %w", err)
	}

	// Seed defaults if store is empty (e.g. fresh Postgres)
	if migrateErr := store.MigrateDefaults(); migrateErr != nil {
		log.Warn("failed to seed default roles", "error", migrateErr)
	}

	// Migrate any filesystem roles that aren't in the store yet
	rolesDir := filepath.Join(stateDir, "roles")
	if _, migrateErr := store.MigrateFromFiles(rolesDir); migrateErr != nil {
		log.Warn("failed to migrate role files to store", "error", migrateErr)
	}

	rm := NewRoleManagerWithStore(stateDir, store)
	if _, loadErr := rm.LoadAllRoles(); loadErr != nil {
		_ = store.Close()
		return nil, nil, loadErr
	}

	return rm, store.Close, nil
}

// DefaultProvider returns the default provider name for this workspace.
func (w *Workspace) DefaultProvider() string {
	if w.Config != nil {
		return w.Config.GetDefaultProvider()
	}
	return "claude"
}

// DefaultProviderCommand returns the command for the default provider.
func (w *Workspace) DefaultProviderCommand() string {
	if w.Config != nil {
		if p := w.Config.GetProvider(w.Config.GetDefaultProvider()); p != nil {
			return p.Command
		}
	}
	return ""
}

// Name returns the workspace name (derived from directory).
func (w *Workspace) Name() string {
	return filepath.Base(w.RootDir)
}

// copyDefaultPrompts copies default prompt files from root prompts/ to .bc/prompts/.
func copyDefaultPrompts(rootDir, stateDir string) error {
	sourceDir := filepath.Join(rootDir, "prompts")
	destDir := filepath.Join(stateDir, "prompts")

	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return fmt.Errorf("failed to read prompts directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) != ".md" {
			continue
		}

		sourcePath := filepath.Join(sourceDir, name)
		destPath := filepath.Join(destDir, name)

		if _, err := os.Stat(destPath); err == nil {
			continue
		}

		if err := copyFile(sourcePath, destPath); err != nil {
			return fmt.Errorf("failed to copy %s: %w", name, err)
		}
	}

	return nil
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	// #nosec G304 - src path is from internal prompts directory
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()

	// #nosec G304 - dst path is in workspace .bc/prompts directory
	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = destination.Close() }()

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}

	if info, err := os.Stat(src); err == nil {
		_ = os.Chmod(dst, info.Mode())
	}

	return nil
}
