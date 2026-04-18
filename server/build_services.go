// build_services.go — factory for per-workspace WorkspaceServices.
//
// Extracted from internal/cmd/serve.go as part of multi-tenant bcd phase M2.
// A single call to BuildWorkspaceServices(ctx, globals, wsRoot) produces a
// fully-initialized WorkspaceServices bundle including background
// goroutines. Its Close() cancels those goroutines and closes each store.
//
// The factory depends ONLY on Globals + a workspace root path, so it can
// be invoked at any time for any registered workspace — which is the
// substrate for multi-workspace dispatch (phases M5-M6).
package server

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bcagent "github.com/rpuneet/bc/pkg/agent"
	bccontainer "github.com/rpuneet/bc/pkg/container"
	"github.com/rpuneet/bc/pkg/cost"
	"github.com/rpuneet/bc/pkg/cron"
	bcdeps "github.com/rpuneet/bc/pkg/deps"
	bcevents "github.com/rpuneet/bc/pkg/events"
	bcgateway "github.com/rpuneet/bc/pkg/gateway"
	bcdiscord "github.com/rpuneet/bc/pkg/gateway/discord"
	bcgithub "github.com/rpuneet/bc/pkg/gateway/github"
	bcslack "github.com/rpuneet/bc/pkg/gateway/slack"
	bctelegram "github.com/rpuneet/bc/pkg/gateway/telegram"
	bcgitlab "github.com/rpuneet/bc/pkg/gateway/gitlab"
	bcjira "github.com/rpuneet/bc/pkg/gateway/jira"
	bclinear "github.com/rpuneet/bc/pkg/gateway/linear"
	bcrss "github.com/rpuneet/bc/pkg/gateway/rss"
	bcsentry "github.com/rpuneet/bc/pkg/gateway/sentry"
	bcstripe "github.com/rpuneet/bc/pkg/gateway/stripe"
	bcbitbucket "github.com/rpuneet/bc/pkg/gateway/bitbucket"
	bcpagerduty "github.com/rpuneet/bc/pkg/gateway/pagerduty"
	bcdatadog "github.com/rpuneet/bc/pkg/gateway/datadog"
	bcgrafana "github.com/rpuneet/bc/pkg/gateway/grafana"
	bcvercel "github.com/rpuneet/bc/pkg/gateway/vercel"
	bcnetlify "github.com/rpuneet/bc/pkg/gateway/netlify"
	bcnotion "github.com/rpuneet/bc/pkg/gateway/notion"
	bcwebhook "github.com/rpuneet/bc/pkg/gateway/webhook"
	bcwhatsapp "github.com/rpuneet/bc/pkg/gateway/whatsapp"
	bcsignal "github.com/rpuneet/bc/pkg/gateway/signal"
	bcmatrix "github.com/rpuneet/bc/pkg/gateway/matrix"
	bcmsteams "github.com/rpuneet/bc/pkg/gateway/msteams"
	bcgooglechat "github.com/rpuneet/bc/pkg/gateway/googlechat"
	bcline "github.com/rpuneet/bc/pkg/gateway/line"
	bcfeishu "github.com/rpuneet/bc/pkg/gateway/feishu"
	bcmattermost "github.com/rpuneet/bc/pkg/gateway/mattermost"
	bcirc "github.com/rpuneet/bc/pkg/gateway/irc"
	bcnostr "github.com/rpuneet/bc/pkg/gateway/nostr"
	bctwitch "github.com/rpuneet/bc/pkg/gateway/twitch"
	bcimessage "github.com/rpuneet/bc/pkg/gateway/imessage"
	bcmqtt "github.com/rpuneet/bc/pkg/gateway/mqtt"
	bctwitter "github.com/rpuneet/bc/pkg/gateway/twitter"
	bcreddit "github.com/rpuneet/bc/pkg/gateway/reddit"
	bchomeassistant "github.com/rpuneet/bc/pkg/gateway/homeassistant"
	"github.com/rpuneet/bc/pkg/log"
	bcmcp "github.com/rpuneet/bc/pkg/mcp"
	bcnotify "github.com/rpuneet/bc/pkg/notify"
	"github.com/rpuneet/bc/pkg/provider"
	bcsecret "github.com/rpuneet/bc/pkg/secret"
	bcstats "github.com/rpuneet/bc/pkg/stats"
	bctemplate "github.com/rpuneet/bc/pkg/template"
	bctool "github.com/rpuneet/bc/pkg/tool"
	bcworkspace "github.com/rpuneet/bc/pkg/workspace"
	bcws "github.com/rpuneet/bc/server/ws"
)

// Globals holds dependencies that are truly workspace-agnostic and shared
// across all per-workspace services. bcd builds one Globals at boot and
// reuses it for every workspace the WorkspaceManager materializes.
type Globals struct {
	Registry     *bcworkspace.Registry
	Stats        *bcstats.Store     // nil when TSDB unavailable
	Deps         *bcdeps.Registry   // optional dependencies registry (bc-db, etc.)
	GlobalHub    *bcws.Hub          // fan-in SSE hub for cross-workspace /api/events
	Templates    *bctemplate.Store  // user-global template store (~/.bc/templates/) — wrapped per-workspace
	SecretsVault *bcsecret.Store    // user-global secrets vault (~/.bc/secrets.vault) — shared across workspaces
	MCPGlobal    *bcmcp.GlobalStore // user-global MCP registry (~/.bc/mcps.json)
	CostsGlobal  *cost.Store        // user-global cost ledger (~/.bc/costs.db) — shared across workspaces
	Build        BuildInfo
}

// BuildWorkspaceServices constructs a fully-initialized WorkspaceServices
// for the workspace rooted at wsRoot. All background goroutines are
// started under an internal context that Close() cancels.
//
// The returned *WorkspaceServices has its closer field set to a function
// that stops goroutines and closes stores. The caller (WorkspaceManager)
// will invoke Close() on eviction / shutdown.
func BuildWorkspaceServices(ctx context.Context, globals *Globals, wsRoot string) (*WorkspaceServices, error) {
	ws, err := bcworkspace.Load(wsRoot)
	if err != nil {
		ws, err = bcworkspace.Init(wsRoot)
		if err != nil {
			return nil, fmt.Errorf("init workspace %s: %w", wsRoot, err)
		}
	}
	return buildWorkspaceServicesFromWS(ctx, globals, ws)
}

// buildWorkspaceServicesFromWS is the inner factory used when callers
// already hold a loaded *workspace.Workspace (e.g. from the registry).
//
//nolint:gocyclo // Linear dependency chain; splitting obscures the flow.
func buildWorkspaceServicesFromWS(ctx context.Context, globals *Globals, ws *bcworkspace.Workspace) (*WorkspaceServices, error) {
	// Child context + waitgroup so Close() can stop every goroutine spawned
	// below and wait for them to exit.
	svcCtx, svcCancel := context.WithCancel(ctx)
	var wg sync.WaitGroup

	// Track cleanup actions; closer invokes them in reverse order.
	var closers []func() error
	addCloser := func(f func() error) { closers = append(closers, f) }

	// Events JSONL writer (append-only).
	eventsJSONL := filepath.Join(ws.StateDir(), "events.jsonl")
	eventWriter := bcevents.NewJSONLWriter(eventsJSONL, 0)

	// Per-workspace SSE hub. Phase M6: also forward every event to the
	// global fan-in hub (if configured) so /api/events returns events
	// across all loaded workspaces, annotated with workspace_id.
	hub := bcws.NewHub()
	go hub.Run()
	if globals != nil && globals.GlobalHub != nil {
		hub.ForwardTo(globals.GlobalHub, bcworkspace.ComputeWorkspaceID(ws.RootDir))
	}
	addCloser(func() error { hub.Stop(); return nil })

	// Agent manager + service.
	agentMgr, containerBackend, agentErr := newAgentManager(ws)
	if agentErr != nil {
		svcCancel()
		return nil, fmt.Errorf("agent manager: %w", agentErr)
	}
	if err := agentMgr.LoadState(); err != nil {
		log.Warn("failed to load agent state", "error", err, "workspace", ws.RootDir)
	}
	if ws.RoleManager != nil {
		agentMgr.SetRoleManager(ws.RoleManager)
	}
	addCloser(func() error { return agentMgr.Close() })
	agentSvc := bcagent.NewAgentService(agentMgr, hub, nil)

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
	// ~/.bc/costs.db (M8e) when Globals.CostsGlobal is supplied — every
	// record is tagged with the workspace id via ScopedStore /
	// Importer.SetWorkspaceID so cross-workspace analytics work out of
	// the box. Fall back to the per-workspace store for legacy callers
	// / tests that assemble Globals by hand without the global ledger.
	var costStore *cost.Store
	var costImporter *cost.Importer
	if globals != nil && globals.CostsGlobal != nil {
		costStore = globals.CostsGlobal
		// Ownership stays with whoever populated Globals; no closer.
		wsID := bcworkspace.ComputeWorkspaceID(ws.RootDir)
		costImporter = cost.NewImporter(costStore, ws.RootDir)
		costImporter.SetWorkspaceID(wsID)
		wg.Add(1)
		go func() {
			defer wg.Done()
			runCostImportLoop(svcCtx, costImporter)
		}()
	} else if cs, err := cost.OpenStore(ws.RootDir); err != nil {
		log.Warn("cost store unavailable", "error", err, "workspace", ws.RootDir)
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

	// Cron store + scheduler.
	var cronStore *cron.Store
	var cronSched *cron.Scheduler
	if cr, err := cron.Open(ws.RootDir); err != nil {
		log.Warn("cron store unavailable", "error", err, "workspace", ws.RootDir)
	} else {
		cronStore = cr
		addCloser(func() error { return cr.Close() })

		cronLogDir := filepath.Join(ws.RootDir, ".bc", "logs", "cron")
		cronSched = cron.NewSchedulerWithConfig(cr, cronLogDir,
			ws.Config.Cron.PollIntervalSeconds, ws.Config.Cron.JobTimeoutSeconds)
		wg.Add(1)
		go func() {
			defer wg.Done()
			cronSched.Run(svcCtx)
		}()
	}

	// Secret store. Prefer the user-global vault (~/.bc/secrets.vault)
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
	} else if ss, err := bcsecret.NewStore(ws.RootDir, passphrase); err != nil {
		log.Warn("secret store unavailable", "error", err, "workspace", ws.RootDir)
	} else {
		secretStore = ss
		addCloser(func() error { return ss.Close() })
	}

	// MCP store.
	var mcpStore *bcmcp.Store
	if ms, err := bcmcp.NewStore(ws.RootDir); err != nil {
		log.Warn("mcp store unavailable", "error", err, "workspace", ws.RootDir)
	} else {
		mcpStore = ms
		addCloser(func() error { return ms.Close() })
	}

	// Tool store.
	var toolStore *bctool.Store
	{
		ts := bctool.NewStore(ws.StateDir())
		if err := ts.Open(); err != nil {
			log.Warn("tool store unavailable", "error", err, "workspace", ws.RootDir)
		} else {
			toolStore = ts
			addCloser(func() error { return ts.Close() })
		}
	}

	// Template store: user-global (~/.bc/templates/) with workspace
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
	if el, err := bcevents.OpenLog(ws.RootDir, filepath.Join(ws.StateDir(), "state.db")); err != nil {
		log.Warn("event log unavailable", "error", err, "workspace", ws.RootDir)
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

	// Stats collector — only runs if a TSDB stats store is configured
	// globally. Uses the current workspace's agentSvc.
	if globals != nil && globals.Stats != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runStatsCollector(svcCtx, globals.Stats, agentSvc, ws)
		}()
	}

	// Notify service (channel subscriptions + delivery).
	var notifyService *bcnotify.Service
	if ns, err := bcnotify.OpenStore(ws.RootDir); err != nil {
		log.Warn("notify store unavailable", "error", err, "workspace", ws.RootDir)
	} else {
		notifyService = bcnotify.NewService(ns, agentSvc, hub)
		wg.Add(1)
		go func() {
			defer wg.Done()
			runNotifyPruneLoop(svcCtx, notifyService)
		}()
	}

	// Gateway manager (Telegram/Discord/Slack adapters).
	gwManager := buildGatewayManager(svcCtx, ws, notifyService, &wg)

	// Provider registry is global but we keep it referenced for parity.
	_ = provider.DefaultRegistry

	svc := &WorkspaceServices{
		Workspace:    ws,
		Agents:       agentSvc,
		AgentMgr:     agentMgr,
		Channels:     notifyService, // same service powers channels for now
		Events:       eventLog,
		EventWriter:  eventWriter,
		Costs:        costStore,
		CostImporter: costImporter,
		Cron:         cronStore,
		CronSched:    cronSched,
		Secrets:      secretStore,
		MCP:          mcpStore,
		MCPGlobal:    globalMCPStore(globals),
		Tools:        toolStore,
		Templates:    tmplStore,
		Gateway:      gwManager,
		Notify:       notifyService,
		Hub:          hub,
		cancel:       svcCancel,
		wg:           &wg,
	}

	// Populate the legacy flat Services bundle so the existing handler
	// constructors in server.New() still receive a fully-wired payload
	// during phases M1-M4.
	svc.Services = Services{
		Agents:        agentSvc,
		Notify:        notifyService,
		Costs:         costStore,
		CostImporter:  costImporter,
		Cron:          cronStore,
		CronScheduler: cronSched,
		Secrets:       secretStore,
		MCP:           mcpStore,
		Tools:         toolStore,
		Templates:     tmplStore,
		EventLog:      eventLog,
		EventWriter:   eventWriter,
		WS:            ws,
		Gateway:       gwManager,
	}
	if globals != nil {
		svc.Services.Stats = globals.Stats
		svc.Services.Registry = globals.Registry
		svc.Services.Deps = globals.Deps
	}

	// Closer runs addCloser funcs in reverse order. cancel+wg.Wait are
	// handled by WorkspaceServices.Close() itself before invoking this.
	svc.closer = func() error {
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
// BuildWorkspaceServices stays compact.
func globalMCPStore(g *Globals) *bcmcp.GlobalStore {
	if g == nil {
		return nil
	}
	return g.MCPGlobal
}

// MCPLayeredView returns a read-oriented composite of global + workspace
// MCP registries for the given WorkspaceServices. Callers use it to
// list / resolve servers with workspace-overrides winning. Returns nil
// when neither layer is available.
func (ws *WorkspaceServices) MCPLayeredView() *bcmcp.LayeredView {
	if ws == nil {
		return nil
	}
	if ws.MCPGlobal == nil && ws.MCP == nil {
		return nil
	}
	return &bcmcp.LayeredView{Global: ws.MCPGlobal, Workspace: ws.MCP}
}

// newAgentManager mirrors the helper that used to live in serve.go.
func newAgentManager(ws *bcworkspace.Workspace) (*bcagent.Manager, *bccontainer.Backend, error) {
	var wsCfg bcworkspace.DockerRuntimeConfig
	if ws.Config != nil {
		wsCfg = ws.Config.Runtime.Docker
	}
	dockerCfg := bccontainer.ConfigFromWorkspace(wsCfg)
	be, err := bccontainer.NewBackend(dockerCfg, "bc-", ws.RootDir, provider.DefaultRegistry)
	if err != nil {
		log.Warn("Docker not available — agents will use tmux runtime only", "error", err, "workspace", ws.RootDir)
		return bcagent.NewWorkspaceManager(ws.AgentsDir(), ws.RootDir), nil, nil
	}
	mgr := bcagent.NewWorkspaceManagerWithRuntime(ws.AgentsDir(), ws.RootDir, be, "docker")
	return mgr, be, nil
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

	// Count enabled GitLab webhook adapters.
	var glCount int
	for _, c := range gw.GitLabs {
		if c.Enabled {
			glCount++
		}
	}

	// Count enabled Jira webhook adapters.
	var jiraCount int
	for _, c := range gw.Jiras {
		if c.Enabled {
			jiraCount++
		}
	}

	// Count enabled Linear webhook adapters.
	var linearCount int
	for _, c := range gw.Linears {
		if c.Enabled {
			linearCount++
		}
	}

	// Count enabled Sentry webhook adapters.
	var sentryCount int
	for _, c := range gw.Sentries {
		if c.Enabled {
			sentryCount++
		}
	}

	// Count enabled Stripe webhook adapters.
	var stripeCount int
	for _, c := range gw.Stripes {
		if c.Enabled {
			stripeCount++
		}
	}

	// Count enabled Bitbucket webhook adapters.
	var bbCount int
	for _, c := range gw.Bitbuckets {
		if c.Enabled {
			bbCount++
		}
	}

	// Count enabled PagerDuty webhook adapters.
	var pdCount int
	for _, c := range gw.PagerDuties {
		if c.Enabled {
			pdCount++
		}
	}

	// Count enabled Datadog webhook adapters.
	var ddCount int
	for _, c := range gw.Datadogs {
		if c.Enabled {
			ddCount++
		}
	}

	// Count enabled Grafana webhook adapters.
	var grafCount int
	for _, c := range gw.Grafanas {
		if c.Enabled {
			grafCount++
		}
	}

	// Count enabled Vercel webhook adapters.
	var vercelCount int
	for _, c := range gw.Vercels {
		if c.Enabled {
			vercelCount++
		}
	}

	// Count enabled Netlify webhook adapters.
	var netlifyCount int
	for _, c := range gw.Netlifys {
		if c.Enabled {
			netlifyCount++
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

	// Count enabled Signal poll adapters.
	var sigCount int
	for _, c := range gw.Signals {
		if c.Enabled && c.APIURL != "" {
			sigCount++
		}
	}

	// Count enabled Matrix poll adapters.
	var matrixCount int
	for _, c := range gw.Matrices {
		if c.Enabled && c.Token != "" {
			matrixCount++
		}
	}

	// Count enabled MS Teams webhook adapters.
	var teamsCount int
	for _, c := range gw.MSTeams {
		if c.Enabled {
			teamsCount++
		}
	}

	// Count enabled Google Chat webhook adapters.
	var gchatCount int
	for _, c := range gw.GoogleChats {
		if c.Enabled {
			gchatCount++
		}
	}

	// Count enabled LINE webhook adapters.
	var lineCount int
	for _, c := range gw.Lines {
		if c.Enabled {
			lineCount++
		}
	}

	// Count enabled Feishu webhook adapters.
	var feishuCount int
	for _, c := range gw.Feishus {
		if c.Enabled {
			feishuCount++
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

	// Count enabled Nostr socket adapters.
	var nostrCount int
	for _, c := range gw.Nostrs {
		if c.Enabled {
			nostrCount++
		}
	}

	// Count enabled Twitch webhook adapters.
	var twitchCount int
	for _, c := range gw.Twitches {
		if c.Enabled {
			twitchCount++
		}
	}

	// Count enabled iMessage poll adapters.
	var imsgCount int
	for _, c := range gw.IMessages {
		if c.Enabled && c.APIURL != "" {
			imsgCount++
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

	// Count enabled Home Assistant socket adapters.
	var haCount int
	for _, c := range gw.HomeAssistants {
		if c.Enabled {
			haCount++
		}
	}

	if tgCount == 0 && !dcEnabled && !slEnabled && ghCount == 0 && whCount == 0 && rssCount == 0 &&
		glCount == 0 && jiraCount == 0 && linearCount == 0 && sentryCount == 0 && stripeCount == 0 &&
		bbCount == 0 && pdCount == 0 && ddCount == 0 && grafCount == 0 && vercelCount == 0 && netlifyCount == 0 && notionCount == 0 &&
		waCount == 0 && sigCount == 0 && matrixCount == 0 && teamsCount == 0 && gchatCount == 0 &&
		lineCount == 0 && feishuCount == 0 && mmCount == 0 && ircCount == 0 && nostrCount == 0 &&
		twitchCount == 0 && imsgCount == 0 && mqttCount == 0 && twitterCount == 0 && redditCount == 0 && haCount == 0 {
		return nil
	}

	m := bcgateway.NewManager()
	if notifyService != nil {
		m.SetChannelStore(&channelPersister{store: notifyService.Store()})
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

	// Register GitLab webhook adapters.
	for label, c := range gw.GitLabs {
		if !c.Enabled {
			continue
		}
		adapterName := "gitlab"
		if label != "" {
			adapterName = "gitlab:" + label
		}
		m.Register(bcgitlab.NewNamed(adapterName, c.Token))
		log.Info("gateway: gitlab adapter registered", "name", adapterName)
	}

	// Register Jira webhook adapters.
	for label, c := range gw.Jiras {
		if !c.Enabled {
			continue
		}
		adapterName := "jira"
		if label != "" {
			adapterName = "jira:" + label
		}
		m.Register(bcjira.NewNamed(adapterName, c.Secret))
		log.Info("gateway: jira adapter registered", "name", adapterName)
	}

	// Register Linear webhook adapters.
	for label, c := range gw.Linears {
		if !c.Enabled {
			continue
		}
		adapterName := "linear"
		if label != "" {
			adapterName = "linear:" + label
		}
		m.Register(bclinear.NewNamed(adapterName, c.Secret))
		log.Info("gateway: linear adapter registered", "name", adapterName)
	}

	// Register Sentry webhook adapters.
	for label, c := range gw.Sentries {
		if !c.Enabled {
			continue
		}
		adapterName := "sentry"
		if label != "" {
			adapterName = "sentry:" + label
		}
		m.Register(bcsentry.NewNamed(adapterName, c.Secret))
		log.Info("gateway: sentry adapter registered", "name", adapterName)
	}

	// Register Stripe webhook adapters.
	for label, c := range gw.Stripes {
		if !c.Enabled {
			continue
		}
		adapterName := "stripe"
		if label != "" {
			adapterName = "stripe:" + label
		}
		m.Register(bcstripe.NewNamed(adapterName, c.Secret))
		log.Info("gateway: stripe adapter registered", "name", adapterName)
	}

	// Register Bitbucket webhook adapters.
	for label, c := range gw.Bitbuckets {
		if !c.Enabled {
			continue
		}
		adapterName := "bitbucket"
		if label != "" {
			adapterName = "bitbucket:" + label
		}
		m.Register(bcbitbucket.NewNamed(adapterName, c.Secret))
		log.Info("gateway: bitbucket adapter registered", "name", adapterName)
	}

	// Register PagerDuty webhook adapters.
	for label, c := range gw.PagerDuties {
		if !c.Enabled {
			continue
		}
		adapterName := "pagerduty"
		if label != "" {
			adapterName = "pagerduty:" + label
		}
		m.Register(bcpagerduty.NewNamed(adapterName, c.Secret))
		log.Info("gateway: pagerduty adapter registered", "name", adapterName)
	}

	// Register Datadog webhook adapters.
	for label, c := range gw.Datadogs {
		if !c.Enabled {
			continue
		}
		adapterName := "datadog"
		if label != "" {
			adapterName = "datadog:" + label
		}
		m.Register(bcdatadog.NewNamed(adapterName, c.Secret))
		log.Info("gateway: datadog adapter registered", "name", adapterName)
	}

	// Register Grafana webhook adapters.
	for label, c := range gw.Grafanas {
		if !c.Enabled {
			continue
		}
		adapterName := "grafana"
		if label != "" {
			adapterName = "grafana:" + label
		}
		m.Register(bcgrafana.NewNamed(adapterName, c.Token))
		log.Info("gateway: grafana adapter registered", "name", adapterName)
	}

	// Register Vercel webhook adapters.
	for label, c := range gw.Vercels {
		if !c.Enabled {
			continue
		}
		adapterName := "vercel"
		if label != "" {
			adapterName = "vercel:" + label
		}
		m.Register(bcvercel.NewNamed(adapterName, c.Secret))
		log.Info("gateway: vercel adapter registered", "name", adapterName)
	}

	// Register Netlify webhook adapters.
	for label, c := range gw.Netlifys {
		if !c.Enabled {
			continue
		}
		adapterName := "netlify"
		if label != "" {
			adapterName = "netlify:" + label
		}
		m.Register(bcnetlify.NewNamed(adapterName, c.Secret))
		log.Info("gateway: netlify adapter registered", "name", adapterName)
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
		m.Register(bcwhatsapp.NewNamed(adapterName, c.VerifyToken))
		log.Info("gateway: whatsapp adapter registered", "name", adapterName)
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

	// Register MS Teams webhook adapters.
	for label, c := range gw.MSTeams {
		if !c.Enabled {
			continue
		}
		adapterName := "msteams"
		if label != "" {
			adapterName = "msteams:" + label
		}
		m.Register(bcmsteams.NewNamed(adapterName, c.Secret))
		log.Info("gateway: msteams adapter registered", "name", adapterName)
	}

	// Register Google Chat webhook adapters.
	for label, c := range gw.GoogleChats {
		if !c.Enabled {
			continue
		}
		adapterName := "googlechat"
		if label != "" {
			adapterName = "googlechat:" + label
		}
		m.Register(bcgooglechat.NewNamed(adapterName, c.Secret))
		log.Info("gateway: googlechat adapter registered", "name", adapterName)
	}

	// Register LINE webhook adapters.
	for label, c := range gw.Lines {
		if !c.Enabled {
			continue
		}
		adapterName := "line"
		if label != "" {
			adapterName = "line:" + label
		}
		m.Register(bcline.NewNamed(adapterName, c.Secret))
		log.Info("gateway: line adapter registered", "name", adapterName)
	}

	// Register Feishu webhook adapters.
	for label, c := range gw.Feishus {
		if !c.Enabled {
			continue
		}
		adapterName := "feishu"
		if label != "" {
			adapterName = "feishu:" + label
		}
		m.Register(bcfeishu.NewNamed(adapterName, c.Secret))
		log.Info("gateway: feishu adapter registered", "name", adapterName)
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
		m.Register(bcmattermost.NewNamed(adapterName, c.Token))
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
		m.Register(bcirc.NewNamed(adapterName, c.Server))
		log.Info("gateway: irc adapter registered", "name", adapterName)
	}

	// Register Nostr socket adapters.
	for label, c := range gw.Nostrs {
		if !c.Enabled {
			continue
		}
		adapterName := "nostr"
		if label != "" {
			adapterName = "nostr:" + label
		}
		m.Register(bcnostr.NewNamed(adapterName, c.RelayURL))
		log.Info("gateway: nostr adapter registered", "name", adapterName)
	}

	// Register Twitch webhook adapters.
	for label, c := range gw.Twitches {
		if !c.Enabled {
			continue
		}
		adapterName := "twitch"
		if label != "" {
			adapterName = "twitch:" + label
		}
		m.Register(bctwitch.NewNamed(adapterName, c.Secret))
		log.Info("gateway: twitch adapter registered", "name", adapterName)
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

	// Register MQTT socket adapters.
	for label, c := range gw.MQTTs {
		if !c.Enabled {
			continue
		}
		adapterName := "mqtt"
		if label != "" {
			adapterName = "mqtt:" + label
		}
		m.Register(bcmqtt.NewNamed(adapterName, c.BrokerURL, c.Topic))
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

	// Register Home Assistant socket adapters.
	for label, c := range gw.HomeAssistants {
		if !c.Enabled {
			continue
		}
		adapterName := "homeassistant"
		if label != "" {
			adapterName = "homeassistant:" + label
		}
		m.Register(bchomeassistant.NewNamed(adapterName, c.URL, c.Token))
		log.Info("gateway: homeassistant adapter registered", "name", adapterName)
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

// runEventPruneLoop prunes stale events (TTL 24h, max 5000 per agent)
// every hour.
func runEventPruneLoop(ctx context.Context, prunable *bcevents.SQLiteLog) {
	if n, err := prunable.Prune(24*time.Hour, 5000); err != nil {
		log.Warn("event prune failed", "error", err)
	} else if n > 0 {
		log.Info("event prune: deleted stale events", "count", n)
	}
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if n, err := prunable.Prune(24*time.Hour, 5000); err != nil {
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

func (p *channelPersister) LoadChannels(ctx context.Context) ([]bcgateway.PersistedChannel, error) {
	ncs, err := p.store.LoadChannels(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]bcgateway.PersistedChannel, len(ncs))
	for i, c := range ncs {
		result[i] = bcgateway.PersistedChannel{
			BCChannel:  c.BCChannel,
			Platform:   c.Platform,
			PlatformID: c.PlatformID,
		}
	}
	return result, nil
}

// Minor helper: ensure string conversion doesn't drop trailing slashes in
// future path-manipulation callers. Currently unused but kept as a
// safety hook for phase M5 where wsRoot may come from external input.
var _ = strings.TrimSpace
