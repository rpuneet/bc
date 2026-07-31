// Package deps implements the optional dependencies manager described in
// docs/proposals/multi-workspace-and-code-tab.md §7.
//
// A Dependency is a named external service (e.g. a database container or a
// code-server instance) that the user can optionally start from the daemon
// Settings UI. The Registry holds the known dependencies; each one is a
// self-contained implementation that knows how to report its status and
// start/stop itself, typically by shelling out to `docker`.
//
// The package is intentionally minimal: no goroutines, no watchers, no
// background loops. Callers (HTTP handlers, CLI, tests) invoke the methods
// synchronously and rely on the operating system (Docker) for the real
// lifecycle.
package deps

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// State is the coarse runtime state of a dependency.
type State string

// Known state values. Implementations should stick to these so the UI can
// map them to colored status dots.
const (
	StateRunning  State = "running"
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateStopping State = "stopping"
	StateError    State = "error"
	StateUnknown  State = "unknown"
)

// Dependency is the contract every optional service must satisfy.
//
// Implementations are expected to be cheap to instantiate and to do all real
// work inside the interface methods so the registry itself can be built
// eagerly at server boot without side effects.
type Dependency interface {
	// ID is the stable short identifier, e.g. "mycel-db".
	ID() string
	// DisplayName is the human-friendly name shown in the UI.
	DisplayName() string
	// Description is a one-line explanation of the dependency.
	Description() string
	// Status reports the current state of the dependency.
	Status(ctx context.Context) (State, error)
	// Start launches the dependency. It is safe to call when already running.
	Start(ctx context.Context) error
	// Stop halts the dependency. It is safe to call when already stopped.
	Stop(ctx context.Context) error
	// Logs returns up to tail lines of recent output, newest last.
	Logs(ctx context.Context, tail int) ([]string, error)
	// Deprecated reports whether the dependency is retained for
	// discoverability only. Deprecated deps refuse Start.
	Deprecated() bool
}

// ErrNotFound is returned by Registry.Get when the ID is not registered.
var ErrNotFound = errors.New("dependency not found")

// ErrDeprecated is returned by Start on deprecated dependencies.
var ErrDeprecated = errors.New("dependency deprecated")

// Registry is a small, concurrency-safe map of dependencies keyed by ID.
type Registry struct {
	deps map[string]Dependency
	mu   sync.RWMutex
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{deps: make(map[string]Dependency)}
}

// Register adds d to the registry, overwriting any existing entry with the
// same ID. Passing nil is a no-op.
func (r *Registry) Register(d Dependency) {
	if d == nil {
		return
	}
	r.mu.Lock()
	r.deps[d.ID()] = d
	r.mu.Unlock()
}

// Get returns the dependency with the given ID, or ErrNotFound.
func (r *Registry) Get(id string) (Dependency, error) {
	r.mu.RLock()
	d, ok := r.deps[id]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return d, nil
}

// List returns every registered dependency sorted by ID for stable output.
func (r *Registry) List() []Dependency {
	r.mu.RLock()
	out := make([]Dependency, 0, len(r.deps))
	for _, d := range r.deps {
		out = append(out, d)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}
