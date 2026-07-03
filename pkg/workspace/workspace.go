// Package workspace provides workspace and project management for bc.
//
// A workspace represents a project directory containing bc configuration
// and agent state in .bc/settings.json.
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
// After M11 the Workspace maintains two independent directories:
//
//   - RootDir:  the project (a pristine git repo bc points at but never
//     writes runtime state into).
//   - DataDir:  the per-workspace runtime directory
//     (~/.bc/workspaces/<id>/) containing preferences.json, state.db,
//     cron.db, agents/, logs/, etc.
//
// StateDir() returns DataDir for new workspaces; for legacy workspaces
// that still keep state inside <RootDir>/.bc/ (pre-M11 migration), it
// returns that path instead so in-flight code keeps working until the
// migration runs.
type Workspace struct {
	Config      *Config      // JSON config
	RoleManager *RoleManager // Role file manager
	RootDir     string       // Project root directory (pristine git repo)
	DataDir     string       // Runtime state dir (~/.bc/workspaces/<id>/); set for M11+ layouts
	stateDir    string       // Resolved state dir (DataDir for M11+, legacy .bc/ for older)
}

// Init initializes a new workspace. State is stored under ~/.bc/workspaces/<id>/.
func Init(rootDir string) (*Workspace, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	// Verify this is a git repository — bc requires git for agent worktrees.
	gitDir := filepath.Join(absRoot, ".git")
	if _, statErr := os.Stat(gitDir); os.IsNotExist(statErr) {
		return nil, fmt.Errorf("not a git repository: %s\nRun 'git init' first, then 'mycel init'", absRoot)
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

	rm, closeStore, err := initRoleManager(stateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to init role manager: %w", err)
	}
	_ = closeStore // store stays open for workspace lifetime

	// Register in global registry so Find()/IsWorkspace() work — M11+
	// workspaces live at ~/.bc/workspaces/<id>/ with NO .bc/ marker in
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

	// Try global state dir first (~/.bc/workspaces/<id>/)
	stateDir, stateDirErr := GlobalStateDir(absRoot)
	if stateDirErr != nil {
		stateDir = filepath.Join(absRoot, ".bc") // fallback to legacy
	}

	// Load config — check global dir first (preferences.json, then
	// settings.json), then legacy .bc/ as a fallback.
	jsonPath := firstExisting(stateDir, PreferencesFileName, LegacySettingsFileName)
	if jsonPath == "" {
		legacyDir := filepath.Join(absRoot, ".bc")
		legacyPath := firstExisting(legacyDir, PreferencesFileName, LegacySettingsFileName)
		if legacyPath != "" {
			stateDir = legacyDir
			jsonPath = legacyPath
		} else {
			return nil, fmt.Errorf("not a mycel workspace (no %s or %s found in %s or %s)",
				PreferencesFileName, LegacySettingsFileName, stateDir, legacyDir)
		}
	}

	cfg, loadErr := LoadConfig(jsonPath)
	if loadErr != nil {
		return nil, fmt.Errorf("failed to load workspace config: %w", loadErr)
	}

	// #3239: preferences.json is the active config, but a human may have
	// edited a legacy settings.json (state dir or project .bc/) since it
	// was last written. When such a file is strictly newer (mtime),
	// overlay it section-by-section so the edit takes effect instead of
	// being silently shadowed.
	overlayPath := ""
	if filepath.Base(jsonPath) == PreferencesFileName {
		overlayPath = applyNewerSettingsOverlay(cfg, jsonPath, stateDir, absRoot)
	}
	if overlayPath != "" {
		log.Info("config: "+PreferencesFileName+" (+overlay from "+filepath.Base(overlayPath)+")",
			"path", jsonPath, "overlay", overlayPath)
	} else {
		log.Info("config: "+filepath.Base(jsonPath), "path", jsonPath)
	}

	cfg.FillDefaults()

	if valErr := cfg.Validate(); valErr != nil {
		return nil, fmt.Errorf("invalid settings.json: %w", valErr)
	}

	// Persist the merged result so preferences.json reflects the overlay
	// and becomes the newest file — subsequent loads skip the overlay.
	// This is the only write Load performs: plain reads never save
	// (the old save-on-read promotion is gone, #3239).
	if overlayPath != "" {
		if saveErr := cfg.Save(jsonPath); saveErr != nil {
			log.Warn("failed to persist merged config", "path", jsonPath, "error", saveErr)
		}
	}

	rm, closeStore, err := loadRoleManager(stateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load roles: %w", err)
	}
	_ = closeStore // store stays open for workspace lifetime

	// DataDir points at the canonical global runtime dir. When stateDir
	// is still the legacy <project>/.bc/ path (pre-migration), leave
	// DataDir at the computed global location so callers can target the
	// new tree even before the migration runs.
	dataDir := stateDir
	if globalDir, gErr := GlobalStateDir(absRoot); gErr == nil {
		dataDir = globalDir
	}

	return &Workspace{
		RootDir:     absRoot,
		DataDir:     dataDir,
		stateDir:    stateDir,
		Config:      cfg,
		RoleManager: rm,
	}, nil
}

// Find searches for a workspace starting from dir and going up.
// It checks the registry first (for .bc/-free workspaces), then
// falls back to the legacy .bc/ directory walk.
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
	// state directory exists at ~/.bc/workspaces/<id>/ where
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
			if prefs := firstExisting(stateDir, PreferencesFileName, LegacySettingsFileName); prefs != "" {
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

	// Legacy fallback: walk up looking for .bc/ directory marker.
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
// A legacy settings.json on disk is left alone for the user to audit.
func (w *Workspace) Save() error {
	configPath := filepath.Join(w.StateDir(), PreferencesFileName)
	return w.Config.Save(configPath)
}

// firstExisting returns the first existing file path among the given
// names under dir, or "" when none exist. Order determines priority.
func firstExisting(dir string, names ...string) string {
	for _, n := range names {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// StateDir returns the resolved state directory path.
// Returns DataDir for M11+ layouts or the legacy <RootDir>/.bc/ path when
// a workspace has not yet been migrated.
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
// file. M11c renames the on-disk filename from settings.json to
// preferences.json; this accessor is the canonical way to find whichever
// file actually lives on disk.
//
// Lookup order: <StateDir>/preferences.json (M11c+), then
// <StateDir>/settings.json (legacy). Returns the preferences.json path
// when neither exists so callers may safely write to it.
func (w *Workspace) SettingsFile() string {
	prefs := filepath.Join(w.StateDir(), PreferencesFileName)
	if _, err := os.Stat(prefs); err == nil {
		return prefs
	}
	legacy := filepath.Join(w.StateDir(), LegacySettingsFileName)
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return prefs
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
// Checks legacy .bc/ directory and global state dir (~/.bc/workspaces/<id>/).
func IsWorkspace(dir string) bool {
	// Check legacy .bc/ marker
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

// openRoleStore creates a RoleStore using the shared database connection.
// Uses the shared driver type to determine the backend (timescale or sqlite).
func openRoleStore(stateDir string) (*RoleStore, error) {
	driver := db.SharedDriver()
	if driver == "timescale" {
		shared := db.Shared()
		if shared == nil {
			return nil, fmt.Errorf("role store: shared timescale connection is nil")
		}
		return NewRoleStoreFromDB(shared, "timescale")
	}

	dbPath := filepath.Join(stateDir, "bc.db")
	return NewRoleStore(dbPath)
}

// initRoleManager creates a role manager with SQL store for workspace Init.
// It creates the store, migrates defaults, and migrates any legacy filesystem
// roles. Returns the manager plus a close function for the store.
func initRoleManager(stateDir string) (*RoleManager, func() error, error) {
	store, err := openRoleStore(stateDir)
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
func loadRoleManager(stateDir string) (*RoleManager, func() error, error) {
	store, err := openRoleStore(stateDir)
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
