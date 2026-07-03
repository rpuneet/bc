package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CurrentRegistryVersion is the current registry schema version.
const CurrentRegistryVersion = 2

// registryIDLength is the number of hex characters in a workspace ID.
// sha256(path)[:12] gives 12 hex chars (48 bits) which is collision-safe for
// practical numbers of workspaces.
const registryIDLength = 12

// RegistryEntry represents a registered workspace.
type RegistryEntry struct {
	CreatedAt      time.Time `json:"created_at"`
	LastUsedAt     time.Time `json:"last_used_at,omitempty"` // last time the workspace was used
	ID             string    `json:"id,omitempty"`           // stable 12-char hash of abs path
	Path           string    `json:"path"`                   // project root (pristine git repo)
	DataDir        string    `json:"data_dir,omitempty"`     // runtime state dir (~/.mycel/workspaces/<id>/); M11+
	Name           string    `json:"name"`
	Alias          string    `json:"alias,omitempty"`            // Short alias for quick access (#1218)
	GithubURL      string    `json:"github_url,omitempty"`       // Optional GitHub remote URL (v2+)
	GithubFullName string    `json:"github_full_name,omitempty"` // Optional GitHub owner/repo (v2+)
}

// GetDataDir returns the per-workspace runtime directory. When the
// registry field is empty (older entries), falls back to computing it
// from the workspace ID via DataDir(id). Returns "" if neither is
// available.
func (e *RegistryEntry) GetDataDir() string {
	if e == nil {
		return ""
	}
	if e.DataDir != "" {
		return e.DataDir
	}
	id := e.ID
	if id == "" {
		id = ComputeWorkspaceID(e.Path)
	}
	if id == "" {
		return ""
	}
	dd, err := DataDir(id)
	if err != nil {
		return ""
	}
	return dd
}

// Registry manages the global list of workspaces at ~/.mycel/workspaces.json.
// Issue #1218: Multi-workspace orchestration support.
type Registry struct {
	path       string
	Active     string          `json:"active,omitempty"` // Path or alias of active workspace
	Workspaces []RegistryEntry `json:"workspaces"`
	mu         sync.RWMutex
	Version    int `json:"version,omitempty"`
}

// ComputeWorkspaceID returns the stable 12-char hex ID for an absolute path.
// It is the first registryIDLength hex chars of sha256(absPath). Empty path
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
	return hex.EncodeToString(sum[:])[:registryIDLength]
}

// GlobalDir returns the path to ~/.mycel/. Honors MYCEL_HOME so test isolation
// (or power-user overrides) route every registry read/write through the
// same sandbox — previously this ignored MYCEL_HOME and always read the
// host's real registry, which let tests corrupt production state.
func GlobalDir() string {
	home, err := MycelHome()
	if err != nil {
		return ""
	}
	return home
}

// RegistryPath returns the path to ~/.mycel/workspaces.json (MYCEL_HOME-aware
// via GlobalDir).
func RegistryPath() string {
	return filepath.Join(GlobalDir(), "workspaces.json")
}

// LoadRegistry loads the global workspace registry.
// Returns an empty registry if the file doesn't exist.
// Always normalizes so callers see a fully-populated in-memory shape.
func LoadRegistry() (*Registry, error) {
	r := &Registry{path: RegistryPath()}

	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			r.Version = CurrentRegistryVersion
			return r, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, r); err != nil {
		return nil, err
	}

	r.normalize()
	return r, nil
}

// Save persists the registry to disk atomically: write to a temp file then
// rename over the destination. This prevents corruption if the process is
// killed mid-write.
func (r *Registry) Save() error {
	r.mu.RLock()
	// Ensure version is current before serializing.
	if r.Version == 0 {
		r.mu.RUnlock()
		r.mu.Lock()
		if r.Version == 0 {
			r.Version = CurrentRegistryVersion
		}
		r.mu.Unlock()
		r.mu.RLock()
	}
	data, mErr := json.MarshalIndent(r, "", "  ")
	r.mu.RUnlock()
	if mErr != nil {
		return mErr
	}

	dir := filepath.Dir(r.path)
	if mkErr := os.MkdirAll(dir, 0750); mkErr != nil {
		return mkErr
	}

	tmp, tErr := os.CreateTemp(dir, ".registry-*.tmp")
	if tErr != nil {
		return tErr
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we fail before rename.
	defer func() {
		if _, statErr := os.Stat(tmpName); statErr == nil {
			_ = os.Remove(tmpName) //nolint:errcheck // best-effort cleanup
		}
	}()

	if chErr := tmp.Chmod(0600); chErr != nil {
		_ = tmp.Close() //nolint:errcheck // best-effort close
		return chErr
	}
	if _, wErr := tmp.Write(data); wErr != nil {
		_ = tmp.Close() //nolint:errcheck // best-effort close
		return wErr
	}
	if sErr := tmp.Sync(); sErr != nil {
		_ = tmp.Close() //nolint:errcheck // best-effort close
		return sErr
	}
	if cErr := tmp.Close(); cErr != nil {
		return cErr
	}
	return os.Rename(tmpName, r.path)
}

// normalize ensures every entry has an ID and DataDir set (defensive
// against hand-edited registry files) and stamps the current schema
// version. Returns true if any change was made. Safe to call multiple
// times.
func (r *Registry) normalize() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	changed := false
	// Only bump forward — a newer build's registry must not be silently
	// downgraded when an older binary saves it.
	if r.Version < CurrentRegistryVersion {
		r.Version = CurrentRegistryVersion
		changed = true
	}
	for i := range r.Workspaces {
		if r.Workspaces[i].ID == "" {
			r.Workspaces[i].ID = ComputeWorkspaceID(r.Workspaces[i].Path)
			changed = true
		}
		if r.Workspaces[i].DataDir == "" {
			if dd, err := DataDir(r.Workspaces[i].ID); err == nil {
				r.Workspaces[i].DataDir = dd
				changed = true
			}
		}
	}
	return changed
}

// Register adds or updates a workspace in the registry.
func (r *Registry) Register(path, name string) {
	_ = r.RegisterWithAlias(path, name, "") //nolint:errcheck // alias conflict not possible with empty alias
}

// RegisterWithAlias adds or updates a workspace with an optional alias.
// Issue #1218: Multi-workspace orchestration.
func (r *Registry) RegisterWithAlias(path, name, alias string) error {
	now := time.Now()

	// Check alias conflict if setting one
	if alias != "" {
		existing := r.FindByAlias(alias)
		if existing != nil && existing.Path != path {
			return &AliasConflictError{Alias: alias, ExistingPath: existing.Path}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Version == 0 {
		r.Version = CurrentRegistryVersion
	}

	for i, w := range r.Workspaces {
		if w.Path == path {
			r.Workspaces[i].Name = name
			r.Workspaces[i].LastUsedAt = now
			if r.Workspaces[i].ID == "" {
				r.Workspaces[i].ID = ComputeWorkspaceID(path)
			}
			if r.Workspaces[i].DataDir == "" {
				if dd, err := DataDir(r.Workspaces[i].ID); err == nil {
					r.Workspaces[i].DataDir = dd
				}
			}
			if alias != "" {
				r.Workspaces[i].Alias = alias
			}
			return nil
		}
	}

	id := ComputeWorkspaceID(path)
	dataDir := ""
	if dd, err := DataDir(id); err == nil {
		dataDir = dd
	}
	r.Workspaces = append(r.Workspaces, RegistryEntry{
		ID:         id,
		Path:       path,
		DataDir:    dataDir,
		Name:       name,
		Alias:      alias,
		CreatedAt:  now,
		LastUsedAt: now,
	})
	return nil
}

// Unregister removes a workspace from the registry.
func (r *Registry) Unregister(path string) {
	for i, w := range r.Workspaces {
		if w.Path == path {
			r.Workspaces = append(r.Workspaces[:i], r.Workspaces[i+1:]...)
			return
		}
	}
}

// Touch updates the last-accessed time for a workspace. Accepts a path, id,
// alias, or name — for backwards compatibility, the single argument was
// historically a path.
func (r *Registry) Touch(identifier string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for i, w := range r.Workspaces {
		if w.Path == identifier || w.ID == identifier || w.Alias == identifier || w.Name == identifier {
			r.Workspaces[i].LastUsedAt = now
			return
		}
	}
}

// PruneStalePaths removes entries whose project directory no longer
// exists on disk — e.g. test tmpdirs that the caller created via
// t.TempDir() and that have since been cleaned up. Unlike Prune(), this
// does not look at .bc/ or the global state dir; it only checks that
// the root Path itself is still a directory. Returns the count removed.
//
// Callers should invoke this before performing batch operations against
// the registry (like the M11 runtime migration) to avoid acting on
// phantom entries.
func (r *Registry) PruneStalePaths() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	pruned := 0
	valid := make([]RegistryEntry, 0, len(r.Workspaces))
	for _, w := range r.Workspaces {
		if w.Path == "" {
			pruned++
			continue
		}
		info, err := os.Stat(w.Path)
		if err != nil || !info.IsDir() {
			pruned++
			continue
		}
		valid = append(valid, w)
	}
	r.Workspaces = valid
	return pruned
}

// Prune removes entries where the workspace no longer exists on disk.
// Checks for .bc/ dir in project root OR state dir in ~/.mycel/workspaces/<id>/.
func (r *Registry) Prune() int {
	pruned := 0
	valid := make([]RegistryEntry, 0, len(r.Workspaces))
	for _, w := range r.Workspaces {
		// Check .bc/ runtime marker (agent worktree layout)
		if _, err := os.Stat(filepath.Join(w.Path, ".bc")); err == nil {
			valid = append(valid, w)
			continue
		}
		// Check global state dir exists
		if stateDir, err := GlobalStateDir(w.Path); err == nil {
			if _, statErr := os.Stat(stateDir); statErr == nil {
				valid = append(valid, w)
				continue
			}
		}
		pruned++
	}
	r.Workspaces = valid
	return pruned
}

// List returns all registered workspaces.
func (r *Registry) List() []RegistryEntry {
	return r.Workspaces
}

// Find returns the entry for a given path, or nil if not found.
func (r *Registry) Find(path string) *RegistryEntry {
	for i, w := range r.Workspaces {
		if w.Path == path {
			return &r.Workspaces[i]
		}
	}
	return nil
}

// FindByAlias returns the entry for a given alias, or nil if not found.
// Issue #1218: Multi-workspace orchestration.
func (r *Registry) FindByAlias(alias string) *RegistryEntry {
	if alias == "" {
		return nil
	}
	for i, w := range r.Workspaces {
		if w.Alias == alias {
			return &r.Workspaces[i]
		}
	}
	return nil
}

// FindByID returns the entry for a given workspace ID (v2+), or nil if not
// found. The ID is the 12-char hex hash of the absolute path, independent of
// user-editable fields like name or alias.
func (r *Registry) FindByID(id string) *RegistryEntry {
	if id == "" {
		return nil
	}
	for i, w := range r.Workspaces {
		if w.ID == id {
			return &r.Workspaces[i]
		}
	}
	return nil
}

// Resolve returns the entry matching an id, alias, name, or path. Returns
// nil if none match.
func (r *Registry) Resolve(identifier string) *RegistryEntry {
	if identifier == "" {
		return nil
	}
	if entry := r.FindByID(identifier); entry != nil {
		return entry
	}
	return r.FindByNameOrAlias(identifier)
}

// FindByNameOrAlias returns the entry matching name, alias, or path.
// Tries alias first, then name, then path.
func (r *Registry) FindByNameOrAlias(identifier string) *RegistryEntry {
	// Check alias first (most specific)
	if entry := r.FindByAlias(identifier); entry != nil {
		return entry
	}
	// Check name
	for i, w := range r.Workspaces {
		if w.Name == identifier {
			return &r.Workspaces[i]
		}
	}
	// Check path last
	return r.Find(identifier)
}

// SetAlias sets or clears the alias for a workspace.
func (r *Registry) SetAlias(path, alias string) error {
	// Check if alias is already in use by another workspace
	if alias != "" {
		existing := r.FindByAlias(alias)
		if existing != nil && existing.Path != path {
			return &AliasConflictError{Alias: alias, ExistingPath: existing.Path}
		}
	}

	for i, w := range r.Workspaces {
		if w.Path == path {
			r.Workspaces[i].Alias = alias
			return nil
		}
	}
	return &WorkspaceNotFoundError{Identifier: path}
}

// GetActive returns the active workspace entry, or nil if none set.
func (r *Registry) GetActive() *RegistryEntry {
	if r.Active == "" {
		return nil
	}
	return r.FindByNameOrAlias(r.Active)
}

// SetActive sets the active workspace by path, alias, or name.
func (r *Registry) SetActive(identifier string) error {
	if identifier == "" {
		r.Active = ""
		return nil
	}
	entry := r.FindByNameOrAlias(identifier)
	if entry == nil {
		return &WorkspaceNotFoundError{Identifier: identifier}
	}
	// Store the alias if available, otherwise the path
	if entry.Alias != "" {
		r.Active = entry.Alias
	} else {
		r.Active = entry.Path
	}
	return nil
}

// AliasConflictError indicates the alias is already in use.
type AliasConflictError struct {
	Alias        string
	ExistingPath string
}

func (e *AliasConflictError) Error() string {
	return "alias '" + e.Alias + "' is already in use by workspace at " + e.ExistingPath
}

// WorkspaceNotFoundError indicates the workspace was not found.
type WorkspaceNotFoundError struct {
	Identifier string
}

func (e *WorkspaceNotFoundError) Error() string {
	return "workspace not found: " + e.Identifier
}
