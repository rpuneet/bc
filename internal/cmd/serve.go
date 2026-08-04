package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	dbpkg "github.com/rpuneet/mycel/pkg/db"
	depspkg "github.com/rpuneet/mycel/pkg/deps"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/log"
	mcppkg "github.com/rpuneet/mycel/pkg/mcp"
	secretpkg "github.com/rpuneet/mycel/pkg/secret"
	statspkg "github.com/rpuneet/mycel/pkg/stats"
	templatepkg "github.com/rpuneet/mycel/pkg/template"
	"github.com/rpuneet/mycel/pkg/tmux"
	"github.com/rpuneet/mycel/server"
	wspkg "github.com/rpuneet/mycel/server/ws"
)

// RunServer starts the mycel server (formerly the daemon) in the foreground.
// the daemon is single-tenant: it constructs shared Globals, builds the one
// Services bundle via server.BuildServices, wires handlers, and blocks
// until the context is canceled or a signal is received.
//
// repoRoot is the repo the daemon anchors on — new agents default their
// repo to it. It may be empty: the server then boots against MycelHome
// only (web UI + global APIs; no agent runtime until a repo exists).
// A non-empty repoRoot that isn't initialized yet is bootstrapped in place
// via home.Init — there is no separate init step.
func RunServer(addr, repoRoot, corsOrigin, apiKey string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return RunServerCtx(ctx, addr, repoRoot, corsOrigin, apiKey)
}

// RunServerCtx is RunServer with a caller-controlled lifetime: the server
// runs until ctx is canceled, then shuts down gracefully (services close,
// PID file removed). Used by embedders that own the process lifecycle —
// e.g. the desktop app, which cancels ctx when the window closes.
func RunServerCtx(ctx context.Context, addr, repoRoot, corsOrigin, apiKey string) error {
	// Normalize addr: ":9374" → "127.0.0.1:9374"
	addr = normalizeAddr(addr)

	// Bootstrap-or-load the global mycel state. repoRoot may be empty —
	// the daemon then boots without an anchor repo; agents carry their
	// own repo paths and repos can be added later via the UI/API.
	h, err := home.Open(repoRoot)
	if err != nil {
		return fmt.Errorf("bootstrap mycel: %w", err)
	}
	if h.RootDir == "" {
		log.Info("no anchor repo — agents must name their own repo (add repos via the web UI)")
	}

	// The single global database (<MycelHome>/mycel.db) is opened lazily
	// through pkg/db. Warm the connection eagerly so storage problems
	// surface at boot, and close it at shutdown.
	defer dbpkg.CloseGlobal() //nolint:errcheck
	{
		if _, driver, dbErr := dbpkg.Global(h.Config.DBStorageSettings()); dbErr != nil {
			log.Warn("failed to open global db", "error", dbErr)
		} else {
			configDriver := ""
			if h.Config != nil {
				configDriver = h.Config.Storage.Default
			}
			log.Info("global database ready", "driver", driver, "config_driver", configDriver)
		}
	}

	// Publish the workspace alongside the address, so the next process to start
	// a daemon — the desktop app, which has no working directory to infer one
	// from — serves the same one rather than a namespace matching no running
	// agent's tmux session (#3569).
	if wsErr := home.PublishDaemonWorkspace(h.RootDir); wsErr != nil {
		log.Warn("could not record which workspace this daemon serves — the desktop app may start one that cannot see these agents",
			"workspace", h.RootDir, "error", wsErr)
	}

	pidPath, pidErr := home.DaemonPidPath()
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

	// The one SSE hub — the bundle publishes into it and server.New()
	// mounts it at /api/events.
	globalHub := wspkg.NewHub()
	go globalHub.Run()
	defer globalHub.Stop()

	// Global stats store — TimescaleDB connection shared across repos.
	var statsStore *statspkg.Store
	{
		dsn := statspkg.StatsDSN()
		if ss, err := statspkg.NewStore(dsn); err != nil {
			log.Warn("stats store unavailable (TimescaleDB)", "error", err, "dsn", redactDSN(dsn))
		} else {
			statsStore = ss
			defer ss.Close() //nolint:errcheck // best-effort
			log.Info("stats store: using TimescaleDB", "dsn", redactDSN(dsn))
		}
	}

	// Optional dependencies registry (mycel-db, mycel-code-server, mycel-browser).
	depsRegistry := depspkg.NewRegistry()
	bcCodeServer := depspkg.NewCodeServer(h.RootDir)
	depsRegistry.Register(depspkg.NewDB())
	depsRegistry.Register(bcCodeServer)
	depsRegistry.Register(depspkg.NewBrowser())

	// User-global template store at ~/.mycel/templates/. Seeded on first run;
	// callers may wrap this store with an override directory.
	var templatesStore *templatepkg.Store
	if globalTmplDir, gtErr := home.GlobalTemplatesDir(); gtErr != nil {
		log.Warn("global templates dir unavailable", "error", gtErr)
	} else {
		if _, ensureErr := home.EnsureGlobalDir(); ensureErr != nil {
			log.Warn("ensure global mycel dir", "error", ensureErr)
		}
		if seedErr := templatepkg.SeedDefaults(globalTmplDir); seedErr != nil {
			log.Warn("seed global template defaults", "error", seedErr)
		}
		templatesStore = templatepkg.NewStore(globalTmplDir)
	}

	// User-global secrets vault at ~/.mycel/secrets.vault. A single vault
	// keeps ANTHROPIC_API_KEY and friends visible to every agent.
	var globalVault *secretpkg.Store
	if vaultPath, vpErr := home.GlobalSecretsVault(); vpErr != nil {
		log.Warn("global secrets vault path unavailable", "error", vpErr)
	} else if passphrase, passErr := secretpkg.Passphrase(); passErr != nil {
		log.Warn("secret passphrase unavailable — global vault disabled", "error", passErr)
	} else if gv, openErr := secretpkg.OpenVaultFile(vaultPath, passphrase); openErr != nil {
		log.Warn("global secrets vault unavailable", "error", openErr, "path", vaultPath)
	} else {
		globalVault = gv
		defer gv.Close() //nolint:errcheck // best-effort
	}

	// User-global MCP registry at ~/.mycel/mcps.json. The DB layer still has
	// their own SQLite-backed overrides; handlers and agent spawn logic
	// compose the two at resolve time.
	var mcpGlobal *mcppkg.GlobalStore
	if mcpPath, mpErr := home.GlobalMCPConfig(); mpErr != nil {
		log.Warn("global mcp config path unavailable", "error", mpErr)
	} else {
		mcpGlobal = mcppkg.NewGlobalStore(mcpPath)
	}

	// Costs are source-direct: BuildServices constructs the cost
	// service from provider session files — there is no ledger to open.

	globals := &server.Globals{
		Stats:        statsStore,
		Deps:         depsRegistry,
		Hub:          globalHub,
		Templates:    templatesStore,
		SecretsVault: globalVault,
		MCPGlobal:    mcpGlobal,
		Build:        server.BuildInfo{Version: version, Commit: commit, BuiltAt: date},
	}

	// Build the single service bundle. A repo-less boot still gets the
	// full bundle — agents carry their own repo paths; the anchor repo
	// is only the default for new agents.
	built, buildErr := server.BuildServices(ctx, globals, h.RootDir)
	if buildErr != nil {
		return fmt.Errorf("build services: %w", buildErr)
	}
	defer built.Close() //nolint:errcheck // best-effort
	bcCodeServer.SetRepoRoot(h.RootDir)
	svc := *built

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

	// Bring existing agents' hook configs up to date with the current
	// generators, so agents created against an older daemon stop reporting to
	// the address that was current when they were made.
	if svc.AgentMgr != nil {
		if n := svc.AgentMgr.RefreshActivityConfigs(); n > 0 {
			log.Info("refreshed agent activity configs", "agents", n)
		}
	}

	// Sessions created before session names dropped the repo hash are still
	// running under the old name. Rename them before anything looks for an
	// agent, or a running agent reads as stopped. The registry decides what is
	// a hash and what is part of an agent's name, which shape alone cannot.
	if svc.AgentMgr != nil {
		if n := tmux.NewManager(tmux.DefaultPrefix).AdoptLegacySessions(ctx, svc.AgentMgr.HasAgent); n > 0 {
			log.Info("adopted tmux sessions named by an older version", "sessions", n)
		}
	}

	srv := server.New(cfg, svc, globalHub, server.WebDist())
	return srv.Start(ctx)
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
