package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	bccost "github.com/rpuneet/mycel/pkg/cost"
	bcdb "github.com/rpuneet/mycel/pkg/db"
	bcdeps "github.com/rpuneet/mycel/pkg/deps"
	"github.com/rpuneet/mycel/pkg/log"
	bcmcp "github.com/rpuneet/mycel/pkg/mcp"
	bcsecret "github.com/rpuneet/mycel/pkg/secret"
	bcstats "github.com/rpuneet/mycel/pkg/stats"
	bctemplate "github.com/rpuneet/mycel/pkg/template"
	bcworkspace "github.com/rpuneet/mycel/pkg/workspace"
	"github.com/rpuneet/mycel/server"
	bcws "github.com/rpuneet/mycel/server/ws"
)

// RunServer starts the mycel server (formerly bcd) in the foreground.
// It loads the workspace registry, constructs shared Globals, builds the
// launch workspace's WorkspaceServices via the server-side factory, wires
// handlers, and blocks until the context is canceled or a signal is
// received.
func RunServer(addr, wsRoot, corsOrigin, apiKey string) error {
	// Normalize addr: ":8080" → "127.0.0.1:8080"
	addr = normalizeAddr(addr)

	ws, err := bcworkspace.Load(wsRoot)
	if err != nil {
		ws, err = bcworkspace.Init(wsRoot)
		if err != nil {
			return fmt.Errorf("init workspace %s: %w", wsRoot, err)
		}
	}

	// Multi-workspace registry: load (or create) the global registry at
	// ~/.mycel/workspaces.json, auto-register the workspace bcd was booted
	// against, and mark it active so legacy /api/ routes resolve correctly.
	registry, regErr := bcworkspace.LoadRegistry()
	if regErr != nil {
		log.Warn("workspace registry unavailable — multi-workspace routes disabled", "error", regErr)
		registry = nil
	}
	if registry != nil {
		if rErr := registry.RegisterWithAlias(ws.RootDir, ws.Name(), ""); rErr != nil {
			log.Warn("workspace registry: register current failed", "error", rErr)
		}
		if registry.GetActive() == nil {
			if sErr := registry.SetActive(ws.RootDir); sErr != nil {
				log.Warn("workspace registry: set active failed", "error", sErr)
			}
		}
		if sErr := registry.Save(); sErr != nil {
			log.Warn("workspace registry: save failed", "error", sErr)
		}
	}

	// Set up shared database connection for all stores.
	var storageCfg *bcdb.StorageSettings
	if ws.Config != nil {
		storageCfg = &bcdb.StorageSettings{
			Default: ws.Config.Storage.Default,
			SQLite:  bcdb.SQLiteSettings{Path: ws.Config.Storage.SQLite.Path},
			Timescale: bcdb.TimescaleSettings{
				Host:     ws.Config.Storage.Timescale.Host,
				Port:     ws.Config.Storage.Timescale.Port,
				User:     ws.Config.Storage.Timescale.User,
				Password: ws.Config.Storage.Timescale.Password,
				Database: ws.Config.Storage.Timescale.Database,
			},
		}
	}
	sharedDB, sharedDriver, dbErr := bcdb.OpenWorkspaceDBWithConfig(ws.RootDir, storageCfg)
	if dbErr != nil {
		log.Warn("failed to open shared workspace db", "error", dbErr)
	} else {
		bcdb.SetShared(sharedDB, sharedDriver)
		defer bcdb.CloseShared() //nolint:errcheck
		configDriver := ""
		if ws.Config != nil {
			configDriver = ws.Config.Storage.Default
		}
		log.Info("shared database ready", "driver", sharedDriver, "config_driver", configDriver)
	}

	pidPath, pidErr := bcworkspace.DaemonPidPath()
	if pidErr != nil {
		log.Warn("failed to resolve daemon pid path", "error", pidErr)
	} else if err := writePID(pidPath); err != nil {
		log.Warn("failed to write PID file", "path", pidPath, "error", err)
	}
	defer func() {
		if pidPath != "" {
			_ = os.Remove(pidPath)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Global fan-in SSE hub — phase M6 will connect per-workspace hubs to
	// this one so /api/events shows cross-workspace activity. For now it
	// is the same hub server.New() receives.
	globalHub := bcws.NewHub()
	go globalHub.Run()
	defer globalHub.Stop()

	// Global stats store — TimescaleDB connection shared across workspaces.
	var statsStore *bcstats.Store
	{
		dsn := bcstats.StatsDSN()
		if ss, err := bcstats.NewStore(dsn); err != nil {
			log.Warn("stats store unavailable (TimescaleDB)", "error", err, "dsn", redactDSN(dsn))
		} else {
			statsStore = ss
			defer ss.Close() //nolint:errcheck // best-effort
			log.Info("stats store: using TimescaleDB", "dsn", redactDSN(dsn))
		}
	}

	// Optional dependencies registry (bc-db, bc-code-server, bc-browser).
	depsRegistry := bcdeps.NewRegistry()
	bcCodeServer := bcdeps.NewBCCodeServer(ws.RootDir)
	depsRegistry.Register(bcdeps.NewBCDB())
	depsRegistry.Register(bcCodeServer)
	depsRegistry.Register(bcdeps.NewBCBrowser())

	// User-global template store at ~/.mycel/templates/. Seeded on first run;
	// each workspace wraps this store with its own override directory.
	var templatesStore *bctemplate.Store
	if globalTmplDir, gtErr := bcworkspace.GlobalTemplatesDir(); gtErr != nil {
		log.Warn("global templates dir unavailable", "error", gtErr)
	} else {
		if _, ensureErr := bcworkspace.EnsureGlobalDir(); ensureErr != nil {
			log.Warn("ensure global bc dir", "error", ensureErr)
		}
		if seedErr := bctemplate.SeedDefaults(globalTmplDir); seedErr != nil {
			log.Warn("seed global template defaults", "error", seedErr)
		}
		templatesStore = bctemplate.NewStore(globalTmplDir)
	}

	// User-global secrets vault at ~/.mycel/secrets.vault. A single vault
	// keeps ANTHROPIC_API_KEY and friends visible across every workspace.
	var globalVault *bcsecret.Store
	if vaultPath, vpErr := bcworkspace.GlobalSecretsVault(); vpErr != nil {
		log.Warn("global secrets vault path unavailable", "error", vpErr)
	} else if passphrase, passErr := bcsecret.Passphrase(); passErr != nil {
		log.Warn("secret passphrase unavailable — global vault disabled", "error", passErr)
	} else if gv, openErr := bcsecret.OpenVaultFile(vaultPath, passphrase); openErr != nil {
		log.Warn("global secrets vault unavailable", "error", openErr, "path", vaultPath)
	} else {
		globalVault = gv
		defer gv.Close() //nolint:errcheck // best-effort
	}

	// User-global MCP registry at ~/.mycel/mcps.json. Workspaces still have
	// their own SQLite-backed overrides; handlers and agent spawn logic
	// compose the two at resolve time.
	var mcpGlobal *bcmcp.GlobalStore
	if mcpPath, mpErr := bcworkspace.GlobalMCPConfig(); mpErr != nil {
		log.Warn("global mcp config path unavailable", "error", mpErr)
	} else {
		mcpGlobal = bcmcp.NewGlobalStore(mcpPath)
	}

	// User-global cost ledger at ~/.mycel/costs.db. Records carry
	// workspace_id for cross-workspace analytics. When the ledger
	// cannot be opened, per-workspace stores continue to work via the
	// build_services.go fallback.
	var costsGlobal *bccost.Store
	if costsPath, cpErr := bcworkspace.GlobalCostsDB(); cpErr != nil {
		log.Warn("global costs path unavailable", "error", cpErr)
	} else if cs, openErr := bccost.OpenGlobalStore(costsPath); openErr != nil {
		log.Warn("global costs ledger unavailable", "error", openErr, "path", costsPath)
	} else {
		costsGlobal = cs
		defer cs.Close() //nolint:errcheck // best-effort
	}

	globals := &server.Globals{
		Registry:     registry,
		Stats:        statsStore,
		Deps:         depsRegistry,
		GlobalHub:    globalHub,
		Templates:    templatesStore,
		SecretsVault: globalVault,
		MCPGlobal:    mcpGlobal,
		CostsGlobal:  costsGlobal,
		Build:        server.BuildInfo{Version: version, Commit: commit, BuiltAt: date},
	}

	// Build the launch workspace's services via the factory.
	launchSvc, buildErr := server.BuildWorkspaceServices(ctx, globals, ws.RootDir)
	if buildErr != nil {
		return fmt.Errorf("build launch workspace services: %w", buildErr)
	}
	defer launchSvc.Close() //nolint:errcheck // best-effort

	// Per-workspace services manager. Phase M5: the factory builds real
	// services for ANY registered workspace on first access. The launch
	// workspace's bundle is reused from the eager build above.
	wsMgr := server.NewWorkspaceManager(registry, func(ctx context.Context, w *bcworkspace.Workspace) (*server.WorkspaceServices, error) {
		if w.RootDir == ws.RootDir {
			bcCodeServer.SetWorkspaceRoot(w.RootDir)
			return launchSvc, nil
		}
		return server.BuildWorkspaceServices(ctx, globals, w.RootDir)
	})
	// WorkspaceManager is wired into Services by NewWithManager, not here.

	if registry != nil {
		if _, loadErr := wsMgr.LoadActive(ctx); loadErr != nil {
			log.Warn("workspace manager: eager load active failed", "error", loadErr)
		}
		wsMgr.StartEvictionLoop(ctx)
		defer wsMgr.Close() //nolint:errcheck // best-effort
	}

	cfg := server.DefaultConfig()
	if addr != "" {
		cfg.Addr = addr
	}
	cfg.CORSOrigin = corsOrigin
	cfg.APIKey = apiKey
	if apiKey != "" {
		log.Info("API key authentication enabled")
	}
	cfg.Build = server.BuildInfo{
		Version: version,
		Commit:  commit,
		BuiltAt: date,
	}

	// Rewrite agent hook settings to point at the actual bcd address.
	updateAgentHookPorts(ws, cfg.Addr)

	// Phase M4: use the manager-based constructor. The server no longer
	// needs a flat Services bundle to function — everything is resolved
	// per-request via the manager + context.
	srv := server.NewWithManager(cfg, wsMgr, globals, server.WebDist())
	return srv.Start(ctx)
}

// updateAgentHookPorts rewrites agent hook settings to use the current bcd address.
// This is necessary because existing tmux sessions don't inherit the BC_BCD_ADDR
// environment variable that is set in the bcd process env.
func updateAgentHookPorts(ws *bcworkspace.Workspace, listenAddr string) {
	bcdURL := "http://" + listenAddr
	agentsDir := filepath.Join(ws.StateDir(), "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		agentName := e.Name()
		settingsGlob := filepath.Join(agentsDir, agentName, "*", ".claude", "settings.json")
		matches, _ := filepath.Glob(settingsGlob) //nolint:errcheck // Glob only errors on bad pattern
		for _, settingsPath := range matches {
			data, readErr := os.ReadFile(settingsPath) //nolint:gosec // path is the result of filepath.Glob under the agents dir
			if readErr != nil {
				continue
			}
			content := string(data)
			updated := strings.ReplaceAll(content, "http://127.0.0.1:9374", bcdURL)
			updated = strings.ReplaceAll(updated, "${BC_BCD_ADDR:-http://127.0.0.1:9374}", bcdURL)

			if updated != content {
				if writeErr := os.WriteFile(settingsPath, []byte(updated), 0644); writeErr != nil { //nolint:gosec // agent settings file
					log.Warn("failed to update hook port", "path", settingsPath, "error", writeErr)
					continue
				}
				log.Info("updated hook port", "agent", agentName, "addr", bcdURL)
			}
		}
	}
}

func writePID(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0600)
}

// redactDSN replaces credentials in a DSN URL with "***" to prevent secret leakage in logs.
func redactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "***"
	}
	if u.User != nil {
		u.User = url.UserPassword("***", "***")
	}
	return u.String()
}
