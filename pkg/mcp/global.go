package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Scope reports which layer of a LayeredStore owns an MCP server config.
type Scope string

const (
	// ScopeGlobal is the user-global MCP registry (~/.mycel/mcps.json).
	ScopeGlobal Scope = "global"
	// ScopeWorkspace is a repo-scoped override (SQLite mcp_servers
	// table in the global mycel.db).
	ScopeWorkspace Scope = "workspace"
)

// GlobalStore is a JSON-file-backed registry of user-trusted MCP
// servers. It lives at ~/.mycel/mcps.json and is shared across all bc
// agents. Concurrency is handled with a file read/write under a
// mutex; a serving process that needs ticker-level freshness should
// re-read on each access.
type GlobalStore struct {
	path string
	mu   sync.Mutex
}

// NewGlobalStore returns a GlobalStore backed by the given path. The
// parent directory is created on first write. Missing files are
// treated as empty registries (List returns nil).
func NewGlobalStore(path string) *GlobalStore {
	return &GlobalStore{path: path}
}

// Path returns the on-disk path for the JSON file.
func (g *GlobalStore) Path() string { return g.path }

// globalRegistry is the on-disk shape of ~/.mycel/mcps.json.
type globalRegistry struct {
	Servers []*ServerConfig `json:"servers"`
	Version int             `json:"version"`
}

// load parses the JSON file; absent file returns an empty registry.
func (g *GlobalStore) load() (*globalRegistry, error) {
	data, err := os.ReadFile(g.path) //nolint:gosec // controlled path under the mycel home
	if os.IsNotExist(err) {
		return &globalRegistry{Version: 1}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read mcps.json: %w", err)
	}
	if len(data) == 0 {
		return &globalRegistry{Version: 1}, nil
	}
	var reg globalRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse mcps.json: %w", err)
	}
	if reg.Version == 0 {
		reg.Version = 1
	}
	return &reg, nil
}

// save atomically rewrites the JSON file via a tmp-file rename.
func (g *GlobalStore) save(reg *globalRegistry) error {
	if err := os.MkdirAll(filepath.Dir(g.path), 0750); err != nil {
		return fmt.Errorf("create mcps.json dir: %w", err)
	}
	// Stable ordering so diffs are readable.
	sort.SliceStable(reg.Servers, func(i, j int) bool {
		return reg.Servers[i].Name < reg.Servers[j].Name
	})
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mcps.json: %w", err)
	}
	tmp := g.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0640); err != nil { //nolint:gosec // 0640 intentional
		return fmt.Errorf("write tmp mcps.json: %w", err)
	}
	if err := os.Rename(tmp, g.path); err != nil {
		return fmt.Errorf("rename mcps.json: %w", err)
	}
	return nil
}

// List returns every registered server config.
func (g *GlobalStore) List() ([]*ServerConfig, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	reg, err := g.load()
	if err != nil {
		return nil, err
	}
	out := make([]*ServerConfig, len(reg.Servers))
	copy(out, reg.Servers)
	return out, nil
}

// Get returns a single server config by name; nil when absent.
func (g *GlobalStore) Get(name string) (*ServerConfig, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	reg, err := g.load()
	if err != nil {
		return nil, err
	}
	for _, s := range reg.Servers {
		if s.Name == name {
			c := *s
			return &c, nil
		}
	}
	return nil, nil
}

// Add inserts a new server config; fails when a name collision exists.
func (g *GlobalStore) Add(cfg *ServerConfig) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	reg, err := g.load()
	if err != nil {
		return err
	}
	for _, s := range reg.Servers {
		if s.Name == cfg.Name {
			return fmt.Errorf("mcp server %q already exists (use 'mycel mcp remove %s' first)", cfg.Name, cfg.Name)
		}
	}
	copy := *cfg
	if copy.CreatedAt.IsZero() {
		copy.CreatedAt = time.Now().UTC()
	}
	reg.Servers = append(reg.Servers, &copy)
	return g.save(reg)
}

// Upsert replaces an existing entry or adds it when not present.
func (g *GlobalStore) Upsert(cfg *ServerConfig) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	reg, err := g.load()
	if err != nil {
		return err
	}
	for i, s := range reg.Servers {
		if s.Name == cfg.Name {
			copy := *cfg
			if copy.CreatedAt.IsZero() {
				copy.CreatedAt = s.CreatedAt
			}
			reg.Servers[i] = &copy
			return g.save(reg)
		}
	}
	copy := *cfg
	if copy.CreatedAt.IsZero() {
		copy.CreatedAt = time.Now().UTC()
	}
	reg.Servers = append(reg.Servers, &copy)
	return g.save(reg)
}

// Remove deletes a server by name.
func (g *GlobalStore) Remove(name string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	reg, err := g.load()
	if err != nil {
		return err
	}
	for i, s := range reg.Servers {
		if s.Name == name {
			reg.Servers = append(reg.Servers[:i], reg.Servers[i+1:]...)
			return g.save(reg)
		}
	}
	return fmt.Errorf("mcp server %q not found", name)
}

// SetEnabled flips the enabled flag on an existing server.
func (g *GlobalStore) SetEnabled(name string, enabled bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	reg, err := g.load()
	if err != nil {
		return err
	}
	for _, s := range reg.Servers {
		if s.Name == name {
			s.Enabled = enabled
			return g.save(reg)
		}
	}
	return fmt.Errorf("mcp server %q not found", name)
}

// LayeredView composes the global registry with an optional DB-backed
// Store. Reads merge the two with DB overrides winning; writes
// go to whichever layer the caller selects via the scope argument.
// This view is read-mostly and intentionally thin — handlers continue
// to use either GlobalStore or *Store directly depending on scope.
type LayeredView struct {
	Global *GlobalStore
	DB     *Store
}

// List returns the union of both layers, the DB layer wins on name
// collision. Each returned ServerConfig has no Scope field (the
// ServerConfig struct is used on-disk too); callers who need the
// owning scope should call ListScoped.
func (v *LayeredView) List() ([]*ServerConfig, error) {
	byName := map[string]*ServerConfig{}
	order := []string{}
	if v.Global != nil {
		gs, err := v.Global.List()
		if err != nil {
			return nil, err
		}
		for _, s := range gs {
			byName[s.Name] = s
			order = append(order, s.Name)
		}
	}
	if v.DB != nil {
		h, err := v.DB.List()
		if err != nil {
			return nil, err
		}
		for _, s := range h {
			if _, seen := byName[s.Name]; !seen {
				order = append(order, s.Name)
			}
			byName[s.Name] = s
		}
	}
	out := make([]*ServerConfig, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out, nil
}

// ScopedServer pairs a ServerConfig with the Scope that owns it.
type ScopedServer struct {
	Config *ServerConfig
	Scope  Scope
}

// ListScoped returns the union annotated with owning scope. DB entries
// entries override global when a name collides.
func (v *LayeredView) ListScoped() ([]ScopedServer, error) {
	byName := map[string]ScopedServer{}
	order := []string{}
	if v.Global != nil {
		gs, err := v.Global.List()
		if err != nil {
			return nil, err
		}
		for _, s := range gs {
			byName[s.Name] = ScopedServer{Config: s, Scope: ScopeGlobal}
			order = append(order, s.Name)
		}
	}
	if v.DB != nil {
		h, err := v.DB.List()
		if err != nil {
			return nil, err
		}
		for _, s := range h {
			if _, seen := byName[s.Name]; !seen {
				order = append(order, s.Name)
			}
			byName[s.Name] = ScopedServer{Config: s, Scope: ScopeWorkspace}
		}
	}
	out := make([]ScopedServer, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out, nil
}

// Get returns the DB entry when present, else the global.
func (v *LayeredView) Get(name string) (*ServerConfig, Scope, error) {
	if v.DB != nil {
		if s, err := v.DB.Get(name); err == nil && s != nil {
			return s, ScopeWorkspace, nil
		}
	}
	if v.Global != nil {
		if s, err := v.Global.Get(name); err == nil && s != nil {
			return s, ScopeGlobal, nil
		}
	}
	return nil, "", fmt.Errorf("mcp server %q not found", name)
}
