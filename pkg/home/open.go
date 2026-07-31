// Package home manages the ~/.mycel home: the global mycel config and
// the repos agents work against.
//
// mycel keeps ONE config (~/.mycel/prefs.json) and ONE database
// (~/.mycel/mycel.db). A Home pairs that global config with an
// optional anchor repo (a git repo new agents default to). Repos stay
// pristine — all runtime state lives under ~/.mycel.
//
// # Basic Usage
//
// Bootstrap-or-load (idempotent — what `mycel up` does):
//
//	h, err := home.Open("/path/to/repo") // or Open("") for no anchor repo
//
// Load strictly (fails when mycel was never set up):
//
//	h, err := home.Load("/path/to/repo")
//
// Find the enclosing git repo and load:
//
//	h, err := home.Find(".")
package home

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/log"
)

// Home pairs the global mycel config with an optional anchor repo.
//
//   - RootDir: the anchor repo (a pristine git repo new agents default
//     to). May be empty when the daemon boots outside any repo.
//   - All runtime state lives under MycelHome() (~/.mycel/).
type Home struct {
	Config      *Config      // the one global config (~/.mycel/prefs.json)
	RoleManager *RoleManager // Role manager (DB-backed)
	RootDir     string       // Anchor repo root ("" = none)

	// ConfigMu guards Config for handlers that read and mutate it
	// concurrently (apps CRUD + catalog + settings PATCH). Readers take
	// RLock, writers Lock; Go maps hard-crash on unsynchronized access.
	ConfigMu sync.RWMutex
}

// Open bootstraps-or-loads the global mycel state. Idempotent: creates
// ~/.mycel (and prefs.json with defaults) on first run, loads the
// existing config afterwards. rootDir may be empty — the daemon then
// runs without an anchor repo and agents must name their own repo.
// A non-empty rootDir must be a git repository.
func Open(rootDir string) (*Home, error) {
	absRoot := ""
	if rootDir != "" {
		var err error
		absRoot, err = filepath.Abs(rootDir)
		if err != nil {
			return nil, err
		}
		if _, statErr := os.Stat(filepath.Join(absRoot, ".git")); os.IsNotExist(statErr) {
			return nil, fmt.Errorf("not a git repository: %s\nRun 'git init' first, then 'mycel up'", absRoot)
		}
	}

	if err := EnsureMycelHome(); err != nil {
		return nil, err
	}

	prefsPath, err := PrefsPath()
	if err != nil {
		return nil, err
	}

	var cfg *Config
	if _, statErr := os.Stat(prefsPath); statErr == nil {
		cfg, err = LoadConfig(prefsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	} else {
		defaults := DefaultConfig()
		cfg = &defaults
		if saveErr := cfg.Save(prefsPath); saveErr != nil {
			return nil, fmt.Errorf("failed to save config: %w", saveErr)
		}
	}

	cfg.FillDefaults()
	if valErr := cfg.Validate(); valErr != nil {
		return nil, fmt.Errorf("invalid %s: %w", PrefsFileName, valErr)
	}

	home, err := MycelHome()
	if err != nil {
		return nil, err
	}
	rm, closeStore, err := initRoleManager(home, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to init role manager: %w", err)
	}
	_ = closeStore // store stays open for the process lifetime

	return &Home{
		RootDir:     absRoot,
		Config:      cfg,
		RoleManager: rm,
	}, nil
}

// Load loads the global mycel state strictly: ~/.mycel/prefs.json must
// already exist (i.e. `mycel up` ran at least once). A non-empty
// rootDir must be a git repository and becomes the anchor repo.
func Load(rootDir string) (*Home, error) {
	absRoot := ""
	if rootDir != "" {
		var err error
		absRoot, err = filepath.Abs(rootDir)
		if err != nil {
			return nil, err
		}
		if _, statErr := os.Stat(filepath.Join(absRoot, ".git")); os.IsNotExist(statErr) {
			return nil, fmt.Errorf("not a git repository: %s", absRoot)
		}
	}

	prefsPath, err := PrefsPath()
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(prefsPath); statErr != nil {
		return nil, fmt.Errorf("mycel is not set up (no %s); run 'mycel up' first", prefsPath)
	}

	cfg, loadErr := LoadConfig(prefsPath)
	if loadErr != nil {
		return nil, fmt.Errorf("failed to load config: %w", loadErr)
	}
	log.Info("config: "+PrefsFileName, "path", prefsPath)

	cfg.FillDefaults()

	if valErr := cfg.Validate(); valErr != nil {
		return nil, fmt.Errorf("invalid %s: %w", PrefsFileName, valErr)
	}

	home, err := MycelHome()
	if err != nil {
		return nil, err
	}
	rm, closeStore, err := loadRoleManager(home, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load roles: %w", err)
	}
	_ = closeStore // store stays open for the process lifetime

	return &Home{
		RootDir:     absRoot,
		Config:      cfg,
		RoleManager: rm,
	}, nil
}

// Find walks up from dir looking for the enclosing git repo root and
// loads the global mycel state anchored on it. Errors when dir is not
// inside a git repository or mycel was never set up.
func Find(dir string) (*Home, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	current := absDir
	for {
		if _, statErr := os.Stat(filepath.Join(current, ".git")); statErr == nil {
			return Load(current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil, fmt.Errorf("no git repository found (searched from %s to root)", absDir)
		}
		current = parent
	}
}

// Save saves the configuration to ~/.mycel/prefs.json.
func (h *Home) Save() error {
	prefsPath, err := PrefsPath()
	if err != nil {
		return err
	}
	return h.Config.Save(prefsPath)
}

// StateDir returns the global state directory (~/.mycel). Kept as a
// method so consumers don't each re-resolve MycelHome.
func (h *Home) StateDir() string {
	home, err := MycelHome()
	if err != nil {
		return ""
	}
	return home
}

// SettingsFile returns the absolute path of the global preferences
// file (~/.mycel/prefs.json). The path is returned whether or not the
// file exists yet, so callers may safely write to it.
func (h *Home) SettingsFile() string {
	p, err := PrefsPath()
	if err != nil {
		return ""
	}
	return p
}

// AgentsDir returns the agent entity root (~/.mycel/agents).
func (h *Home) AgentsDir() string {
	return filepath.Join(h.StateDir(), globalAgentsDirName)
}

// LogsDir returns the daemon/process logs directory. An absolute
// Config.Logs.Path wins; a relative one is resolved under ~/.mycel;
// empty means ~/.mycel/logs.
func (h *Home) LogsDir() string {
	if h.Config != nil && h.Config.Logs.Path != "" {
		if filepath.IsAbs(h.Config.Logs.Path) {
			return h.Config.Logs.Path
		}
		return filepath.Join(h.StateDir(), h.Config.Logs.Path)
	}
	return filepath.Join(h.StateDir(), globalLogsDirName)
}

// RolesDir returns the legacy filesystem roles directory
// (~/.mycel/roles). Roles live in the database; this directory is only
// consulted as a migration source.
func (h *Home) RolesDir() string {
	return filepath.Join(h.StateDir(), "roles")
}

// GetRole returns a role by name, loading it if necessary.
func (h *Home) GetRole(name string) (*Role, error) {
	if h.RoleManager == nil {
		return nil, fmt.Errorf("role manager not initialized")
	}

	if role, ok := h.RoleManager.GetRole(name); ok {
		return role, nil
	}

	return h.RoleManager.LoadRole(name)
}

// GetRolePrompt returns the prompt content for a role.
func (h *Home) GetRolePrompt(name string) string {
	role, err := h.GetRole(name)
	if err != nil {
		return ""
	}
	return role.Prompt
}

// openRoleStore creates a RoleStore on the single global database
// (<MycelHome>/mycel.db, or the configured TimescaleDB). The
// connection is borrowed from pkg/db and Close on the store is a
// no-op.
func openRoleStore(cfg *Config) (*RoleStore, error) {
	wsDB, driver, err := db.Global(cfg.DBStorageSettings())
	if err != nil {
		return nil, fmt.Errorf("role store: open global db: %w", err)
	}
	return NewRoleStoreFromDB(wsDB.DB, driver)
}

// NewGlobalRoleManager creates a role manager backed by the single global
// database. stateDir is only used to migrate any legacy filesystem roles
// under <stateDir>/roles into the store. Package-level helpers that
// resolve roles without a *Home must use this constructor.
func NewGlobalRoleManager(stateDir string) (*RoleManager, error) {
	store, err := openRoleStore(nil)
	if err != nil {
		return nil, err
	}
	_, _ = store.MigrateFromFiles(filepath.Join(stateDir, "roles")) //nolint:errcheck // best-effort
	return NewRoleManagerWithStore(stateDir, store), nil
}

// initRoleManager creates a role manager with SQL store for Open.
// It creates the store, migrates defaults, and migrates any legacy
// filesystem roles. Returns the manager plus a close function for the
// store.
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

// loadRoleManager creates a role manager with SQL store for Load.
// It opens the store, migrates any new filesystem files, and loads all
// roles into the cache.
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

// DefaultProvider returns the default provider name.
func (h *Home) DefaultProvider() string {
	if h.Config != nil {
		return h.Config.GetDefaultProvider()
	}
	return "claude"
}

// DefaultProviderCommand returns the command for the default provider.
func (h *Home) DefaultProviderCommand() string {
	if h.Config != nil {
		if p := h.Config.GetProvider(h.Config.GetDefaultProvider()); p != nil {
			return p.Command
		}
	}
	return ""
}

// Name returns the anchor repo name (derived from directory), or
// "mycel" when no anchor repo is set.
func (h *Home) Name() string {
	if h.RootDir == "" {
		return "mycel"
	}
	return filepath.Base(h.RootDir)
}
