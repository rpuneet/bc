// Package server — workspace_services.go provides per-workspace service
// bundling and a lazy-loading manager so bcd can hold multiple workspaces
// open simultaneously.
//
// `WorkspaceServices` holds per-workspace stores with a `Workspace`
// reference plus a closer so the manager can shut down a workspace cleanly
// on eviction. The flat `Services` struct (server.go) is projected from
// these fields when constructing the HTTP server.
//
// Call flow:
//
//	bcd boot
//	  → NewWorkspaceManager(registry, factory)
//	  → mgr.LoadActive(ctx)          // eager-load the active workspace
//	  → handler: mgr.Get(id) or mgr.Load(ctx, id)  // lazy-load on demand
//	  → background goroutine every ~1m evicts idle entries after 30m
//
// The factory is injected so internal/cmd/serve.go can supply the real
// initialization code (stats store, cron scheduler, gateway manager, etc.)
// without this package depending on it.
package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/cron"
	"github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/mcp"
	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/stats"
	"github.com/rpuneet/mycel/pkg/template"
	"github.com/rpuneet/mycel/pkg/tool"
	"github.com/rpuneet/mycel/pkg/workspace"
	"github.com/rpuneet/mycel/server/handlers"
	"github.com/rpuneet/mycel/server/ws"
)

// idleEvictionThreshold is how long a workspace must sit untouched before the
// manager closes it. 30 minutes matches the proposal (§4.2).
const idleEvictionThreshold = 30 * time.Minute

// evictionLoopInterval is how often the manager sweeps for idle workspaces.
const evictionLoopInterval = 1 * time.Minute

// WorkspaceServices holds the complete set of per-workspace stores and
// managers for one workspace.
//
// This struct is the unit of eviction: when the manager closes a workspace,
// it calls Close which tears down every service that was opened via the
// factory. Handler constructors receive a flat Services struct projected
// from these named fields via NewWithManager / New.
type WorkspaceServices struct {
	Workspace *workspace.Workspace

	// Per-workspace stores/services (populated by the factory in serve.go).
	// May be nil when the corresponding backend is unavailable in a given
	// workspace (e.g. secret store if passphrase missing).
	Agents       *agent.AgentService
	AgentMgr     *agent.Manager
	Channels     *notify.Service // bc currently co-locates channels in notify
	Events       events.EventStore
	EventWriter  *events.JSONLWriter
	Costs        *cost.Store
	CostImporter *cost.Importer
	Cron         *cron.Store
	CronSched    *cron.Scheduler
	Templates    *template.Store
	Secrets      *secret.Store
	MCP          *mcp.Store
	MCPGlobal    *mcp.GlobalStore // user-global MCP registry (~/.mycel/mcps.json) — shared across workspaces
	Tools        *tool.Store
	Gateway      *gateway.Manager
	Notify       *notify.Service
	Hub          *ws.Hub
	Stats        *stats.Store // global stats store — set by factory when Globals.Stats is available

	// Degraded maps service name → human-readable reason for every
	// service that failed to initialize and was left nil by the factory
	// (warn-and-continue sites in build_services.go). Surfaced via
	// /api/health, `mycel doctor`, and 503 responses so silent
	// degradation becomes loud and diagnosable.
	Degraded map[string]string

	// cancel stops background goroutines started by the factory. It is
	// optional — the closer is the ultimate resource-teardown call, but
	// cancel is invoked first so goroutines can observe shutdown before
	// their underlying stores close.
	cancel context.CancelFunc
	// wg lets Close wait for background goroutines to exit.
	wg *sync.WaitGroup

	closer     func() error
	lastAccess time.Time
	mu         sync.Mutex
}

// Touch marks this workspace as recently used; prevents idle eviction.
func (ws *WorkspaceServices) Touch() {
	ws.mu.Lock()
	ws.lastAccess = time.Now()
	ws.mu.Unlock()
}

// LastAccess returns the last time Touch was called.
func (ws *WorkspaceServices) LastAccess() time.Time {
	ws.mu.Lock()
	t := ws.lastAccess
	ws.mu.Unlock()
	return t
}

// Close stops background goroutines started by the factory, waits for them
// to exit, then invokes the factory-supplied closer to tear down stores.
// Safe to call multiple times.
//
// The per-workspace database connection is deliberately NOT closed here:
// stores borrow it from the pkg/db registry, which keeps it cached so an
// evicted-then-reloaded workspace reuses the same handle (an idle SQLite
// connection is one pooled conn — cheap) and so other holders (role
// store, CLI helpers) never see a use-after-close. Registry connections
// are closed at process shutdown via db.CloseAllWorkspaceDBs.
func (ws *WorkspaceServices) Close() error {
	ws.mu.Lock()
	cancel := ws.cancel
	wg := ws.wg
	c := ws.closer
	ws.cancel = nil
	ws.wg = nil
	ws.closer = nil
	ws.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if wg != nil {
		wg.Wait()
	}
	if c == nil {
		return nil
	}
	return c()
}

// workspaceViewFromServices projects a *WorkspaceServices onto the
// handler-facing WorkspaceView type. Exists to avoid importing handlers
// from this file and vice-versa.
func workspaceViewFromServices(svc *WorkspaceServices) *handlers.WorkspaceView {
	if svc == nil {
		return nil
	}
	return &handlers.WorkspaceView{
		Workspace:    svc.Workspace,
		Agents:       svc.Agents,
		Events:       svc.Events,
		EventWriter:  svc.EventWriter,
		Costs:        svc.Costs,
		CostImporter: svc.CostImporter,
		Cron:         svc.Cron,
		CronSched:    svc.CronSched,
		Templates:    svc.Templates,
		Secrets:      svc.Secrets,
		MCP:          svc.MCP,
		Tools:        svc.Tools,
		Gateway:      svc.Gateway,
		Notify:       svc.Notify,
		Stats:        svc.Stats,
		Hub:          svc.Hub,
		Degraded:     svc.Degraded,
	}
}

// WorkspaceFactory builds a fully-initialized WorkspaceServices for the given
// workspace root. Implementations should open all stores, start background
// loops, and return a closer that tears them down in reverse order.
type WorkspaceFactory func(ctx context.Context, ws *workspace.Workspace) (*WorkspaceServices, error)

// WorkspaceManager caches per-workspace services, lazy-loading on first
// access and evicting idle entries on a background loop.
type WorkspaceManager struct {
	registry *workspace.Registry
	factory  WorkspaceFactory
	byID     map[string]*WorkspaceServices
	stopCh   chan struct{}
	mu       sync.RWMutex
	stopOnce sync.Once
}

// NewWorkspaceManager constructs a manager bound to the given registry and
// factory. Nothing is loaded until Load / LoadActive is called.
func NewWorkspaceManager(registry *workspace.Registry, factory WorkspaceFactory) *WorkspaceManager {
	return &WorkspaceManager{
		registry: registry,
		factory:  factory,
		byID:     make(map[string]*WorkspaceServices),
		stopCh:   make(chan struct{}),
	}
}

// Registry returns the underlying workspace registry.
func (m *WorkspaceManager) Registry() *workspace.Registry {
	return m.registry
}

// Get returns already-loaded services for the given workspace ID, or nil if
// the workspace is not currently loaded. It does NOT trigger a load.
func (m *WorkspaceManager) Get(wsID string) *WorkspaceServices {
	m.mu.RLock()
	svc := m.byID[wsID]
	m.mu.RUnlock()
	if svc != nil {
		svc.Touch()
	}
	return svc
}

// Load returns services for the given workspace ID, initializing them if
// necessary. Returns an error if the workspace is not in the registry or if
// the factory fails.
func (m *WorkspaceManager) Load(ctx context.Context, wsID string) (*WorkspaceServices, error) {
	if wsID == "" {
		return nil, errors.New("workspace id required")
	}

	// Fast path: already loaded.
	m.mu.RLock()
	svc, ok := m.byID[wsID]
	m.mu.RUnlock()
	if ok {
		svc.Touch()
		return svc, nil
	}

	// Slow path: resolve, load under write lock.
	m.mu.Lock()
	defer m.mu.Unlock()

	if svc, ok := m.byID[wsID]; ok {
		svc.Touch()
		return svc, nil
	}

	if m.registry == nil {
		return nil, errors.New("workspace registry unavailable")
	}

	entry := m.registry.FindByID(wsID)
	if entry == nil {
		// Accept alias/name/path for caller convenience.
		entry = m.registry.Resolve(wsID)
	}
	if entry == nil {
		return nil, fmt.Errorf("workspace %q not registered", wsID)
	}

	ws, err := workspace.Load(entry.Path)
	if err != nil {
		// Only auto-init if the workspace is uninitialized (no config file).
		// Permission errors, corrupt config, etc. should propagate as real errors.
		errMsg := err.Error()
		if !strings.Contains(errMsg, "not a bc workspace") && !strings.Contains(errMsg, "no preferences.json") && !strings.Contains(errMsg, "no settings.json") {
			return nil, fmt.Errorf("load workspace %s: %w", entry.Path, err)
		}
		ws, err = workspace.Init(entry.Path)
		if err != nil {
			return nil, fmt.Errorf("init workspace %s: %w", entry.Path, err)
		}
		log.Info("workspace auto-initialized on first access", "path", entry.Path)
	}

	if m.factory == nil {
		return nil, errors.New("workspace manager has no factory configured")
	}

	built, err := m.factory(ctx, ws)
	if err != nil {
		return nil, fmt.Errorf("init services for %s: %w", entry.Path, err)
	}
	if built == nil {
		return nil, fmt.Errorf("factory returned nil services for %s", entry.Path)
	}
	built.Workspace = ws
	built.Touch()

	m.byID[wsID] = built

	// Best-effort: bump LastUsedAt in the registry.
	m.registry.Touch(wsID)

	log.Info("workspace services loaded", "id", wsID, "path", entry.Path)
	return built, nil
}

// LoadActive loads the registry's active workspace. Returns an error if no
// active workspace is set.
func (m *WorkspaceManager) LoadActive(ctx context.Context) (*WorkspaceServices, error) {
	if m.registry == nil {
		return nil, errors.New("workspace registry unavailable")
	}
	active := m.registry.GetActive()
	if active == nil {
		return nil, errors.New("no active workspace")
	}
	if active.ID == "" {
		active.ID = workspace.ComputeWorkspaceID(active.Path)
	}
	return m.Load(ctx, active.ID)
}

// Active returns the services for the active workspace if already loaded,
// else nil. Never triggers a load.
func (m *WorkspaceManager) Active() *WorkspaceServices {
	if m.registry == nil {
		return nil
	}
	active := m.registry.GetActive()
	if active == nil {
		return nil
	}
	id := active.ID
	if id == "" {
		id = workspace.ComputeWorkspaceID(active.Path)
	}
	return m.Get(id)
}

// List returns all currently-loaded workspace services.
func (m *WorkspaceManager) List() []*WorkspaceServices {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*WorkspaceServices, 0, len(m.byID))
	for _, v := range m.byID {
		out = append(out, v)
	}
	return out
}

// Evict closes and removes the services for the given workspace ID.
func (m *WorkspaceManager) Evict(wsID string) error {
	m.mu.Lock()
	svc, ok := m.byID[wsID]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.byID, wsID)
	m.mu.Unlock()
	if err := svc.Close(); err != nil {
		log.Warn("workspace evict: close failed", "id", wsID, "error", err)
		return err
	}
	log.Info("workspace services evicted", "id", wsID)
	return nil
}

// Close tears down every loaded workspace. Safe to call multiple times.
func (m *WorkspaceManager) Close() error {
	m.stopOnce.Do(func() { close(m.stopCh) })

	m.mu.Lock()
	ids := make([]string, 0, len(m.byID))
	svcs := make([]*WorkspaceServices, 0, len(m.byID))
	for id, svc := range m.byID {
		ids = append(ids, id)
		svcs = append(svcs, svc)
	}
	m.byID = make(map[string]*WorkspaceServices)
	m.mu.Unlock()

	var firstErr error
	for i, svc := range svcs {
		if err := svc.Close(); err != nil {
			log.Warn("workspace close: error", "id", ids[i], "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// StartEvictionLoop launches a goroutine that periodically evicts workspaces
// whose LastAccess is older than the idle threshold. The loop exits when ctx
// is canceled or Close is called.
//
// The registry's active workspace is never evicted so that legacy /api/...
// routes always find a default.
func (m *WorkspaceManager) StartEvictionLoop(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(evictionLoopInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.sweepIdle()
			}
		}
	}()
}

// sweepIdle evicts any workspace idle beyond the threshold, except the
// registry's active workspace.
func (m *WorkspaceManager) sweepIdle() {
	var activeID string
	if m.registry != nil {
		if a := m.registry.GetActive(); a != nil {
			activeID = a.ID
			if activeID == "" {
				activeID = workspace.ComputeWorkspaceID(a.Path)
			}
		}
	}

	cutoff := time.Now().Add(-idleEvictionThreshold)

	m.mu.RLock()
	candidates := make([]string, 0)
	for id, svc := range m.byID {
		if id == activeID {
			continue
		}
		if svc.LastAccess().Before(cutoff) {
			candidates = append(candidates, id)
		}
	}
	m.mu.RUnlock()

	for _, id := range candidates {
		if err := m.Evict(id); err != nil {
			log.Warn("workspace evict: error during idle sweep", "id", id, "error", err)
		}
	}
}
