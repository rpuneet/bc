// Package server implements the mycel daemon HTTP API server.
//
// The server exposes mycel state over HTTP so the bc CLI can operate as a
// thin client. It binds to localhost only by default and serves:
//
//   - REST API at /api/…  (JSON, one handler file per resource)
//   - SSE stream at /api/events  (real-time agent state updates)
//   - Static web UI at /  (embedded web/dist, served when built)
//   - Health probe at /health
//
// Default address: 127.0.0.1:9374
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/attachment"
	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/deps"
	"github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/marketplace"
	"github.com/rpuneet/mycel/pkg/mcp"
	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/stats"
	"github.com/rpuneet/mycel/pkg/template"
	"github.com/rpuneet/mycel/pkg/tool"
	"github.com/rpuneet/mycel/server/handlers"
	servermcp "github.com/rpuneet/mycel/server/mcp"
	"github.com/rpuneet/mycel/server/ws"
)

const defaultAddr = "127.0.0.1:9374"

// BuildInfo holds build-time metadata injected via ldflags.
type BuildInfo struct {
	Version string // semantic version tag (e.g. "0.3.1"), or "dev" for source builds
	Commit  string // short git commit hash
	BuiltAt string // UTC build timestamp (RFC 3339)
}

// Config holds server configuration.
type Config struct {
	Build      BuildInfo // build-time metadata
	Addr       string    // default "127.0.0.1:9374"
	CORSOrigin string    // allowed origin (default "*")
	APIKey     string    // optional API key for Bearer token auth (empty = disabled)
	CORS       bool      // enable permissive CORS headers (safe for loopback)
}

// DefaultConfig returns the default server configuration.
func DefaultConfig() Config {
	return Config{Addr: defaultAddr, CORS: true}
}

// Services bundles all service/store dependencies for the handlers.
// the daemon is single-tenant: exactly one Services value is built at boot
// (see BuildServices) and lives for the process lifetime.
type Services struct {
	Agents   *agent.AgentService
	AgentMgr *agent.Manager
	// Costs computes analytics directly from provider session files
	// (source-direct — no ledger).
	Costs       *cost.Service
	Secrets     *secret.Store
	MCP         *mcp.Store
	MCPGlobal   *mcp.GlobalStore // user-global MCP registry (~/.mycel/mcps.json)
	Tools       *tool.Store
	Templates   *template.Store
	Stats       *stats.Store
	EventLog    events.EventStore
	EventWriter *events.JSONLWriter
	Home        *home.Home
	Gateway     *gateway.Manager
	Notify      *notify.Service
	// Hub is the process-wide SSE hub the bundle publishes into.
	Hub *ws.Hub
	// Degraded maps service name → reason for services that failed to
	// initialize and were left nil (see BuildServices).
	// Surfaced by /api/health so degradation is loud, not silent.
	Degraded map[string]string
	// Deps is the optional dependencies registry (mycel-db, mycel-code-server,
	// mycel-browser). May be nil in tests; when nil the /api/deps handler
	// returns an empty list and 404 for detail routes.
	Deps *deps.Registry

	// cancel stops background goroutines started by BuildServices. It is
	// invoked first so goroutines can observe shutdown before their
	// underlying stores close.
	// lifecycle carries teardown state behind a pointer so Services can be
	// copied by value; sync.Once makes Close idempotent and concurrent-safe.
	lifecycle *serviceLifecycle
	// wg lets Close wait for background goroutines to exit.
}

// serviceLifecycle owns the background-goroutine teardown for one built
// service bundle: cancel stops the goroutines, wg waits for them, closer
// tears down stores in reverse construction order.
type serviceLifecycle struct {
	cancel context.CancelFunc
	wg     *sync.WaitGroup
	closer func() error
	once   sync.Once
}

// Close stops background goroutines started by BuildServices, waits for
// them to exit, then invokes the factory-supplied closer to tear down
// stores. Safe to call multiple times and on a hand-assembled Services.
//
// The global database connection is deliberately NOT closed here: stores
// borrow it from pkg/db, which keeps it cached process-wide. It is closed
// at process shutdown via db.CloseGlobal.
func (s *Services) Close() error {
	lc := s.lifecycle
	if lc == nil {
		return nil
	}
	var err error
	lc.once.Do(func() {
		if lc.cancel != nil {
			lc.cancel()
		}
		if lc.wg != nil {
			lc.wg.Wait()
		}
		if lc.closer != nil {
			err = lc.closer()
		}
	})
	return err
}

// Server is the mycel daemon HTTP server.
type Server struct {
	httpServer *http.Server
	handler    http.Handler
	addr       string
}

// New creates a daemon server with the given config, services, SSE hub, and optional static files.
func New(cfg Config, svc Services, hub *ws.Hub, staticFiles fs.FS) *Server {
	if cfg.Addr == "" {
		cfg.Addr = defaultAddr
	}

	mux := http.NewServeMux()

	// Health probes
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","addr":%q,"commit":%q,"built_at":%q}`, cfg.Addr, cfg.Build.Commit, cfg.Build.BuiltAt) //nolint:errcheck // writing to response
	})

	// /api/health and /healthz: external-probe-friendly health endpoints
	// that verify DB connectivity with a SELECT 1 roundtrip. Registered
	// before the SPA catch-all so they aren't shadowed.
	apiHealth := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// version = semver tag when available (release builds), else commit
		// hash so source builds still round-trip a meaningful identifier.
		v := cfg.Build.Version
		if v == "" || v == "dev" {
			v = cfg.Build.Commit
		}
		// Services that failed to initialize at build time (see
		// Services.Degraded) flip the status to "degraded" and
		// are listed with their reasons so the outage is diagnosable
		// from a single curl. The healthy "ok" shape is unchanged.
		if len(svc.Degraded) > 0 {
			_ = json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck // writing to response
				"status":   "degraded",
				"db":       "ok",
				"version":  v,
				"commit":   cfg.Build.Commit,
				"degraded": svc.Degraded,
			})
			return
		}
		fmt.Fprintf(w, `{"status":"ok","db":"ok","version":%q,"commit":%q}`, v, cfg.Build.Commit) //nolint:errcheck
	}
	mux.HandleFunc("/api/health", apiHealth)
	mux.HandleFunc("/healthz", apiHealth)

	// Readiness probe — verifies downstream dependencies
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		checks := map[string]string{}
		status := "ok"

		// Check database connectivity
		if svc.Costs != nil {
			if _, err := svc.Costs.TotalSummary(r.Context()); err != nil {
				checks["db"] = "error: " + err.Error()
				status = "degraded"
			} else {
				checks["db"] = "ok"
			}
		}

		// Check agent runtime
		if svc.Agents != nil {
			checks["agents"] = fmt.Sprintf("%d total", len(svc.Agents.Manager().ListAgents()))
		}

		w.Header().Set("Content-Type", "application/json")
		if status != "ok" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		writeJSON := func(v any) {
			_ = json.NewEncoder(w).Encode(v) //nolint:errcheck
		}
		writeJSON(map[string]any{"status": status, "checks": checks})
	})

	// Wire event persistence into the SSE hub
	if hub != nil && svc.EventWriter != nil {
		hub.SetWriter(svc.EventWriter)
	}

	// SSE event stream
	if hub != nil {
		mux.Handle("/api/events", hub)
	}

	// Resource handlers (only registered when service is available)
	if svc.Agents != nil {
		ah := handlers.NewAgentHandler(svc.Agents, svc.Costs, svc.Home, hub)
		if svc.EventLog != nil {
			ah.SetEventStore(svc.EventLog)
		}
		if svc.Stats != nil {
			ah.SetStatsStore(svc.Stats)
		}
		if svc.Home != nil {
			// Prefer the layered store populated by BuildServices;
			// fall back to a single-layer repo-scoped store for callers
			// that construct Services manually (eg. legacy tests).
			if svc.Templates != nil {
				ah.SetTemplateStore(svc.Templates)
			} else {
				templatesDir := filepath.Join(svc.Home.StateDir(), "templates")
				ah.SetTemplateStore(template.NewStore(templatesDir))
			}
		}
		ah.SetTerminalHandler(handlers.NewTerminalHandler(svc.Agents, cfg.CORSOrigin))
		ah.Register(mux)
	}
	// Wire gateway inbound callback for notify dispatch and SSE publish.
	if svc.Gateway != nil {
		svc.Gateway.SetInboundHandler(func(ch, sender, senderID, senderAvatar, content, messageID string, mentions []string, raw json.RawMessage, automated bool) {
			// Publish SSE event for web UI (non-blocking)
			if hub != nil {
				hub.Publish("channel.message", map[string]any{
					"channel": ch,
					"message": map[string]any{
						"sender":     sender,
						"avatar_url": senderAvatar,
						"content":    content,
						"type":       "text",
					},
				})
			}
			// Dispatch to notify subscribers (new subscription system).
			// Handles @mention filtering and delivery logging.
			if svc.Notify != nil {
				platform := ""
				if idx := strings.Index(ch, ":"); idx > 0 {
					platform = ch[:idx]
				}
				var opts []notify.DispatchOption
				if automated {
					opts = append(opts, notify.Automated())
				}
				svc.Notify.Dispatch(ch, platform, sender, senderID, senderAvatar, content, messageID, mentions, nil, raw, opts...)
			}
		})

		// Record what agents send, not just what arrives, so channel history
		// reads as a conversation instead of a one-sided log.
		if svc.Notify != nil {
			svc.Gateway.SetOutboundHandler(svc.Notify.RecordOutbound)
		}
	}
	if svc.Costs != nil {
		handlers.NewCostHandler(svc.Costs).Register(mux)
	}
	if svc.Secrets != nil {
		handlers.NewSecretHandler(svc.Secrets).Register(mux)
	}
	if svc.MCP != nil {
		// Wire config lookup so SetEnabled can auto-insert config-only servers
		// (e.g., github, playwright) that exist in the tool store but not in
		// the mcp_servers table.
		if svc.Tools != nil {
			svc.MCP.SetConfigLookup(func(name string) *mcp.ServerConfig {
				t, err := svc.Tools.Get(context.Background(), name)
				if err != nil || t == nil || t.Type != tool.ToolTypeMCP {
					return nil
				}
				transport := mcp.TransportStdio
				if t.Transport == "sse" {
					transport = mcp.TransportSSE
				}
				return &mcp.ServerConfig{
					Name:      t.Name,
					Transport: transport,
					Command:   t.Command,
					URL:       t.URL,
					Args:      t.Args,
					Env:       t.Env,
					Enabled:   t.Enabled,
				}
			})
		}
		handlers.NewMCPHandler(svc.MCP).Register(mux)
	}
	if svc.Tools != nil {
		handlers.NewToolHandler(svc.Tools).Register(mux)
	}
	// Unified tools endpoint (MCP + CLI) — always registered
	handlers.NewUnifiedToolsHandler(svc.MCP, svc.Tools, svc.Agents, svc.Home).Register(mux)

	// Provider registry endpoint — always registered
	handlers.NewProviderHandler(provider.DefaultRegistry, svc.Agents, svc.Costs, svc.Home).Register(mux)
	if svc.EventLog != nil || svc.EventWriter != nil {
		eh := handlers.NewEventHandler(svc.EventLog)
		if svc.EventWriter != nil {
			eh.SetWriter(svc.EventWriter)
		}
		eh.Register(mux)
	}
	// Mount webhook HTTP handlers (GitHub, generic webhook) at /hooks/{name}.
	if svc.Gateway != nil {
		for name, h := range svc.Gateway.WebhookHandlers() {
			mux.Handle("/hooks/"+name, h)
		}
	}
	// Register gateway handler when a gateway manager is present OR when notify
	// service is available — notify subscription routes must be accessible even
	// when no gateway adapter is active.
	if svc.Gateway != nil || svc.Notify != nil {
		gh := handlers.NewGatewayHandler(svc.Gateway, svc.Home)
		if svc.Notify != nil {
			gh.SetNotifyService(svc.Notify)
		}
		// /api/apps: descriptor catalog + instance CRUD + auth flows.
		// The gateway handler serves the channel/subscription surface
		// (/api/apps/channels*, /api/notify/*) and the per-instance
		// routes the apps router delegates to.
		handlers.NewAppsHandler(gh, svc.Gateway, svc.Home, svc.Secrets).Register(mux)
		gh.Register(mux)
	}
	// Repo listing + discovery scanners for the folder picker. The repos
	// handler is nil-safe (empty list without an agent service); adding a
	// repo IS creating an agent with that repo path, so there is no
	// registration surface.
	rootDir := ""
	if svc.Home != nil {
		rootDir = svc.Home.RootDir
	}
	handlers.NewReposHandler(svc.Agents, rootDir).Register(mux)
	handlers.NewDiscoveryHandler().Register(mux)
	// Optional dependencies manager (mycel-db, mycel-code-server, mycel-browser).
	// Always registered so the UI can render an empty list when no deps
	// are configured; the handler is nil-safe internally.
	handlers.NewDepsHandler(svc.Deps).Register(mux)
	// Streamed host-dependency installer used by the setup wizard and the
	// Tools page. Loopback only; runs vetted install/upgrade commands (from
	// the table or the tools registry) and streams their output.
	handlers.NewDepsInstallHandler().SetToolStore(svc.Tools).Register(mux)
	// Read-only autodetect of host OS + package managers (brew/apt/npm/…)
	// with versions. Shells out only to each manager's own --version probe.
	handlers.NewPackageManagersHandler().Register(mux)
	// Guarded registry search + install: `brew search`/`npm search`/… behind
	// strict input validation, argv-slice exec (no shell), timeouts, result
	// caps, and a loopback gate. Install streams NDJSON like the deps installer.
	handlers.NewPackageSearchHandler().Register(mux)
	// Hand an external link to the host's default browser. The desktop app's
	// webview lives on the daemon's http:// origin, where Wails never injects
	// BrowserOpenURL, so window.open is a no-op; the UI posts the URL here
	// instead. Loopback-only, http/https-only, exec'd as a single argv (no shell).
	handlers.NewOpenURLHandler().Register(mux)
	// Per-repo cost rollup. Handler is nil-safe and returns 503 when the
	// global ledger isn't wired.
	mux.Handle("/api/global/costs", handlers.NewGlobalCostsHandler(svc.Costs))
	// Degradation reasons for 503 responses (see serviceUnavailable).
	handlers.SetDegraded(svc.Degraded)
	// Code tab endpoints — anchored at the single bundle's repo root.
	wtResolver := func(name string) string {
		if svc.Agents == nil {
			return ""
		}
		return svc.Agents.Manager().WorktreeDirFor(name)
	}
	handlers.NewCodeHandler(handlers.NewStaticRepoResolver(rootDir)).WithWorktreeResolver(wtResolver).Register(mux)
	if svc.Home != nil {
		handlers.NewRolesHandler(svc.Home).Register(mux)
		handlers.NewDoctorHandler(svc.Home).Register(mux)
		handlers.NewSettingsHandler(svc.Home).Register(mux)
		// First-run setup wizard state. Agent count comes from the live
		// manager (nil-safe closure).
		handlers.NewOnboardingHandler(svc.Home, func() int {
			if svc.AgentMgr != nil {
				return svc.AgentMgr.AgentCount()
			}
			return 0
		}).Register(mux)

		// Templates — prefer the layered store from BuildServices
		// (global ~/.mycel/templates/ + repo-scoped override). Fallback to
		// a single-layer store for legacy test callers that
		// assemble Services by hand.
		tmplStore := svc.Templates
		templatesDir := filepath.Join(svc.Home.StateDir(), "templates")
		if tmplStore == nil {
			tmplStore = template.NewStore(templatesDir)
			if seedErr := template.SeedDefaults(templatesDir); seedErr != nil {
				log.Warn("seed default templates", "error", seedErr)
			}
		}
		if migrErr := migrateRolesToTemplates(svc.Home.RolesDir(), templatesDir); migrErr != nil {
			log.Warn("migrate roles to templates", "error", migrErr)
		}
		handlers.NewTemplateHandler(tmplStore).Register(mux)

		// Marketplace — live catalog aggregating MCP registry, GitHub, vendor skill
		// sources (Claude/openclaw/Gemini), and local templates. Template
		// installs write directly to tmplStore (deterministic); every other
		// item type dispatches an install instruction via the agent-message
		// path (AgentService.Send); sender may be nil when agents are unavailable.
		var mktSender handlers.AgentSender
		if svc.Agents != nil {
			mktSender = svc.Agents
		}
		handlers.NewMarketplaceHandler(marketplace.NewAggregator(tmplStore, nil), mktSender).
			WithTemplateStore(tmplStore).Register(mux)

		// File upload/download for channel attachments + shared screenshots
		fileStore := attachment.NewStore(svc.Home.StateDir())
		fileStore.AddSharedDir("/tmp/mycel-shared")
		handlers.NewFileHandler(fileStore).Register(mux)
	}

	// Stats endpoints (always registered; nil-safe internally)
	sh := handlers.NewStatsHandler(svc.Agents, svc.Costs, svc.Tools, svc.Home, svc.Stats)
	if svc.Gateway != nil {
		sh.SetGateway(svc.Gateway)
	}
	if svc.Notify != nil {
		sh.SetNotify(svc.Notify)
	}
	sh.Register(mux)

	// Agent-facing MCP server (streamable HTTP), mounted at /_mcp/{agent}.
	// The path segment is the trusted sender identity for agent tools.
	if svc.Home != nil {
		mcpCfg := servermcp.Config{
			Home:    svc.Home,
			Costs:   svc.Costs,
			Gateway: svc.Gateway,
			Notify:  svc.Notify,
			Version: cfg.Build.Version,
		}
		if svc.Agents != nil {
			mcpCfg.Agents = svc.Agents.Manager()
			mcpCfg.AgentSvc = svc.Agents
		}
		if mcpSrv, mcpErr := servermcp.New(mcpCfg); mcpErr != nil {
			log.Warn("MCP server unavailable", "error", mcpErr)
		} else {
			mcpSrv.Register(mux)
		}
	}

	// Static web UI with SPA fallback — serves files if they exist,
	// otherwise falls back to index.html for client-side routing.
	if staticFiles != nil {
		fileServer := http.FileServer(http.FS(staticFiles))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Unmatched API/hook paths must never fall through to the SPA:
			// returning index.html with 200 makes client bugs (calls to
			// endpoints that don't exist) silently unfixable.
			path := r.URL.Path
			if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/hooks/") || strings.HasPrefix(path, "/_mcp/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			}
			// Legacy workspace-prefixed URLs (/w/<hash>/<page>) are NOT
			// redirected server-side: browsers that cached the old
			// /<page> → /w/<hash>/<page> 301 would loop forever
			// (cached 301 → server 301 → cached 301 …). They fall
			// through to index.html below, and the client-side
			// LegacyWorkspaceRedirect route rewrites the URL in-app.
			//
			// Try serving the exact file first
			if path != "/" {
				if f, err := staticFiles.Open(path[1:]); err == nil {
					_ = f.Close() //nolint:errcheck // best-effort close
					fileServer.ServeHTTP(w, r)
					return
				}
			}
			// Fallback: serve index.html for SPA client-side routes
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		})
	} else {
		// A binary built without the UI bundle registered no root handler at
		// all, so opening the daemon in a browser gave Go's bare
		// "404 page not found" — indistinguishable from a broken daemon, a
		// wrong port, or a crashed server, when in fact the daemon is fine and
		// only its UI is absent. Saying so is the difference between a
		// two-minute fix and an afternoon.
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/hooks/") || strings.HasPrefix(r.URL.Path, "/_mcp/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			// 503, not 404: the daemon answered, and this address will serve
			// the UI once the binary carries it.
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(noWebUIPage))
		})
	}

	// Middleware chain (outermost runs first):
	// RateLimit → APIKeyAuth → RequestID → RequestLogger → Recovery → Gzip →
	// MaxBodySize → CORS → RejectCrossOriginMutations → mux
	var handler http.Handler = mux
	origin := cfg.CORSOrigin
	if origin == "" {
		origin = "*"
	}
	// Inside CORS so a preflight is still answered and the 403 carries CORS
	// headers, but applied even when CORS is off: a request that needs no
	// preflight reaches the mux regardless of what headers come back.
	handler = handlers.RejectCrossOriginMutations(origin, handler)
	if cfg.CORS {
		handler = handlers.CORSWithOrigin(origin, handler)
	}
	handler = handlers.MaxBodySize(1 << 20)(handler) // 1MB request body limit
	handler = handlers.Gzip(handler)
	handler = handlers.Recovery(handler)
	handler = handlers.RequestLogger(handler)
	handler = handlers.RequestID(handler)
	handler = handlers.APIKeyAuth(cfg.APIKey)(handler)
	limiter := handlers.NewRateLimiter(100, 200)
	handler = handlers.RateLimit(limiter)(handler)

	return &Server{
		addr:    cfg.Addr,
		handler: handler,
		httpServer: &http.Server{
			Addr:        cfg.Addr,
			Handler:     handler,
			ReadTimeout: 30 * time.Second,
			// WriteTimeout must be 0 for SSE connections (/api/events) which are long-lived.
			// Per-handler timeouts are used instead where needed.
			WriteTimeout: 0,
			IdleTimeout:  120 * time.Second,
		},
	}
}

// migrateRolesToTemplates copies .md role files from rolesDir into templatesDir
// as agent templates. It is idempotent: files whose template already exists are
// skipped. It logs each migration and is a no-op when rolesDir does not exist.
func migrateRolesToTemplates(rolesDir, templatesDir string) error {
	entries, err := os.ReadDir(rolesDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read roles dir: %w", err)
	}

	tmplStore := template.NewStore(templatesDir)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")

		// Skip if template already exists.
		if _, _, getErr := tmplStore.Get(name); getErr == nil {
			continue
		}

		data, readErr := os.ReadFile(filepath.Join(rolesDir, e.Name())) //nolint:gosec // trusted path
		if readErr != nil {
			log.Warn("migrate roles: failed to read role file", "role", name, "error", readErr)
			continue
		}

		t := template.Template{
			Name:        name,
			Description: "Migrated from role: " + name,
			MCPs:        []string{"mycel"},
		}
		if createErr := tmplStore.Create(t, string(data), template.ScopeGlobal); createErr != nil {
			log.Warn("migrate roles: failed to create template", "role", name, "error", createErr)
			continue
		}
		log.Info("migrate roles: created template from role", "name", name)
	}
	return nil
}

// Handler returns the HTTP handler (useful for httptest.NewServer in tests).
func (s *Server) Handler() http.Handler {
	return s.handler
}

// Addr returns the resolved listen address (updated after Start is called with :0).
func (s *Server) Addr() string {
	return s.addr
}

// Start begins listening. It blocks until ctx is canceled or an error occurs.
func (s *Server) Start(ctx context.Context) error {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	s.addr = ln.Addr().String()

	log.Info("daemon listening", "addr", s.addr)

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutCtx); err != nil {
			log.Warn("server shutdown error", "error", err)
		}
	}()

	if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
