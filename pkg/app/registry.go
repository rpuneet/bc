package app

import "sort"

// Registry holds registered app plugins keyed by descriptor ID.
type Registry struct {
	plugins map[string]Plugin
}

// NewRegistry creates an empty plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
	}
}

// Register adds a plugin to the registry.
func (r *Registry) Register(p Plugin) {
	r.plugins[p.Describe().ID] = p
}

// Get returns a plugin by descriptor ID.
func (r *Registry) Get(id string) (Plugin, bool) {
	p, ok := r.plugins[id]
	return p, ok
}

// List returns all registered plugins sorted by descriptor ID.
func (r *Registry) List() []Plugin {
	plugins := make([]Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		plugins = append(plugins, p)
	}
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Describe().ID < plugins[j].Describe().ID
	})
	return plugins
}

// DefaultRegistry is the global app plugin registry. Plugin packages
// register themselves in init(); pkg/app/builtin imports the enabled set
// for side effects.
var DefaultRegistry = NewRegistry()

// Register adds a plugin to the default registry.
func Register(p Plugin) {
	DefaultRegistry.Register(p)
}

// Get returns a plugin from the default registry by descriptor ID.
func Get(id string) (Plugin, bool) {
	return DefaultRegistry.Get(id)
}

// List returns all plugins in the default registry sorted by ID.
func List() []Plugin {
	return DefaultRegistry.List()
}
