// Package server implements the bcd HTTP API server.
//
// The server exposes workspace state over HTTP so the bc CLI can operate as a
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
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/marketplace"
	"github.com/rpuneet/mycel/pkg/mcp"
	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/stats"
	"github.com/rpuneet/mycel/pkg/template"
	"github.com/rpuneet/mycel/pkg/tool"
	"github.com/rpuneet/mycel/pkg/workspace"
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
// bcd is single-tenant: exactly one Services value is built at boot
// (see BuildServices) and lives for the process lifetime.
type Services struct {
	Agents       *agent.AgentService
	AgentMgr     *agent.Manager
	Costs        *cost.Store
	CostImporter *cost.Importer
	Secrets      *secret.Store
	MCP          *mcp.Store
	MCPGlobal    *mcp.GlobalStore // user-global MCP registry (~/.mycel/mcps.json)
	Tools        *tool.Store
	Templates    *template.Store
	Stats        *stats.Store
	EventLog     events.EventStore
	EventWriter  *events.JSONLWriter
	WS           *workspace.Workspace
	Gateway      *gateway.Manager
	Notify       *notify.Service
	// Hub is the process-wide SSE hub the bundle publishes into.
	Hub *ws.Hub
	// Degraded maps service name → reason for services that failed to
	// initialize and were left nil (see BuildServices).
	// Surfaced by /api/health so degradation is loud, not silent.
	Degraded map[string]string
	// Deps is the optional dependencies registry (bc-db, bc-code-server,
	// bc-browser). May be nil in tests; when nil the /api/deps handler
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

// Server is the bcd HTTP server.
type Server struct {
	httpServer *http.Server
	handler    http.Handler
	addr       string
}

// New creates a bcd server with the given config, services, SSE hub, and optional static files.
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
		if svc.Costs != nil {
			if db := svc.Costs.DB(); db != nil {
				ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
				defer cancel()
				var one int
				if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
					w.WriteHeader(http.StatusServiceUnavailable)
					fmt.Fprintf(w, `{"status":"unhealthy","db":"error: %s"}`, strings.ReplaceAll(err.Error(), `"`, `'`)) //nolint:errcheck
					return
				}
			}
		}
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
			if _, err := svc.Costs.WorkspaceSummary(r.Context()); err != nil {
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
		ah := handlers.NewAgentHandler(svc.Agents, svc.Costs, svc.WS, hub)
		if svc.EventLog != nil {
			ah.SetEventStore(svc.EventLog)
		}
		if svc.Stats != nil {
			ah.SetStatsStore(svc.Stats)
		}
		if svc.WS != nil {
			// Prefer the layered store populated by BuildServices;
			// fall back to a single-layer per-workspace store for callers
			// that construct Services manually (eg. legacy tests).
			if svc.Templates != nil {
				ah.SetTemplateStore(svc.Templates)
			} else {
				templatesDir := filepath.Join(svc.WS.StateDir(), "templates")
				ah.SetTemplateStore(template.NewStore(templatesDir))
			}
		}
		ah.SetTerminalHandler(handlers.NewTerminalHandler(svc.Agents, cfg.CORSOrigin))
		ah.Register(mux)
	}
	// Wire gateway inbound callback for notify dispatch and SSE publish.
	if svc.Gateway != nil {
		svc.Gateway.SetInboundHandler(func(ch, sender, senderID, content, messageID string, mentions []string, raw json.RawMessage) {
			// Publish SSE event for web UI (non-blocking)
			if hub != nil {
				hub.Publish("channel.message", map[string]any{
					"channel": ch,
					"message": map[string]any{
						"sender":  sender,
						"content": content,
						"type":    "text",
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
				svc.Notify.Dispatch(ch, platform, sender, senderID, content, messageID, mentions, nil, raw)
			}
		})
	}
	if svc.Costs != nil {
		handlers.NewCostHandler(svc.Costs, svc.CostImporter).Register(mux)
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
	handlers.NewUnifiedToolsHandler(svc.MCP, svc.Tools, svc.Agents, svc.WS).Register(mux)

	// Provider registry endpoint — always registered
	handlers.NewProviderHandler(provider.DefaultRegistry, svc.Agents, svc.Costs, svc.WS).Register(mux)
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
	// in workspaces without an active gateway adapter.
	if svc.Gateway != nil || svc.Notify != nil {
		gh := handlers.NewGatewayHandler(svc.Gateway, svc.WS)
		if svc.Notify != nil {
			gh.SetNotifyService(svc.Notify)
		}
		// /api/apps: descriptor catalog + instance CRUD + auth flows.
		// The gateway handler serves the channel/subscription surface
		// (/api/apps/channels*, /api/notify/*) and the per-instance
		// routes the apps router delegates to.
		handlers.NewAppsHandler(gh, svc.Gateway, svc.WS, svc.Secrets).Register(mux)
		gh.Register(mux)
	}
	// Repo listing + discovery scanners for the folder picker. The repos
	// handler is nil-safe (empty list without an agent service); adding a
	// repo IS creating an agent with that repo path, so there is no
	// registration surface.
	rootDir := ""
	if svc.WS != nil {
		rootDir = svc.WS.RootDir
	}
	handlers.NewReposHandler(svc.Agents, rootDir).Register(mux)
	handlers.NewDiscoveryHandler().Register(mux)
	// Optional dependencies manager (bc-db, bc-code-server, bc-browser).
	// Always registered so the UI can render an empty list when no deps
	// are configured; the handler is nil-safe internally.
	handlers.NewDepsHandler(svc.Deps).Register(mux)
	// Per-repo cost rollup. Handler is nil-safe and returns 503 when the
	// global ledger isn't wired.
	mux.Handle("/api/global/costs", handlers.NewGlobalCostsHandler(svc.Costs))
	// Degradation reasons for 503 responses (see serviceUnavailable).
	handlers.SetDegraded(svc.Degraded)
	// Code tab endpoints — anchored at the single bundle's repo root.
	handlers.NewCodeHandler(handlers.NewStaticWorkspaceResolver(rootDir)).Register(mux)
	if svc.WS != nil {
		handlers.NewRolesHandler(svc.WS).Register(mux)
		handlers.NewDoctorHandler(svc.WS).Register(mux)
		handlers.NewSettingsHandler(svc.WS).Register(mux)

		// Templates — prefer the layered store from BuildServices
		// (global ~/.mycel/templates/ + per-workspace override). Fallback to
		// a single-layer workspace store for legacy test callers that
		// assemble Services by hand.
		tmplStore := svc.Templates
		templatesDir := filepath.Join(svc.WS.StateDir(), "templates")
		if tmplStore == nil {
			tmplStore = template.NewStore(templatesDir)
			if seedErr := template.SeedDefaults(templatesDir); seedErr != nil {
				log.Warn("seed default templates", "error", seedErr)
			}
		}
		if migrErr := migrateRolesToTemplates(svc.WS.RolesDir(), templatesDir); migrErr != nil {
			log.Warn("migrate roles to templates", "error", migrErr)
		}
		handlers.NewTemplateHandler(tmplStore).Register(mux)

		// Marketplace — live catalog aggregating MCP registry, GitHub, vendor skill
		// sources (Claude/openclaw/Gemini), and local templates. The install
		// endpoint reuses the agent-message path (AgentService.Send) to dispatch
		// install instructions; sender may be nil when agents are unavailable.
		var mktSender handlers.AgentSender
		if svc.Agents != nil {
			mktSender = svc.Agents
		}
		handlers.NewMarketplaceHandler(marketplace.NewAggregator(tmplStore, nil), mktSender).Register(mux)

		// File upload/download for channel attachments + shared screenshots
		fileStore := attachment.NewStore(svc.WS.StateDir())
		fileStore.AddSharedDir("/tmp/bc-shared")
		handlers.NewFileHandler(fileStore).Register(mux)
	}

	// Stats endpoints (always registered; nil-safe internally)
	sh := handlers.NewStatsHandler(svc.Agents, svc.Costs, svc.Tools, svc.WS, svc.Stats)
	if svc.Gateway != nil {
		sh.SetGateway(svc.Gateway)
	}
	if svc.Notify != nil {
		sh.SetNotify(svc.Notify)
	}
	sh.Register(mux)

	// Agent-facing MCP server (streamable HTTP), mounted at /_mcp/{agent}.
	// The path segment is the trusted sender identity for agent tools.
	if svc.WS != nil {
		mcpCfg := servermcp.Config{
			Workspace: svc.WS,
			Costs:     svc.Costs,
			Gateway:   svc.Gateway,
			Notify:    svc.Notify,
			Version:   cfg.Build.Version,
		}
		if svc.Agents != nil {
			mcpCfg.Agents = svc.Agents.Manager()
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
	}

	// Middleware chain (outermost runs first):
	// RateLimit → APIKeyAuth → RequestID → RequestLogger → Recovery → Gzip → MaxBodySize → CORS → mux
	var handler http.Handler = mux
	if cfg.CORS {
		origin := cfg.CORSOrigin
		if origin == "" {
			origin = "*"
		}
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
			MCPs:        []string{"bc"},
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

	log.Info("bcd listening", "addr", s.addr)

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
