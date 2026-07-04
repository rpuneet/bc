// Package workspace provides workspace and project management for bc.
//
// A workspace represents a project directory whose configuration and
// agent state live under ~/.mycel/workspaces/<id>/ (preferences.json,
// state databases, agents/, logs/).
//
// # Basic Usage
//
// Find the current workspace:
//
//	ws, err := workspace.Find(".")
//	if err != nil {
//	    log.Fatal("not in a mycel workspace")
//	}
//	fmt.Println("Workspace:", ws.Name())
//
// Initialize a new workspace:
//
//	ws, err := workspace.Init("/path/to/project")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Load an existing workspace:
//
//	ws, err := workspace.Load("/path/to/project")
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

	rm, closeStore, err := initRoleManager(stateDir, absRoot, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to init role manager: %w", err)
	}
	_ = closeStore // store stays open for workspace lifetime

	// Register in global registry so Find()/IsWorkspace() work — M11+
	// workspaces live at ~/.mycel/workspaces/<id>/ with NO .bc/ marker in
	// the project directory, so the registry is the ONLY thing that
	// makes `bc <cmd>` resolve after init. If we can't persist it, init
	// must fail loudly (#3173) — silently succeeding here leaves the
	// user with a phantom workspace that every subsequent command
	// rejects as "not in a mycel workspace".
	reg, regErr := LoadRegistry()
	if regErr != nil {
		if closeStore != nil {
			_ = closeStore() //nolint:errcheck // best-effort cleanup on error path
		}
		return nil, fmt.Errorf("failed to load workspace registry (%s): %w", RegistryPath(), regErr)
	}
	reg.Register(absRoot, cfg.User.Name)
	if saveErr := reg.Save(); saveErr != nil {
		if closeStore != nil {
			_ = closeStore() //nolint:errcheck // best-effort cleanup on error path
		}
		return nil, fmt.Errorf("failed to persist workspace registry (%s): %w", RegistryPath(), saveErr)
	}

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
		return nil, fmt.Errorf("not a mycel workspace (no %s found in %s); run 'mycel up' from your repo (or add one in the web UI)",
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

	rm, closeStore, err := loadRoleManager(stateDir, absRoot, cfg)
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

// Find searches for a workspace starting from dir and going up.
// It checks the registry first, then falls back to a .bc/ directory
// marker walk (agent worktrees live under <project>/.bc/agents/).
func Find(dir string) (*Workspace, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	// Registry-first: check if any registered workspace matches this dir or a parent.
	if reg, regErr := LoadRegistry(); regErr == nil {
		current := absDir
		for {
			if entry := reg.Find(current); entry != nil {
				return Load(current)
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}

	// Self-heal (#3173): if the registry says nothing matches but a
	// state directory exists at ~/.mycel/workspaces/<id>/ where
	// <id> == ComputeWorkspaceID(walked-path), the workspace was
	// initialized but the registry file got out of sync — most often
	// because Init's Save() failed on a fresh HOME, or the user hand-
	// deleted workspaces.json. Register the entry on the fly so the
	// next `bc` call sees a healed registry.
	current := absDir
	for {
		id := ComputeWorkspaceID(current)
		stateDir, sdErr := DataDir(id)
		if sdErr == nil {
			prefs := filepath.Join(stateDir, PreferencesFileName)
			if _, statErr := os.Stat(prefs); statErr == nil {
				// Preserve the configured user name when re-registering so
				// the self-heal doesn't clobber it with an empty string.
				name := ""
				if cfg, cfgErr := LoadConfig(prefs); cfgErr == nil && cfg != nil {
					name = cfg.User.Name
				}
				if reg, regErr := LoadRegistry(); regErr == nil {
					reg.Register(current, name)
					if saveErr := reg.Save(); saveErr != nil {
						log.Warn("workspace registry: self-heal save failed", "path", current, "error", saveErr)
					} else {
						log.Info("workspace registry: self-healed on find", "id", id, "path", current)
					}
				}
				return Load(current)
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// Fallback: walk up looking for a .bc/ directory marker (runtime
	// layout — agent worktrees live under <project>/.bc/agents/).
	current = absDir
	for {
		stateDir := filepath.Join(current, ".bc")
		if _, err := os.Stat(stateDir); err == nil {
			return Load(current)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return nil, fmt.Errorf("no workspace found (searched from %s to root)", absDir)
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

// IsWorkspace checks if a directory is a workspace.
// Checks the .bc/ runtime marker and the global state dir
// (~/.mycel/workspaces/<id>/).
func IsWorkspace(dir string) bool {
	// Check .bc/ runtime marker (agent worktree layout)
	stateDir := filepath.Join(dir, ".bc")
	if _, err := os.Stat(stateDir); err == nil {
		return true
	}
	// Check global state dir exists on disk
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	if globalDir, gErr := GlobalStateDir(absDir); gErr == nil {
		if _, statErr := os.Stat(globalDir); statErr == nil {
			return true
		}
	}
	return false
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

// openRoleStore creates a RoleStore for the workspace. When the
// workspace is configured for TimescaleDB (or DATABASE_URL forces it),
// the role store shares the per-workspace registry connection; in
// SQLite mode it keeps its own file at <stateDir>/bc.db.
func openRoleStore(stateDir, rootDir string, cfg *Config) (*RoleStore, error) {
	settings := cfg.DBStorageSettings()
	wantsTimescale := db.IsPostgresEnabled() ||
		(settings != nil && settings.Default == "timescale")
	if wantsTimescale {
		wsDB, driver, err := db.ForWorkspace(rootDir, settings)
		if err != nil {
			return nil, fmt.Errorf("role store: open workspace db: %w", err)
		}
		if driver == "timescale" {
			return NewRoleStoreFromDB(wsDB.DB, "timescale")
		}
		// Timescale unreachable — the registry fell back to SQLite;
		// keep roles in the local role db below as before.
	}

	dbPath := filepath.Join(stateDir, "bc.db")
	return NewRoleStore(dbPath)
}

// initRoleManager creates a role manager with SQL store for workspace Init.
// It creates the store, migrates defaults, and migrates any legacy filesystem
// roles. Returns the manager plus a close function for the store.
func initRoleManager(stateDir, rootDir string, cfg *Config) (*RoleManager, func() error, error) {
	store, err := openRoleStore(stateDir, rootDir, cfg)
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
func loadRoleManager(stateDir, rootDir string, cfg *Config) (*RoleManager, func() error, error) {
	store, err := openRoleStore(stateDir, rootDir, cfg)
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
