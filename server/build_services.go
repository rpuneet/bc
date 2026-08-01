// build_services.go — factory for the single the daemon service bundle.
//
// the daemon is single-tenant: one Services bundle is built at boot against the
// one global database (db.Global) and lives for the process lifetime.
// A single call to BuildServices(ctx, globals, repoRoot) produces the
// fully-initialized bundle including background goroutines. Its Close()
// cancels those goroutines and closes each store.
package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
	apppkg "github.com/rpuneet/mycel/pkg/app"

	// Ensure the built-in app plugins are registered before the gateway
	// manager iterates cfg.Apps.
	_ "github.com/rpuneet/mycel/pkg/app/builtin"
	containerpkg "github.com/rpuneet/mycel/pkg/container"
	"github.com/rpuneet/mycel/pkg/cost"
	dbpkg "github.com/rpuneet/mycel/pkg/db"
	depspkg "github.com/rpuneet/mycel/pkg/deps"
	eventspkg "github.com/rpuneet/mycel/pkg/events"
	gatewaypkg "github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/log"
	mcppkg "github.com/rpuneet/mycel/pkg/mcp"
	notifypkg "github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/provider"
	secretpkg "github.com/rpuneet/mycel/pkg/secret"
	statspkg "github.com/rpuneet/mycel/pkg/stats"
	templatepkg "github.com/rpuneet/mycel/pkg/template"
	toolpkg "github.com/rpuneet/mycel/pkg/tool"
	wspkg "github.com/rpuneet/mycel/server/ws"
)

// Globals holds dependencies that are process-wide and independent of the
// bundle's anchor repo. the daemon builds one Globals at boot and hands it to
// BuildServices exactly once.
type Globals struct {
	Stats        *statspkg.Store     // nil when TSDB unavailable
	Deps         *depspkg.Registry   // optional dependencies registry (mycel-db, etc.)
	Hub          *wspkg.Hub          // the one SSE hub for /api/events (owned by the caller)
	Templates    *templatepkg.Store  // user-global template store (~/.mycel/templates/)
	SecretsVault *secretpkg.Store    // user-global secrets vault (~/.mycel/secrets.vault)
	MCPGlobal    *mcppkg.GlobalStore // user-global MCP registry (~/.mycel/mcps.json)
	Build        BuildInfo
}

// BuildServices constructs the single fully-initialized Services bundle
// anchored at repoRoot (the repo the daemon was booted against — new agents default
// their repo to it). All background goroutines are started under an
// internal context that Close() cancels.
//
// The returned *Services has its closer field set to a function that stops
// goroutines and closes stores. The caller (RunServer) invokes Close() at
// shutdown.
func BuildServices(ctx context.Context, globals *Globals, repoRoot string) (*Services, error) {
	// Open is idempotent: it bootstraps ~/.mycel (prefs.json, dirs) on
	// first run and loads the existing config afterwards. repoRoot may be
	// empty — the daemon then boots without an anchor repo and agents
	// carry their own repo paths.
	h, err := home.Open(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("open home for repo %q: %w", repoRoot, err)
	}
	return buildServicesFromHome(ctx, globals, h)
}

// buildServicesFromHome is the inner factory used when the caller already
// holds a loaded *home.Home.
//
//nolint:gocyclo // Linear dependency chain; splitting obscures the flow.
func buildServicesFromHome(ctx context.Context, globals *Globals, h *home.Home) (*Services, error) {
	// Child context + waitgroup so Close() can stop every goroutine spawned
	// below and wait for them to exit.
	svcCtx, svcCancel := context.WithCancel(ctx)
	var wg sync.WaitGroup

	// Track cleanup actions; closer invokes them in reverse order.
	var closers []func() error
	addCloser := func(f func() error) { closers = append(closers, f) }

	// Degraded services registry — every warn-and-continue site below
	// records its failure here so /api/health, `mycel doctor`, and 503
	// responses can surface WHY a service is missing instead of a bare
	// "not available".
	degraded := map[string]string{}

	// Single global database: every repo's stores share the one
	// mycel.db at <MycelHome>/mycel.db (or the global TimescaleDB).
	// Isolation between repos comes from data keys (agent name, repo
	// path), not from separate files. The handle is cached process-wide
	// and stays open across service eviction; stores borrow it and
	// never close it.
	wsDB, wsDriver, dbErr := dbpkg.Global(h.Config.DBStorageSettings())
	if dbErr != nil {
		log.Warn("global database unavailable", "error", dbErr, "repo", h.RootDir)
		degraded["storage"] = "global database unavailable: " + dbErr.Error()
	}

	// Storage driver mismatch: pkg/db falls back to SQLite when the
	// configured TimescaleDB is unreachable (db.OpenGlobalDBWithConfig
	// logs "falling back to sqlite"). Compare the configured default
	// against the driver actually in use so the mismatch is visible.
	if h.Config != nil && dbErr == nil {
		configured := h.Config.Storage.Default
		if (configured == "timescale" || configured == "sql") && wsDriver != "timescale" {
			active := wsDriver
			if active == "" {
				active = "none"
			}
			degraded["storage"] = fmt.Sprintf(
				"configured %s database is not active (running on %s) — timescale unreachable, data is going to the fallback store and will not sync back",
				configured, active)
		}
	}

	// Events JSONL writer (append-only) — lives with the other process
	// logs at ~/.mycel/logs/.
	eventsJSONL := filepath.Join(h.LogsDir(), "events.jsonl")
	eventWriter := eventspkg.NewJSONLWriter(eventsJSONL, 0)

	// The one SSE hub. the daemon is single-tenant, so the bundle publishes
	// straight into the process-wide hub supplied via Globals (owned by
	// the caller — no closer). Legacy callers/tests that don't wire a
	// hub get a private one that the closer tears down.
	var hub *wspkg.Hub
	if globals != nil && globals.Hub != nil {
		hub = globals.Hub
	} else {
		hub = wspkg.NewHub()
		go hub.Run()
		addCloser(func() error { hub.Stop(); return nil })
	}

	// Agent manager + service.
	agentMgr, containerBackend, runtimeReason, agentErr := newAgentManager(h)
	if agentErr != nil {
		svcCancel()
		return nil, fmt.Errorf("agent manager: %w", agentErr)
	}
	if runtimeReason != "" {
		degraded["runtime"] = runtimeReason
	}
	if err := agentMgr.LoadState(); err != nil {
		log.Warn("failed to load agent state", "error", err, "repo", h.RootDir)
	}
	if h.RoleManager != nil {
		agentMgr.SetRoleManager(h.RoleManager)
	}
	if h.Config != nil {
		agentMgr.ApplyConfig(h.Config)
	}
	addCloser(func() error { return agentMgr.Close() })

	// Background container metrics collector (Docker backend only).
	if containerBackend != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runContainerStatsCollector(svcCtx, containerBackend, agentMgr)
		}()
	}

	// Tool health loop.
	agentMgr.StartToolHealthLoop(svcCtx, agentpkg.DefaultToolHealthInterval)
	addCloser(func() error { agentMgr.StopToolHealthLoop(); return nil })

	// Source-direct cost service: costs are computed from provider
	// session files on demand (60s in-process cache, ?refresh=1 to
	// invalidate) — there is no ledger. Budget thresholds live in
	// prefs.json and are evaluated against the computed totals.
	userHome, homeErr := os.UserHomeDir()
	if homeErr != nil {
		log.Warn("cannot resolve user home for cost sources", "error", homeErr)
	}
	costSvc := cost.NewService(provider.DefaultRegistry, cost.Options{
		Home:      userHome,
		AgentsDir: h.AgentsDir(),
	}, &prefsBudgetStore{h: h})

	// Wire cost querier into agent service.
	agentSvc := agentpkg.NewAgentService(agentMgr, hub, &costServiceAdapter{svc: costSvc})

	// Secret store. Prefer the user-global vault (~/.mycel/secrets.vault)
	// supplied by Globals so a single secret set once is visible across
	// every repo. When Globals.SecretsVault is unset (legacy
	// callers), fall back to the repo-scoped <repo>/.mycel/secrets.db.
	var secretStore *secretpkg.Store
	if globals != nil && globals.SecretsVault != nil {
		secretStore = globals.SecretsVault
		// Don't register a closer: ownership stays with whoever
		// populated Globals (typically RunServer).
	} else if passphrase, passErr := secretpkg.Passphrase(); passErr != nil {
		log.Warn("secret passphrase unavailable — secret store disabled", "error", passErr)
		degraded["secrets"] = "secret passphrase unavailable: " + passErr.Error()
	} else if ss, err := secretpkg.NewStore(h.RootDir, passphrase); err != nil {
		log.Warn("secret store unavailable", "error", err, "repo", h.RootDir)
		degraded["secrets"] = "secret store unavailable: " + err.Error()
	} else {
		secretStore = ss
		addCloser(func() error { return ss.Close() })
	}

	// MCP store.
	var mcpStore *mcppkg.Store
	if ms, err := mcppkg.NewStore(wsDB, wsDriver); err != nil {
		log.Warn("mcp store unavailable", "error", err, "repo", h.RootDir)
		degraded["mcp"] = "mcp store unavailable: " + err.Error()
	} else {
		mcpStore = ms
		addCloser(func() error { return ms.Close() })
	}

	// Tool store.
	var toolStore *toolpkg.Store
	{
		ts := toolpkg.NewStore(wsDB, wsDriver)
		if err := ts.Open(); err != nil {
			log.Warn("tool store unavailable", "error", err, "repo", h.RootDir)
			degraded["tools"] = "tool store unavailable: " + err.Error()
		} else {
			toolStore = ts
			addCloser(func() error { return ts.Close() })
		}
	}

	// Template store: the single user-global store (~/.mycel/templates/).
	var tmplStore *templatepkg.Store
	if globals != nil && globals.Templates != nil {
		tmplStore = globals.Templates
	} else {
		tmplStore = templatepkg.NewStore(filepath.Join(h.StateDir(), "templates"))
	}

	// Event log (SQLite) + pruning loop.
	var eventLog eventspkg.EventStore
	if el, err := eventspkg.OpenLog(wsDB, wsDriver); err != nil {
		log.Warn("event log unavailable", "error", err, "repo", h.RootDir)
		degraded["events"] = "event log unavailable: " + err.Error()
	} else {
		eventLog = el
		addCloser(func() error { return el.Close() })
		if prunable, ok := el.(*eventspkg.SQLiteLog); ok {
			wg.Add(1)
			go func() {
				defer wg.Done()
				runEventPruneLoop(svcCtx, prunable)
			}()
		}
	}

	// Hook-event persistence is service-owned: IngestHookEvent (shared by
	// the HTTP hook endpoint and future transcript tailers) appends here.
	if eventLog != nil {
		agentSvc.SetHookEventStore(eventLog)
	}

	// Stats collector — only runs if a TSDB stats store is configured
	// globally. Uses the current agentSvc.
	if globals != nil && globals.Stats != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runStatsCollector(svcCtx, globals.Stats, agentSvc)
		}()
	}

	// Transcript tailer — captures Live activity for providers that write a
	// readable session log instead of invoking hooks (ActivityModeTranscript,
	// e.g. pi). Parsed activity flows through the same IngestHookEvent path as
	// HTTP hooks, so both feed one Live feed.
	wg.Add(1)
	go func() {
		defer wg.Done()
		runTranscriptTailer(svcCtx, agentSvc)
	}()

	// Notify service (channel subscriptions + delivery).
	var notifyService *notifypkg.Service
	if ns, err := notifypkg.OpenStore(wsDB, wsDriver); err != nil {
		log.Warn("notify store unavailable", "error", err, "repo", h.RootDir)
		degraded["notify"] = "notify store unavailable: " + err.Error()
	} else {
		notifyService = notifypkg.NewServiceWithContext(svcCtx, ns, agentSvc, hub)
		wg.Add(1)
		go func() {
			defer wg.Done()
			runNotifyPruneLoop(svcCtx, notifyService)
		}()
	}

	if notifyService != nil {
		svcNotify := notifyService
		addCloser(func() error {
			if !svcNotify.DrainDispatches(3 * time.Second) {
				log.Warn("notify: dispatch goroutines still running at shutdown")
			}
			return nil
		})
	}

	// Gateway manager: one adapter per enabled app instance in cfg.Apps,
	// built through the app plugin registry with vault-backed secrets.
	gwManager := buildGatewayManager(svcCtx, h, notifyService, secretStore, degraded, &wg)
	if gwManager != nil {
		// Registered after the notify closer so it runs BEFORE it (closers
		// run in reverse): adapters stop feeding messages, then in-flight
		// dispatches drain, then stores close.
		addCloser(func() error { gwManager.Stop(context.Background()); return nil })
	}

	// Provider registry is global but we keep it referenced for parity.
	_ = provider.DefaultRegistry

	svc := &Services{
		Home:        h,
		Agents:      agentSvc,
		AgentMgr:    agentMgr,
		EventLog:    eventLog,
		EventWriter: eventWriter,
		Costs:       costSvc,
		Secrets:     secretStore,
		MCP:         mcpStore,
		MCPGlobal:   globalMCPStore(globals),
		Tools:       toolStore,
		Templates:   tmplStore,
		Gateway:     gwManager,
		Notify:      notifyService,
		Hub:         hub,
		Degraded:    degraded,
		lifecycle:   &serviceLifecycle{cancel: svcCancel, wg: &wg},
	}

	// Propagate global-scoped stores onto the bundle so handlers can
	// reach them without a separate Globals reference.
	if globals != nil {
		svc.Stats = globals.Stats
		svc.Deps = globals.Deps
	}

	// Closer runs addCloser funcs in reverse order. cancel+wg.Wait are
	// handled by Services.Close() itself before invoking this.
	svc.lifecycle.closer = func() error {
		var firstErr error
		for i := len(closers) - 1; i >= 0; i-- {
			if err := closers[i](); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
	return svc, nil
}

// globalMCPStore returns the Globals.MCPGlobal pointer, or nil when
// globals itself is nil. Kept as a helper so the composite literal in
// BuildServices stays compact.
func globalMCPStore(g *Globals) *mcppkg.GlobalStore {
	if g == nil {
		return nil
	}
	return g.MCPGlobal
}

// MCPLayeredView returns a read-oriented composite of global + local MCP
// registries for the bundle. Callers use it to list / resolve servers
// with local overrides winning. Returns nil when neither layer is
// available.
func (s *Services) MCPLayeredView() *mcppkg.LayeredView {
	if s == nil {
		return nil
	}
	if s.MCPGlobal == nil && s.MCP == nil {
		return nil
	}
	return &mcppkg.LayeredView{Global: s.MCPGlobal, DB: s.MCP}
}

// newAgentManager mirrors the helper that used to live in serve.go.
// The third return value is a non-empty degradation reason when the
// docker runtime was expected but unavailable and agents silently fall
// back to tmux.
func newAgentManager(h *home.Home) (*agentpkg.Manager, *containerpkg.Backend, string, error) {
	var homeCfg home.DockerRuntimeConfig
	runtimeDefault := ""
	if h.Config != nil {
		homeCfg = h.Config.Runtime.Docker
		runtimeDefault = h.Config.Runtime.Default
	}
	dockerCfg := containerpkg.ConfigFromHome(homeCfg)
	be, err := containerpkg.NewBackend(dockerCfg, agentpkg.DefaultSessionPrefix, h.RootDir, provider.DefaultRegistry)
	if err != nil {
		log.Warn("Docker not available — agents will use tmux runtime only", "error", err, "repo", h.RootDir)
		reason := ""
		if runtimeDefault != "tmux" {
			// Only flag degradation when tmux was NOT the configured
			// runtime — an explicit tmux default is working as intended.
			reason = fmt.Sprintf("docker runtime unavailable — agents fall back to tmux: %v", err)
		}
		return agentpkg.NewManagerWithRepo(h.AgentsDir(), h.RootDir), nil, reason, nil
	}
	mgr := agentpkg.NewManagerWithRuntime(h.AgentsDir(), h.RootDir, be, "docker")
	return mgr, be, "", nil
}

// buildGatewayManager constructs the gateway.Manager and registers an
// adapter for every enabled app instance in the global "apps"
// config. Each instance resolves its plugin from the app registry,
// validates its config against the descriptor, resolves secret fields
// from the vault, and Builds the live adapter. Unknown apps, invalid
// config, and Build failures are recorded in degraded (key "app:<name>")
// and skipped so one broken integration never takes the daemon down.
//
// A manager is always returned (even with zero adapters) so:
//  1. health endpoints never report "gateway manager not available"
//     solely because nothing was configured at boot, and
//  2. POST /api/apps/{name} can hot-start adapters without a restart.
func buildGatewayManager(ctx context.Context, h *home.Home, notifyService *notifypkg.Service, vault *secretpkg.Store, degraded map[string]string, wg *sync.WaitGroup) *gatewaypkg.Manager {
	m := gatewaypkg.NewManager()
	m.SetStartContext(ctx)
	if notifyService != nil {
		m.SetChannelStore(&channelPersister{store: notifyService.Store()})
	}

	for name, ic := range h.Config.Apps {
		if !ic.Enabled {
			continue
		}
		plugin, ok := apppkg.Get(ic.App)
		if !ok {
			degraded["app:"+name] = fmt.Sprintf("unknown app %q", ic.App)
			continue
		}
		if err := apppkg.ValidateConfig(plugin.Describe(), ic.Config); err != nil {
			degraded["app:"+name] = "invalid config: " + err.Error()
			continue
		}
		var secrets apppkg.SecretSource
		if vault != nil {
			secrets = apppkg.VaultSecrets{Store: vault, Instance: name}
		}
		inst := apppkg.ResolveInstance(name, ic, secrets)
		adapter, err := plugin.Build(inst, apppkg.Env{StateDir: appStateDir(h, name)})
		if err != nil {
			degraded["app:"+name] = "build failed: " + err.Error()
			continue
		}
		m.Register(adapter)
		log.Info("gateway: app adapter registered", "name", name, "app", ic.App)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := m.Start(ctx); err != nil && ctx.Err() == nil {
			log.Error("gateway manager stopped", "error", err)
		}
	}()
	return m
}

// appStateDir returns the per-instance state directory for stateful apps
// (WhatsApp session DB, caches): <state>/apps/<instance-name>/.
func appStateDir(h *home.Home, instance string) string {
	return filepath.Join(h.StateDir(), "apps", instance)
}

// eventPruneMaxPerAgent caps retained events per agent. 5,000 was too
// small — a busy agent burned through it in about an hour and the Live
// page lost history (#3279). The 24h TTL is the real bound; the cap is
// a safety net against runaway writers.
const eventPruneMaxPerAgent = 50000

// runEventPruneLoop prunes stale events (TTL 24h, max
// eventPruneMaxPerAgent per agent) every hour.
func runEventPruneLoop(ctx context.Context, prunable *eventspkg.SQLiteLog) {
	if n, err := prunable.Prune(24*time.Hour, eventPruneMaxPerAgent); err != nil {
		log.Warn("event prune failed", "error", err)
	} else if n > 0 {
		log.Info("event prune: deleted stale events", "count", n)
	}
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if n, err := prunable.Prune(24*time.Hour, eventPruneMaxPerAgent); err != nil {
				log.Warn("event prune failed", "error", err)
			} else if n > 0 {
				log.Info("event prune: deleted stale events", "count", n)
			}
		case <-ctx.Done():
			return
		}
	}
}

// runNotifyPruneLoop keeps the last 1000 delivery-log entries per channel.
func runNotifyPruneLoop(ctx context.Context, svc *notifypkg.Service) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := svc.PruneOldActivity(ctx, 1000); err != nil {
				log.Warn("notify: periodic prune failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// channelPersister bridges notify.Store → gateway.ChannelStore.
type channelPersister struct {
	store *notifypkg.Store
}

func (p *channelPersister) SaveChannel(ctx context.Context, channel, platform, platformID string) error {
	return p.store.SaveChannel(ctx, channel, platform, platformID)
}

func (p *channelPersister) UpdateChannelPlatformID(ctx context.Context, channel, platformID string) error {
	return p.store.UpdateChannelPlatformID(ctx, channel, platformID)
}

func (p *channelPersister) LoadChannels(ctx context.Context) ([]gatewaypkg.PersistedChannel, error) {
	ncs, err := p.store.LoadChannels(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]gatewaypkg.PersistedChannel, len(ncs))
	for i, c := range ncs {
		result[i] = gatewaypkg.PersistedChannel{
			Channel:          c.Channel,
			Platform:         c.Platform,
			PlatformID:       c.PlatformID,
			DisplayName:      c.DisplayName,
			Kind:             c.Kind,
			AvatarURL:        c.AvatarURL,
			ParticipantCount: c.ParticipantCount,
		}
	}
	return result, nil
}

func (p *channelPersister) UpsertChannelMeta(ctx context.Context, channel, displayName, kind, avatarURL string, participantCount int) error {
	return p.store.UpsertChannelMeta(ctx, channel, displayName, kind, avatarURL, participantCount)
}

// costServiceAdapter bridges cost.Service → agentpkg.CostQuerier.
type costServiceAdapter struct {
	svc *cost.Service
}

func (a *costServiceAdapter) AgentCostSummary(agentID string) (*agentpkg.CostSummary, error) {
	sum, err := a.svc.AgentSummary(context.Background(), agentID)
	if err != nil {
		return nil, err
	}
	return &agentpkg.CostSummary{
		AgentID:      sum.AgentID,
		InputTokens:  sum.InputTokens,
		OutputTokens: sum.OutputTokens,
		TotalTokens:  sum.TotalTokens,
		TotalCostUSD: sum.TotalCostUSD,
		RequestCount: sum.RecordCount,
	}, nil
}

// prefsBudgetStore persists budget thresholds in the global prefs
// (~/.mycel/prefs.json) via the global config.
type prefsBudgetStore struct {
	h  *home.Home
	mu sync.Mutex
}

func (p *prefsBudgetStore) All() (map[string]cost.BudgetConfig, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]cost.BudgetConfig, len(p.h.Config.Budgets))
	for scope, cfg := range p.h.Config.Budgets {
		out[scope] = cfg
	}
	return out, nil
}

func (p *prefsBudgetStore) Set(scope string, b cost.BudgetConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.h.Config.Budgets == nil {
		p.h.Config.Budgets = map[string]cost.BudgetConfig{}
	}
	p.h.Config.Budgets[scope] = b
	return p.h.Save()
}

func (p *prefsBudgetStore) Delete(scope string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.h.Config.Budgets[scope]; !ok {
		return fmt.Errorf("budget not found for scope %q", scope)
	}
	delete(p.h.Config.Budgets, scope)
	return p.h.Save()
}
