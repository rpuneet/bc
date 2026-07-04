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
// bcd is single-tenant: it constructs shared Globals, builds the one
// Services bundle via server.BuildServices, wires handlers, and blocks
// until the context is canceled or a signal is received.
//
// wsRoot is the repo the daemon anchors on — new agents default their
// repo to it. It may be empty: the server then boots against MycelHome
// only (web UI + global APIs; no agent runtime until a repo exists).
// A non-empty wsRoot that isn't initialized yet is bootstrapped in place
// via workspace.Init — there is no separate init step.
func RunServer(addr, wsRoot, corsOrigin, apiKey string) error {
	// Normalize addr: ":8080" → "127.0.0.1:8080"
	addr = normalizeAddr(addr)

	var ws *bcworkspace.Workspace
	if wsRoot != "" {
		var err error
		ws, err = bcworkspace.Load(wsRoot)
		if err != nil {
			ws, err = bcworkspace.Init(wsRoot)
			if err != nil {
				return fmt.Errorf("bootstrap repo %s: %w", wsRoot, err)
			}
			log.Info("workspace bootstrapped", "root", ws.RootDir, "state", ws.StateDir())
		}
	} else {
		log.Info("no repo yet — run 'mycel up' inside a git repo to anchor the daemon")
	}

	// The single global database (<MycelHome>/mycel.db) is opened lazily
	// through pkg/db — including for a workspace-less boot where repos
	// are added later via the API. Warm the connection eagerly so
	// storage problems surface at boot, and close it at shutdown.
	defer bcdb.CloseGlobal() //nolint:errcheck
	{
		var cfg *bcdb.StorageSettings
		if ws != nil {
			cfg = ws.Config.DBStorageSettings()
		}
		if _, driver, dbErr := bcdb.Global(cfg); dbErr != nil {
			log.Warn("failed to open global db", "error", dbErr)
		} else {
			configDriver := ""
			if ws != nil && ws.Config != nil {
				configDriver = ws.Config.Storage.Default
			}
			log.Info("global database ready", "driver", driver, "config_driver", configDriver)
		}
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

	// The one SSE hub — the bundle publishes into it and server.New()
	// mounts it at /api/events.
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
	codeServerRoot := ""
	if ws != nil {
		codeServerRoot = ws.RootDir
	}
	bcCodeServer := bcdeps.NewBCCodeServer(codeServerRoot)
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

	// User-global cost ledger at ~/.mycel/costs.db. Records carry the
	// repo path for cross-repo analytics. When the ledger
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
		Stats:        statsStore,
		Deps:         depsRegistry,
		Hub:          globalHub,
		Templates:    templatesStore,
		SecretsVault: globalVault,
		MCPGlobal:    mcpGlobal,
		CostsGlobal:  costsGlobal,
		Build:        server.BuildInfo{Version: version, Commit: commit, BuiltAt: date},
	}

	// Build the single service bundle. A repo-less boot serves the web UI
	// + global APIs only; everything else reports degraded until the
	// daemon is restarted inside a repo.
	svc := server.Services{
		Stats: statsStore,
		Deps:  depsRegistry,
		Degraded: map[string]string{
			"repos": "no repo adopted yet — run 'mycel up' inside a git repo, or create an agent with a repo path",
		},
	}
	if ws != nil {
		built, buildErr := server.BuildServices(ctx, globals, ws.RootDir)
		if buildErr != nil {
			return fmt.Errorf("build services: %w", buildErr)
		}
		defer built.Close() //nolint:errcheck // best-effort
		bcCodeServer.SetWorkspaceRoot(ws.RootDir)
		svc = *built
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
	if ws != nil {
		updateAgentHookPorts(ws, cfg.Addr)
	}

	srv := server.New(cfg, svc, globalHub, server.WebDist())
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
