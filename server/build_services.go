// build_services.go — factory for the single bcd service bundle.
//
// bcd is single-tenant: one Services bundle is built at boot against the
// one global database (db.Global) and lives for the process lifetime.
// A single call to BuildServices(ctx, globals, wsRoot) produces the
// fully-initialized bundle including background goroutines. Its Close()
// cancels those goroutines and closes each store.
package server

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	bcagent "github.com/rpuneet/mycel/pkg/agent"
	bccontainer "github.com/rpuneet/mycel/pkg/container"
	"github.com/rpuneet/mycel/pkg/cost"
	bcdb "github.com/rpuneet/mycel/pkg/db"
	bcdeps "github.com/rpuneet/mycel/pkg/deps"
	bcevents "github.com/rpuneet/mycel/pkg/events"
	bcgateway "github.com/rpuneet/mycel/pkg/gateway"
	bcdiscord "github.com/rpuneet/mycel/pkg/gateway/discord"
	bcgithub "github.com/rpuneet/mycel/pkg/gateway/github"
	bcimessage "github.com/rpuneet/mycel/pkg/gateway/imessage"
	bcirc "github.com/rpuneet/mycel/pkg/gateway/irc"
	bcmatrix "github.com/rpuneet/mycel/pkg/gateway/matrix"
	bcmattermost "github.com/rpuneet/mycel/pkg/gateway/mattermost"
	bcmqtt "github.com/rpuneet/mycel/pkg/gateway/mqtt"
	bcnotion "github.com/rpuneet/mycel/pkg/gateway/notion"
	bcreddit "github.com/rpuneet/mycel/pkg/gateway/reddit"
	bcrss "github.com/rpuneet/mycel/pkg/gateway/rss"
	bcsignal "github.com/rpuneet/mycel/pkg/gateway/signal"
	bcslack "github.com/rpuneet/mycel/pkg/gateway/slack"
	bctelegram "github.com/rpuneet/mycel/pkg/gateway/telegram"
	bctwitter "github.com/rpuneet/mycel/pkg/gateway/twitter"
	bcwebhook "github.com/rpuneet/mycel/pkg/gateway/webhook"
	bcwhatsapp "github.com/rpuneet/mycel/pkg/gateway/whatsapp"
	"github.com/rpuneet/mycel/pkg/log"
	bcmcp "github.com/rpuneet/mycel/pkg/mcp"
	bcnotify "github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/provider"
	bcsecret "github.com/rpuneet/mycel/pkg/secret"
	bcstats "github.com/rpuneet/mycel/pkg/stats"
	bctemplate "github.com/rpuneet/mycel/pkg/template"
	bctool "github.com/rpuneet/mycel/pkg/tool"
	bcworkspace "github.com/rpuneet/mycel/pkg/workspace"
	bcws "github.com/rpuneet/mycel/server/ws"
)

// Globals holds dependencies that are process-wide and independent of the
// bundle's anchor repo. bcd builds one Globals at boot and hands it to
// BuildServices exactly once.
type Globals struct {
	Stats        *bcstats.Store     // nil when TSDB unavailable
	Deps         *bcdeps.Registry   // optional dependencies registry (bc-db, etc.)
	Hub          *bcws.Hub          // the one SSE hub for /api/events (owned by the caller)
	Templates    *bctemplate.Store  // user-global template store (~/.mycel/templates/)
	SecretsVault *bcsecret.Store    // user-global secrets vault (~/.mycel/secrets.vault)
	MCPGlobal    *bcmcp.GlobalStore // user-global MCP registry (~/.mycel/mcps.json)
	CostsGlobal  *cost.Store        // user-global cost ledger — shared process-wide
	Build        BuildInfo
}

// BuildServices constructs the single fully-initialized Services bundle
// anchored at wsRoot (the repo bcd was booted against — new agents default
// their repo to it). All background goroutines are started under an
// internal context that Close() cancels.
//
// The returned *Services has its closer field set to a function that stops
// goroutines and closes stores. The caller (RunServer) invokes Close() at
// shutdown.
func BuildServices(ctx context.Context, globals *Globals, wsRoot string) (*Services, error) {
	ws, err := bcworkspace.Load(wsRoot)
	if err != nil {
		ws, err = bcworkspace.Init(wsRoot)
		if err != nil {
			return nil, fmt.Errorf("init workspace %s: %w", wsRoot, err)
		}
	}
	return buildServicesFromWS(ctx, globals, ws)
}

// buildServicesFromWS is the inner factory used when the caller already
// holds a loaded *workspace.Workspace.
//
//nolint:gocyclo // Linear dependency chain; splitting obscures the flow.
func buildServicesFromWS(ctx context.Context, globals *Globals, ws *bcworkspace.Workspace) (*Services, error) {
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

	// Single global database: every workspace's stores share the one
	// mycel.db at <MycelHome>/mycel.db (or the global TimescaleDB).
	// Isolation between repos comes from data keys (agent name, repo
	// path), not from separate files. The handle is cached process-wide
	// and stays open across service eviction; stores borrow it and
	// never close it.
	wsDB, wsDriver, dbErr := bcdb.Global(ws.Config.DBStorageSettings())
	if dbErr != nil {
		log.Warn("global database unavailable", "error", dbErr, "workspace", ws.RootDir)
		degraded["storage"] = "global database unavailable: " + dbErr.Error()
	}

	// Storage driver mismatch: pkg/db falls back to SQLite when the
	// configured TimescaleDB is unreachable (db.OpenGlobalDBWithConfig
	// logs "falling back to sqlite"). Compare the configured default
	// against the driver actually in use so the mismatch is visible.
	if ws.Config != nil && dbErr == nil {
		configured := ws.Config.Storage.Default
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

	// Events JSONL writer (append-only).
	eventsJSONL := filepath.Join(ws.StateDir(), "events.jsonl")
	eventWriter := bcevents.NewJSONLWriter(eventsJSONL, 0)

	// The one SSE hub. bcd is single-tenant, so the bundle publishes
	// straight into the process-wide hub supplied via Globals (owned by
	// the caller — no closer). Legacy callers/tests that don't wire a
	// hub get a private one that the closer tears down.
	var hub *bcws.Hub
	if globals != nil && globals.Hub != nil {
		hub = globals.Hub
	} else {
		hub = bcws.NewHub()
		go hub.Run()
		addCloser(func() error { hub.Stop(); return nil })
	}

	// Agent manager + service.
	agentMgr, containerBackend, runtimeReason, agentErr := newAgentManager(ws)
	if agentErr != nil {
		svcCancel()
		return nil, fmt.Errorf("agent manager: %w", agentErr)
	}
	if runtimeReason != "" {
		degraded["runtime"] = runtimeReason
	}
	if err := agentMgr.LoadState(); err != nil {
		log.Warn("failed to load agent state", "error", err, "workspace", ws.RootDir)
	}
	if ws.RoleManager != nil {
		agentMgr.SetRoleManager(ws.RoleManager)
	}
	if ws.Config != nil {
		agentMgr.ApplyWorkspaceConfig(ws.Config)
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
	agentMgr.StartToolHealthLoop(svcCtx, bcagent.DefaultToolHealthInterval)
	addCloser(func() error { agentMgr.StopToolHealthLoop(); return nil })

	// Cost store + importer. Prefer the user-global ledger at
	// ~/.mycel/costs.db when Globals.CostsGlobal is supplied — every
	// record is tagged with the repo path via ScopedStore /
	// Importer.SetRepo so cross-repo analytics work out of the box.
	// Fall back to the per-workspace store for legacy callers / tests
	// that assemble Globals by hand without the global ledger.
	var costStore *cost.Store
	var costImporter *cost.Importer
	if globals != nil && globals.CostsGlobal != nil {
		costStore = globals.CostsGlobal
		// Ownership stays with whoever populated Globals; no closer.
		costImporter = cost.NewImporter(costStore, ws.RootDir)
		costImporter.SetRepo(ws.RootDir)
		wg.Add(1)
		go func() {
			defer wg.Done()
			runCostImportLoop(svcCtx, costImporter)
		}()
	} else if cs, err := cost.OpenStore(wsDB, wsDriver, ws.RootDir); err != nil {
		log.Warn("cost store unavailable", "error", err, "workspace", ws.RootDir)
		degraded["costs"] = "cost store unavailable: " + err.Error()
	} else {
		costStore = cs
		addCloser(func() error { return cs.Close() })

		costImporter = cost.NewImporter(cs, ws.RootDir)
		wg.Add(1)
		go func() {
			defer wg.Done()
			runCostImportLoop(svcCtx, costImporter)
		}()
	}

	// Wire cost querier into agent service (after cost store is initialized).
	var costQuerier bcagent.CostQuerier
	if costStore != nil {
		costQuerier = &costStoreAdapter{store: costStore}
	}
	agentSvc := bcagent.NewAgentService(agentMgr, hub, costQuerier)

	// Secret store. Prefer the user-global vault (~/.mycel/secrets.vault)
	// supplied by Globals so a single secret set once is visible across
	// every workspace. When Globals.SecretsVault is unset (legacy
	// callers), fall back to the per-workspace <ws>/.bc/secrets.db.
	var secretStore *bcsecret.Store
	if globals != nil && globals.SecretsVault != nil {
		secretStore = globals.SecretsVault
		// Don't register a closer: ownership stays with whoever
		// populated Globals (typically RunServer).
	} else if passphrase, passErr := bcsecret.Passphrase(); passErr != nil {
		log.Warn("secret passphrase unavailable — secret store disabled", "error", passErr)
		degraded["secrets"] = "secret passphrase unavailable: " + passErr.Error()
	} else if ss, err := bcsecret.NewStore(ws.RootDir, passphrase); err != nil {
		log.Warn("secret store unavailable", "error", err, "workspace", ws.RootDir)
		degraded["secrets"] = "secret store unavailable: " + err.Error()
	} else {
		secretStore = ss
		addCloser(func() error { return ss.Close() })
	}

	// MCP store.
	var mcpStore *bcmcp.Store
	if ms, err := bcmcp.NewStore(wsDB, wsDriver); err != nil {
		log.Warn("mcp store unavailable", "error", err, "workspace", ws.RootDir)
		degraded["mcp"] = "mcp store unavailable: " + err.Error()
	} else {
		mcpStore = ms
		addCloser(func() error { return ms.Close() })
	}

	// Tool store.
	var toolStore *bctool.Store
	{
		ts := bctool.NewStore(wsDB, wsDriver)
		if err := ts.Open(); err != nil {
			log.Warn("tool store unavailable", "error", err, "workspace", ws.RootDir)
			degraded["tools"] = "tool store unavailable: " + err.Error()
		} else {
			toolStore = ts
			addCloser(func() error { return ts.Close() })
		}
	}

	// Template store: user-global (~/.mycel/templates/) with workspace
	// override. If globals.Templates is nil (legacy callers that did not
	// initialize it), fall back to a workspace-local single-layer store
	// so existing behavior is preserved.
	var tmplStore *bctemplate.Store
	wsTemplatesDir := filepath.Join(ws.StateDir(), "templates")
	if globals != nil && globals.Templates != nil {
		tmplStore = globals.Templates.WithOverride(wsTemplatesDir)
	} else {
		tmplStore = bctemplate.NewStore(wsTemplatesDir)
	}

	// Event log (SQLite) + pruning loop.
	var eventLog bcevents.EventStore
	if el, err := bcevents.OpenLog(wsDB, wsDriver); err != nil {
		log.Warn("event log unavailable", "error", err, "workspace", ws.RootDir)
		degraded["events"] = "event log unavailable: " + err.Error()
	} else {
		eventLog = el
		addCloser(func() error { return el.Close() })
		if prunable, ok := el.(*bcevents.SQLiteLog); ok {
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
	// globally. Uses the current workspace's agentSvc.
	if globals != nil && globals.Stats != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runStatsCollector(svcCtx, globals.Stats, agentSvc)
		}()
	}

	// Notify service (channel subscriptions + delivery).
	var notifyService *bcnotify.Service
	if ns, err := bcnotify.OpenStore(wsDB, wsDriver); err != nil {
		log.Warn("notify store unavailable", "error", err, "workspace", ws.RootDir)
		degraded["notify"] = "notify store unavailable: " + err.Error()
	} else {
		notifyService = bcnotify.NewServiceWithContext(svcCtx, ns, agentSvc, hub)
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

	// Gateway manager (Telegram/Discord/Slack adapters).
	gwManager := buildGatewayManager(svcCtx, ws, notifyService, &wg)
	if gwManager != nil {
		// Registered after the notify closer so it runs BEFORE it (closers
		// run in reverse): adapters stop feeding messages, then in-flight
		// dispatches drain, then stores close.
		addCloser(func() error { gwManager.Stop(context.Background()); return nil })
	}

	// Provider registry is global but we keep it referenced for parity.
	_ = provider.DefaultRegistry

	svc := &Services{
		WS:           ws,
		Agents:       agentSvc,
		AgentMgr:     agentMgr,
		EventLog:     eventLog,
		EventWriter:  eventWriter,
		Costs:        costStore,
		CostImporter: costImporter,
		Secrets:      secretStore,
		MCP:          mcpStore,
		MCPGlobal:    globalMCPStore(globals),
		Tools:        toolStore,
		Templates:    tmplStore,
		Gateway:      gwManager,
		Notify:       notifyService,
		Hub:          hub,
		Degraded:     degraded,
		lifecycle:    &serviceLifecycle{cancel: svcCancel, wg: &wg},
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
func globalMCPStore(g *Globals) *bcmcp.GlobalStore {
	if g == nil {
		return nil
	}
	return g.MCPGlobal
}

// MCPLayeredView returns a read-oriented composite of global + local MCP
// registries for the bundle. Callers use it to list / resolve servers
// with local overrides winning. Returns nil when neither layer is
// available.
func (s *Services) MCPLayeredView() *bcmcp.LayeredView {
	if s == nil {
		return nil
	}
	if s.MCPGlobal == nil && s.MCP == nil {
		return nil
	}
	return &bcmcp.LayeredView{Global: s.MCPGlobal, Workspace: s.MCP}
}

// newAgentManager mirrors the helper that used to live in serve.go.
// The third return value is a non-empty degradation reason when the
// docker runtime was expected but unavailable and agents silently fall
// back to tmux.
func newAgentManager(ws *bcworkspace.Workspace) (*bcagent.Manager, *bccontainer.Backend, string, error) {
	var wsCfg bcworkspace.DockerRuntimeConfig
	runtimeDefault := ""
	if ws.Config != nil {
		wsCfg = ws.Config.Runtime.Docker
		runtimeDefault = ws.Config.Runtime.Default
	}
	dockerCfg := bccontainer.ConfigFromWorkspace(wsCfg)
	be, err := bccontainer.NewBackend(dockerCfg, bcagent.DefaultSessionPrefix, ws.RootDir, provider.DefaultRegistry)
	if err != nil {
		log.Warn("Docker not available — agents will use tmux runtime only", "error", err, "workspace", ws.RootDir)
		reason := ""
		if runtimeDefault != "tmux" {
			// Only flag degradation when tmux was NOT the configured
			// runtime — an explicit tmux workspace is working as intended.
			reason = fmt.Sprintf("docker runtime unavailable — agents fall back to tmux: %v", err)
		}
		return bcagent.NewWorkspaceManager(ws.AgentsDir(), ws.RootDir), nil, reason, nil
	}
	mgr := bcagent.NewWorkspaceManagerWithRuntime(ws.AgentsDir(), ws.RootDir, be, "docker")
	return mgr, be, "", nil
}

// buildGatewayManager constructs the gateway.Manager from workspace config
// and registers adapters for any enabled platforms. Returns nil if no
// adapters are enabled.
func buildGatewayManager(ctx context.Context, ws *bcworkspace.Workspace, notifyService *bcnotify.Service, wg *sync.WaitGroup) *bcgateway.Manager {
	gw := ws.Config.Gateways

	dcEnabled := gw.Discord != nil && gw.Discord.Enabled && gw.Discord.BotToken != ""
	slEnabled := gw.Slack != nil && gw.Slack.Enabled && gw.Slack.BotToken != "" && gw.Slack.AppToken != ""

	// Count enabled Telegram bots from the Telegrams map.
	var tgCount int
	for _, tc := range gw.Telegrams {
		if tc.Enabled && tc.BotToken != "" {
			tgCount++
		}
	}

	// Count enabled GitHub webhook adapters.
	var ghCount int
	for _, gc := range gw.GitHubs {
		if gc.Enabled {
			ghCount++
		}
	}

	// Count enabled generic webhook adapters.
	var whCount int
	for _, wc := range gw.Webhooks {
		if wc.Enabled {
			whCount++
		}
	}

	// Count enabled RSS feed adapters.
	var rssCount int
	for _, rc := range gw.RSSFeeds {
		if rc.Enabled && rc.URL != "" {
			rssCount++
		}
	}

	// Count enabled Notion poll adapters.
	var notionCount int
	for _, c := range gw.Notions {
		if c.Enabled && c.Token != "" {
			notionCount++
		}
	}

	// Count enabled WhatsApp webhook adapters.
	var waCount int
	for _, c := range gw.WhatsApps {
		if c.Enabled {
			waCount++
		}
	}

	// Count enabled Matrix poll adapters.
	var matrixCount int
	for _, c := range gw.Matrices {
		if c.Enabled && c.Token != "" {
			matrixCount++
		}
	}

	// Count enabled Mattermost webhook adapters.
	var mmCount int
	for _, c := range gw.Mattermosts {
		if c.Enabled {
			mmCount++
		}
	}

	// Count enabled IRC socket adapters.
	var ircCount int
	for _, c := range gw.IRCs {
		if c.Enabled {
			ircCount++
		}
	}

	// Count enabled MQTT socket adapters.
	var mqttCount int
	for _, c := range gw.MQTTs {
		if c.Enabled {
			mqttCount++
		}
	}

	// Count enabled Twitter poll adapters.
	var twitterCount int
	for _, c := range gw.Twitters {
		if c.Enabled && c.BearerToken != "" {
			twitterCount++
		}
	}

	// Count enabled Reddit poll adapters.
	var redditCount int
	for _, c := range gw.Reddits {
		if c.Enabled && c.BearerToken != "" {
			redditCount++
		}
	}

	// Count enabled Signal poll adapters.
	var signalCount int
	for _, c := range gw.Signals {
		if c.Enabled && c.APIURL != "" {
			signalCount++
		}
	}

	// Count enabled iMessage poll adapters.
	var imessageCount int
	for _, c := range gw.IMessages {
		if c.Enabled && c.APIURL != "" {
			imessageCount++
		}
	}

	// Always construct a manager (even with zero adapters) so:
	//  1. health endpoints never return "gateway manager not available" solely
	//     because nothing was configured at boot, and
	//  2. PATCH /api/gateways/{platform} can hot-start adapters without a
	//     daemon restart (see GatewayHandler.ensureTelegramAdapter).
	m := bcgateway.NewManager()
	m.SetStartContext(ctx)
	if notifyService != nil {
		m.SetChannelStore(&channelPersister{store: notifyService.Store()})
	}

	if tgCount == 0 && !dcEnabled && !slEnabled && ghCount == 0 && whCount == 0 && rssCount == 0 &&
		notionCount == 0 && waCount == 0 && matrixCount == 0 &&
		mmCount == 0 && ircCount == 0 && mqttCount == 0 && twitterCount == 0 && redditCount == 0 &&
		signalCount == 0 && imessageCount == 0 {
		// Empty manager still needs Start so StartAdapter can share the
		// process lifetime context.
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.Start(ctx); err != nil && ctx.Err() == nil {
				log.Error("gateway manager stopped", "error", err)
			}
		}()
		return m
	}

	// Register Telegram adapters. Label "" → adapter name "telegram",
	// label "foo" → adapter name "telegram:foo".
	for label, tc := range gw.Telegrams {
		if !tc.Enabled || tc.BotToken == "" {
			continue
		}
		adapterName := "telegram"
		if label != "" {
			adapterName = "telegram:" + label
		}
		tgAdapter := bctelegram.NewNamed(adapterName, tc.BotToken, tc.Mode)
		if err := tgAdapter.DiscoverViaUpdate(); err != nil {
			log.Warn("telegram: discovery failed", "adapter", adapterName, "error", err)
		}
		m.Register(tgAdapter)
		log.Info("gateway: telegram adapter registered", "name", adapterName)
	}
	if dcEnabled {
		m.Register(bcdiscord.New(gw.Discord.BotToken))
		log.Info("gateway: discord adapter registered")
	}
	if slEnabled {
		m.Register(bcslack.New(gw.Slack.BotToken, gw.Slack.AppToken))
		log.Info("gateway: slack adapter registered")
	}

	// Register GitHub webhook adapters. Label "" → adapter name "github",
	// label "bc" → adapter name "github:bc".
	for label, gc := range gw.GitHubs {
		if !gc.Enabled {
			continue
		}
		adapterName := "github"
		if label != "" {
			adapterName = "github:" + label
		}
		m.Register(bcgithub.NewNamed(adapterName, gc.Secret))
		log.Info("gateway: github adapter registered", "name", adapterName)
	}

	// Register generic webhook adapters. Label "" → adapter name "webhook",
	// label "deploy" → adapter name "webhook:deploy".
	for label, wc := range gw.Webhooks {
		if !wc.Enabled {
			continue
		}
		adapterName := "webhook"
		if label != "" {
			adapterName = "webhook:" + label
		}
		m.Register(bcwebhook.NewWithSecret(adapterName, wc.Secret))
		log.Info("gateway: webhook adapter registered", "name", adapterName)
	}

	// Register RSS feed adapters. Label "" → adapter name "rss",
	// label "blog" → adapter name "rss:blog".
	for label, rc := range gw.RSSFeeds {
		if !rc.Enabled || rc.URL == "" {
			continue
		}
		adapterName := "rss"
		if label != "" {
			adapterName = "rss:" + label
		}
		m.Register(bcrss.NewNamed(adapterName, rc.URL, rc.Interval))
		log.Info("gateway: rss adapter registered", "name", adapterName, "url", rc.URL)
	}

	// Register Notion poll adapters.
	for label, c := range gw.Notions {
		if !c.Enabled || c.Token == "" {
			continue
		}
		adapterName := "notion"
		if label != "" {
			adapterName = "notion:" + label
		}
		m.Register(bcnotion.NewNamed(adapterName, c.Token, c.Interval))
		log.Info("gateway: notion adapter registered", "name", adapterName)
	}

	// Register WhatsApp webhook adapters.
	for label, c := range gw.WhatsApps {
		if !c.Enabled {
			continue
		}
		adapterName := "whatsapp"
		if label != "" {
			adapterName = "whatsapp:" + label
		}
		waStateDir := filepath.Join(ws.StateDir(), "gateways", adapterName)
		wa := bcwhatsapp.NewNamed(adapterName, waStateDir)
		wa.SetIncludeSelfMessages(c.IncludeSelfMessages)
		m.Register(wa)
		log.Info("gateway: whatsapp adapter registered", "name", adapterName, "include_self", c.IncludeSelfMessages)
	}

	// Register Matrix poll adapters.
	for label, c := range gw.Matrices {
		if !c.Enabled || c.Token == "" {
			continue
		}
		adapterName := "matrix"
		if label != "" {
			adapterName = "matrix:" + label
		}
		m.Register(bcmatrix.NewNamed(adapterName, c.Homeserver, c.Token, c.Interval))
		log.Info("gateway: matrix adapter registered", "name", adapterName)
	}

	// Register Mattermost webhook adapters.
	for label, c := range gw.Mattermosts {
		if !c.Enabled {
			continue
		}
		adapterName := "mattermost"
		if label != "" {
			adapterName = "mattermost:" + label
		}
		m.Register(bcmattermost.New(adapterName, bcmattermost.Config{
			URL:   c.URL,
			Token: c.Token,
		}))
		log.Info("gateway: mattermost adapter registered", "name", adapterName)
	}

	// Register IRC socket adapters.
	for label, c := range gw.IRCs {
		if !c.Enabled {
			continue
		}
		adapterName := "irc"
		if label != "" {
			adapterName = "irc:" + label
		}
		m.Register(bcirc.New(adapterName, bcirc.Config{
			Server:   c.Server,
			Nick:     "bc-bot",
			Channels: c.Channels,
			UseTLS:   true,
		}))
		log.Info("gateway: irc adapter registered", "name", adapterName)
	}

	// Register MQTT socket adapters.
	for label, c := range gw.MQTTs {
		if !c.Enabled {
			continue
		}
		adapterName := "mqtt"
		if label != "" {
			adapterName = "mqtt:" + label
		}
		topics := []string{c.Topic}
		if c.Topic == "" {
			topics = []string{"#"}
		}
		m.Register(bcmqtt.New(adapterName, bcmqtt.Config{
			Broker:   c.BrokerURL,
			ClientID: "bc-" + adapterName,
			Topics:   topics,
		}))
		log.Info("gateway: mqtt adapter registered", "name", adapterName)
	}

	// Register Twitter poll adapters.
	for label, c := range gw.Twitters {
		if !c.Enabled || c.BearerToken == "" {
			continue
		}
		adapterName := "twitter"
		if label != "" {
			adapterName = "twitter:" + label
		}
		m.Register(bctwitter.NewNamed(adapterName, c.BearerToken, c.UserID, c.Interval))
		log.Info("gateway: twitter adapter registered", "name", adapterName)
	}

	// Register Signal poll adapters.
	for label, c := range gw.Signals {
		if !c.Enabled || c.APIURL == "" {
			continue
		}
		adapterName := "signal"
		if label != "" {
			adapterName = "signal:" + label
		}
		m.Register(bcsignal.NewNamed(adapterName, c.APIURL, c.Interval))
		log.Info("gateway: signal adapter registered", "name", adapterName)
	}

	// Register iMessage poll adapters.
	for label, c := range gw.IMessages {
		if !c.Enabled || c.APIURL == "" {
			continue
		}
		adapterName := "imessage"
		if label != "" {
			adapterName = "imessage:" + label
		}
		m.Register(bcimessage.NewNamed(adapterName, c.APIURL, c.Password, c.Interval))
		log.Info("gateway: imessage adapter registered", "name", adapterName)
	}

	// Register Reddit poll adapters.
	for label, c := range gw.Reddits {
		if !c.Enabled || c.BearerToken == "" {
			continue
		}
		adapterName := "reddit"
		if label != "" {
			adapterName = "reddit:" + label
		}
		m.Register(bcreddit.NewNamed(adapterName, c.Subreddit, c.BearerToken, c.Interval))
		log.Info("gateway: reddit adapter registered", "name", adapterName)
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

// runCostImportLoop drains the cost importer once immediately, then every
// 5 minutes until ctx is canceled.
func runCostImportLoop(ctx context.Context, imp *cost.Importer) {
	if n, err := imp.ImportAll(ctx); err != nil {
		log.Warn("cost import failed", "error", err)
	} else if n > 0 {
		log.Info("cost import: imported records", "count", n)
	}
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if n, err := imp.ImportAll(ctx); err != nil {
				log.Warn("cost import failed", "error", err)
			} else if n > 0 {
				log.Info("cost import: imported records", "count", n)
			}
		case <-ctx.Done():
			return
		}
	}
}

// eventPruneMaxPerAgent caps retained events per agent. 5,000 was too
// small — a busy agent burned through it in about an hour and the Live
// page lost history (#3279). The 24h TTL is the real bound; the cap is
// a safety net against runaway writers.
const eventPruneMaxPerAgent = 50000

// runEventPruneLoop prunes stale events (TTL 24h, max
// eventPruneMaxPerAgent per agent) every hour.
func runEventPruneLoop(ctx context.Context, prunable *bcevents.SQLiteLog) {
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
func runNotifyPruneLoop(ctx context.Context, svc *bcnotify.Service) {
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
	store *bcnotify.Store
}

func (p *channelPersister) SaveChannel(ctx context.Context, bcChannel, platform, platformID string) error {
	return p.store.SaveChannel(ctx, bcChannel, platform, platformID)
}

func (p *channelPersister) UpdateChannelPlatformID(ctx context.Context, bcChannel, platformID string) error {
	return p.store.UpdateChannelPlatformID(ctx, bcChannel, platformID)
}

func (p *channelPersister) LoadChannels(ctx context.Context) ([]bcgateway.PersistedChannel, error) {
	ncs, err := p.store.LoadChannels(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]bcgateway.PersistedChannel, len(ncs))
	for i, c := range ncs {
		result[i] = bcgateway.PersistedChannel{
			BCChannel:        c.BCChannel,
			Platform:         c.Platform,
			PlatformID:       c.PlatformID,
			DisplayName:      c.DisplayName,
			Kind:             c.Kind,
			ParticipantCount: c.ParticipantCount,
		}
	}
	return result, nil
}

func (p *channelPersister) UpsertChannelMeta(ctx context.Context, bcChannel, displayName, kind string, participantCount int) error {
	return p.store.UpsertChannelMeta(ctx, bcChannel, displayName, kind, participantCount)
}

// costStoreAdapter bridges cost.Store → bcagent.CostQuerier.
type costStoreAdapter struct {
	store *cost.Store
}

func (a *costStoreAdapter) AgentCostSummary(agentID string) (*bcagent.CostSummary, error) {
	sum, err := a.store.AgentSummary(context.Background(), agentID)
	if err != nil {
		return nil, err
	}
	return &bcagent.CostSummary{
		AgentID:      sum.AgentID,
		InputTokens:  sum.InputTokens,
		OutputTokens: sum.OutputTokens,
		TotalTokens:  sum.TotalTokens,
		TotalCostUSD: sum.TotalCostUSD,
		RequestCount: sum.RecordCount,
	}, nil
}
