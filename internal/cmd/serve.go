package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	bcagent "github.com/rpuneet/bc/pkg/agent"
	bccontainer "github.com/rpuneet/bc/pkg/container"
	bccost "github.com/rpuneet/bc/pkg/cost"
	bccron "github.com/rpuneet/bc/pkg/cron"
	bcdb "github.com/rpuneet/bc/pkg/db"
	bcevents "github.com/rpuneet/bc/pkg/events"
	bcgateway "github.com/rpuneet/bc/pkg/gateway"
	bcdiscord "github.com/rpuneet/bc/pkg/gateway/discord"
	bcslack "github.com/rpuneet/bc/pkg/gateway/slack"
	bctelegram "github.com/rpuneet/bc/pkg/gateway/telegram"
	"github.com/rpuneet/bc/pkg/log"
	bcmcp "github.com/rpuneet/bc/pkg/mcp"
	bcnotify "github.com/rpuneet/bc/pkg/notify"
	"github.com/rpuneet/bc/pkg/provider"
	bcsecret "github.com/rpuneet/bc/pkg/secret"
	bcstats "github.com/rpuneet/bc/pkg/stats"
	bctoken "github.com/rpuneet/bc/pkg/token"
	bctool "github.com/rpuneet/bc/pkg/tool"
	bcworkspace "github.com/rpuneet/bc/pkg/workspace"
	"github.com/rpuneet/bc/server"
	bcws "github.com/rpuneet/bc/server/ws"
)

// RunServer starts the bc server (formerly bcd) in the foreground.
// It loads the workspace, wires up all services, and blocks until
// the context is canceled or a signal is received.
func RunServer(addr, wsRoot, corsOrigin, apiKey string) error {
	ws, err := bcworkspace.Load(wsRoot)
	if err != nil {
		ws, err = bcworkspace.Init(wsRoot)
		if err != nil {
			return fmt.Errorf("init workspace %s: %w", wsRoot, err)
		}
	}

	// Set up shared database connection for all stores.
	// Settings.json storage config is the source of truth; DATABASE_URL env var overrides for Docker.
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
		log.Info("shared database ready", "driver", sharedDriver, "config_driver", ws.Config.Storage.Default)
	}

	pidPath := filepath.Join(ws.StateDir(), "bcd.pid")
	if err := writePID(pidPath); err != nil {
		log.Warn("failed to write PID file", "path", pidPath, "error", err)
	}
	defer os.Remove(pidPath) //nolint:errcheck // best-effort cleanup

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// SSE event persistence writer
	eventsJSONL := filepath.Join(ws.StateDir(), "events.jsonl")
	eventWriter := bcevents.NewJSONLWriter(eventsJSONL, 0)

	hub := bcws.NewHub()
	go hub.Run()
	defer hub.Stop()

	agentMgr, containerBackend, agentErr := newServerAgentManager(ws)
	if agentErr != nil {
		return fmt.Errorf("agent manager: %w", agentErr)
	}
	if err := agentMgr.LoadState(); err != nil {
		log.Warn("failed to load agent state", "error", err)
	}
	defer agentMgr.Close() //nolint:errcheck // best-effort
	if ws.RoleManager != nil {
		agentMgr.SetRoleManager(ws.RoleManager)
	}
	agentSvc := bcagent.NewAgentService(agentMgr, hub, nil)

	// Background container metrics collector: periodically samples Docker
	// resource usage via the container.Backend API and persists records into
	// the SQLite agent_stats table. This feeds the /api/agents/{name}/stats
	// endpoint so the dashboard shows real CPU/memory/disk/IO per agent.
	if containerBackend != nil {
		go runContainerStatsCollector(ctx, containerBackend, agentMgr)
	}

	// Background tool health loop: periodically checks that each running
	// agent's tool binary is still available, marking agents as stuck if
	// their tool disappears. Runs every 30s.
	agentMgr.StartToolHealthLoop(ctx, bcagent.DefaultToolHealthInterval)
	defer agentMgr.StopToolHealthLoop()

	var costStore *bccost.Store
	var costImporter *bccost.Importer
	if cs, err := bccost.OpenStore(ws.RootDir); err != nil {
		log.Warn("cost store unavailable", "error", err)
	} else {
		costStore = cs
		defer cs.Close() //nolint:errcheck // best-effort

		costImporter = bccost.NewImporter(cs, ws.RootDir)
		go func() {
			if n, err := costImporter.ImportAll(ctx); err != nil {
				log.Warn("cost import failed", "error", err)
			} else if n > 0 {
				log.Info("cost import: imported records", "count", n)
			}
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if n, err := costImporter.ImportAll(ctx); err != nil {
						log.Warn("cost import failed", "error", err)
					} else if n > 0 {
						log.Info("cost import: imported records", "count", n)
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	var cronStore *bccron.Store
	var cronScheduler *bccron.Scheduler
	if cr, err := bccron.Open(ws.RootDir); err != nil {
		log.Warn("cron store unavailable", "error", err)
	} else {
		cronStore = cr
		defer cr.Close() //nolint:errcheck // best-effort

		cronLogDir := filepath.Join(ws.RootDir, ".bc", "logs", "cron")
		cronScheduler = bccron.NewSchedulerWithConfig(cr, cronLogDir,
			ws.Config.Cron.PollIntervalSeconds, ws.Config.Cron.JobTimeoutSeconds)
		go cronScheduler.Run(ctx)
	}

	var secretStore *bcsecret.Store
	passphrase, passphraseErr := bcsecret.Passphrase()
	if passphraseErr != nil {
		log.Warn("secret passphrase unavailable — secret store disabled", "error", passphraseErr)
	}
	if passphraseErr == nil {
		if ss, err := bcsecret.NewStore(ws.RootDir, passphrase); err != nil {
			log.Warn("secret store unavailable", "error", err)
		} else {
			secretStore = ss
			defer ss.Close() //nolint:errcheck // best-effort
		}
	}

	var mcpStore *bcmcp.Store
	if ms, err := bcmcp.NewStore(ws.RootDir); err != nil {
		log.Warn("mcp store unavailable", "error", err)
	} else {
		mcpStore = ms
		defer ms.Close() //nolint:errcheck // best-effort
	}

	var toolStore *bctool.Store
	ts := bctool.NewStore(ws.StateDir())
	if err := ts.Open(); err != nil {
		log.Warn("tool store unavailable", "error", err)
	} else {
		toolStore = ts
		defer ts.Close() //nolint:errcheck // best-effort
	}

	var eventLog bcevents.EventStore
	if el, err := bcevents.OpenLog(ws.RootDir, filepath.Join(ws.StateDir(), "state.db")); err != nil {
		log.Warn("event log unavailable", "error", err)
	} else {
		eventLog = el
		defer el.Close() //nolint:errcheck // best-effort
	}

	// TimescaleDB stats store — always attempt to connect. Uses STATS_DATABASE_URL
	// env var if set, otherwise falls back to the default bc-db container DSN.
	// If connection fails, the stats endpoints return empty data gracefully.
	var statsStore *bcstats.Store
	{
		dsn := bcstats.StatsDSN()
		if ss, err := bcstats.NewStore(dsn); err != nil {
			log.Warn("stats store unavailable (TimescaleDB)", "error", err, "dsn", dsn)
		} else {
			statsStore = ss
			defer ss.Close() //nolint:errcheck // best-effort
			log.Info("stats store: using TimescaleDB", "dsn", dsn)

			// Background system metrics collector
			go runStatsCollector(ctx, ss, agentSvc, ws)
		}
	}

	var notifyService *bcnotify.Service
	if ns, err := bcnotify.OpenStore(ws.RootDir); err != nil {
		log.Warn("notify store unavailable", "error", err)
	} else {
		notifyService = bcnotify.NewService(ns, agentSvc, hub)
	}

	// Periodic delivery log pruning — keep last 1000 entries per channel.
	if notifyService != nil {
		go func() {
			ticker := time.NewTicker(1 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := notifyService.PruneOldActivity(ctx, 1000); err != nil {
						log.Warn("notify: periodic prune failed", "error", err)
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Gateway manager for external messaging platforms (Telegram, Discord, Slack).
	var gwManager *bcgateway.Manager
	{
		gw := ws.Config.Gateways
		hasAdapters := false

		// Check if any gateway is enabled
		if (gw.Telegram != nil && gw.Telegram.Enabled && gw.Telegram.BotToken != "") ||
			(gw.Discord != nil && gw.Discord.Enabled && gw.Discord.BotToken != "") ||
			(gw.Slack != nil && gw.Slack.Enabled && gw.Slack.BotToken != "") {
			gwManager = bcgateway.NewManager()
			// Wire channel persistence before Start so channels survive restarts.
			if notifyService != nil {
				gwManager.SetChannelStore(&channelPersister{store: notifyService.Store()})
			}
		}

		// Telegram adapter
		if gwManager != nil && gw.Telegram != nil && gw.Telegram.Enabled && gw.Telegram.BotToken != "" {
			tgAdapter := bctelegram.New(gw.Telegram.BotToken, gw.Telegram.Mode)
			if err := tgAdapter.DiscoverViaUpdate(); err != nil {
				log.Warn("telegram: discovery failed", "error", err)
			}
			gwManager.Register(tgAdapter)
			hasAdapters = true
			log.Info("gateway: telegram adapter registered")
		}

		// Discord adapter
		if gwManager != nil && gw.Discord != nil && gw.Discord.Enabled && gw.Discord.BotToken != "" {
			dcAdapter := bcdiscord.New(gw.Discord.BotToken)
			gwManager.Register(dcAdapter)
			hasAdapters = true
			log.Info("gateway: discord adapter registered")
		}

		// Slack adapter (Socket Mode)
		if gwManager != nil && gw.Slack != nil && gw.Slack.Enabled && gw.Slack.BotToken != "" && gw.Slack.AppToken != "" {
			slAdapter := bcslack.New(gw.Slack.BotToken, gw.Slack.AppToken)
			gwManager.Register(slAdapter)
			hasAdapters = true
			log.Info("gateway: slack adapter registered")
		}

		// Wire inbound handler and start
		if gwManager != nil && hasAdapters {
			go func() {
				if err := gwManager.Start(ctx); err != nil && ctx.Err() == nil {
					log.Error("gateway manager stopped", "error", err)
				}
			}()
		}
	}

	svc := server.Services{
		Agents:        agentSvc,
		Notify:        notifyService,
		Costs:         costStore,
		CostImporter:  costImporter,
		Cron:          cronStore,
		CronScheduler: cronScheduler,
		Secrets:       secretStore,
		MCP:           mcpStore,
		Tools:         toolStore,
		Stats:         statsStore,
		EventLog:      eventLog,
		EventWriter:   eventWriter,
		WS:            ws,
		Gateway:       gwManager,
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
		Commit:  commit,
		BuiltAt: date,
	}

	srv := server.New(cfg, svc, hub, server.WebDist())
	return srv.Start(ctx)
}

func newServerAgentManager(ws *bcworkspace.Workspace) (*bcagent.Manager, *bccontainer.Backend, error) {
	var wsCfg bcworkspace.DockerRuntimeConfig
	if ws.Config != nil {
		wsCfg = ws.Config.Runtime.Docker
	}
	dockerCfg := bccontainer.ConfigFromWorkspace(wsCfg)
	be, err := bccontainer.NewBackend(dockerCfg, "bc-", ws.RootDir, provider.DefaultRegistry)
	if err != nil {
		log.Warn("Docker not available — agents will use tmux runtime only", "error", err)
		return bcagent.NewWorkspaceManager(ws.AgentsDir(), ws.RootDir), nil, nil
	}
	mgr := bcagent.NewWorkspaceManagerWithRuntime(ws.AgentsDir(), ws.RootDir, be, "docker")
	return mgr, be, nil
}

func writePID(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0600)
}

// dockerStatsEntry represents one line of `docker stats --no-stream --format '{{json .}}'`.
type dockerStatsEntry struct {
	Container string `json:"Container"` // container ID
	Name      string `json:"Name"`      // container name
	CPUPerc   string `json:"CPUPerc"`
	MemUsage  string `json:"MemUsage"`
	MemPerc   string `json:"MemPerc"`
	NetIO     string `json:"NetIO"`
	BlockIO   string `json:"BlockIO"`
}

// runStatsCollector periodically samples system and agent metrics into TimescaleDB.
// It shells out to `docker stats --no-stream` every 30s, classifies containers as
// system (bc-db, *-daemon) or agent (bc-*-agent-*), and records resource usage.
func runStatsCollector(ctx context.Context, ss *bcstats.Store, agents *bcagent.AgentService, ws *bcworkspace.Workspace) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Track per-agent watermarks to avoid re-recording token entries.
	tokenWatermarks := make(map[string]time.Time)

	// Build an agent lookup by name for enriching agent metrics.
	agentLookup := func() map[string]*bcagent.Agent {
		if agents == nil {
			return nil
		}
		list, err := agents.List(ctx, bcagent.ListOptions{})
		if err != nil {
			log.Debug("stats: agent list failed", "error", err)
			return nil
		}
		m := make(map[string]*bcagent.Agent, len(list))
		for _, a := range list {
			m[a.Name] = a
		}
		return m
	}

	for {
		select {
		case <-ticker.C:
			now := time.Now()

			// -- docker stats --
			entries := collectDockerStats(ctx)
			agentsByName := agentLookup()

			for _, e := range entries {
				cpu := parsePercent(e.CPUPerc)
				memUsed, memLimit := parseMemUsage(e.MemUsage)
				memPct := parsePercent(e.MemPerc)
				netRx, netTx := parseIOPair(e.NetIO)
				diskR, diskW := parseIOPair(e.BlockIO)

				name := e.Name

				switch {
				case isSystemContainer(name):
					if err := ss.RecordSystem(ctx, bcstats.SystemMetric{
						Time:           now,
						SystemName:     name,
						CPUPercent:     cpu,
						MemUsedBytes:   memUsed,
						MemLimitBytes:  memLimit,
						MemPercent:     memPct,
						NetRxBytes:     netRx,
						NetTxBytes:     netTx,
						DiskReadBytes:  diskR,
						DiskWriteBytes: diskW,
					}); err != nil {
						log.Debug("stats: record system metric", "name", name, "error", err)
					}

				case isAgentContainer(name):
					agentName := extractAgentName(name)
					var role, tool, state string
					if a, ok := agentsByName[agentName]; ok {
						role = string(a.Role)
						tool = a.Tool
						state = string(a.State)
					}
					if err := ss.RecordAgent(ctx, bcstats.AgentMetric{
						Time:           now,
						AgentName:      agentName,
						Role:           role,
						Tool:           tool,
						Runtime:        "docker",
						State:          state,
						CPUPercent:     cpu,
						MemUsedBytes:   memUsed,
						MemLimitBytes:  memLimit,
						MemPercent:     memPct,
						NetRxBytes:     netRx,
						NetTxBytes:     netTx,
						DiskReadBytes:  diskR,
						DiskWriteBytes: diskW,
					}); err != nil {
						log.Debug("stats: record agent metric", "agent", agentName, "error", err)
					}
				}
			}

			// -- token metrics from agent JSONL sessions --
			if ws != nil {
				agentsDir := filepath.Join(ws.RootDir, ".bc", "agents")
				for agentName := range agentsByName {
					entries, tokenErr := bctoken.CollectAgentSince(agentsDir, agentName, tokenWatermarks[agentName])
					if tokenErr != nil || len(entries) == 0 {
						continue
					}
					var latestSuccess time.Time
					for _, e := range entries {
						if err := ss.RecordToken(ctx, bcstats.TokenMetric{
							Time:         e.Timestamp,
							AgentName:    e.AgentName,
							Model:        e.Model,
							InputTokens:  e.InputTokens,
							OutputTokens: e.OutputTokens,
							CacheRead:    e.CacheRead,
							CacheCreate:  e.CacheCreate,
						}); err != nil {
							log.Debug("stats: record token metric", "agent", agentName, "error", err)
							continue // don't advance watermark past failed inserts
						}
						if e.Timestamp.After(latestSuccess) {
							latestSuccess = e.Timestamp
						}
					}
					if !latestSuccess.IsZero() {
						tokenWatermarks[agentName] = latestSuccess
					}
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

// collectDockerStats runs `docker stats --no-stream` and returns parsed entries.
func collectDockerStats(ctx context.Context) []dockerStatsEntry {
	cmd := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{json .}}")
	out, err := cmd.Output()
	if err != nil {
		log.Debug("stats: docker stats failed", "error", err)
		return nil
	}

	var entries []dockerStatsEntry
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e dockerStatsEntry
		if err := json.Unmarshal(line, &e); err != nil {
			log.Debug("stats: parse docker stats line", "error", err)
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

// isSystemContainer returns true for bc-db, bc-playwright, or *-daemon containers.
func isSystemContainer(name string) bool {
	if name == "bc-db" || name == "bc-playwright" {
		return true
	}
	return strings.Contains(name, "-daemon")
}

// isAgentContainer returns true for bc-<hash>-<name> containers that are NOT system containers.
func isAgentContainer(name string) bool {
	if !strings.HasPrefix(name, "bc-") {
		return false
	}
	return !isSystemContainer(name)
}

// extractAgentName extracts the agent name from a container name like bc-<hash>-<name>.
// The hash is always 6 hex chars, so prefix is "bc-XXXXXX-" (10 chars).
func extractAgentName(containerName string) string {
	// bc-a08b6d-agent-01 -> agent-01
	if len(containerName) > 10 && strings.HasPrefix(containerName, "bc-") {
		return containerName[10:]
	}
	return containerName
}

// runContainerStatsCollector periodically samples Docker container resource
// metrics via the container.Backend API (Docker socket) and persists them
// into the SQLite agent_stats table. This feeds GET /api/agents/{name}/stats
// so the dashboard shows real CPU/memory/disk/IO per agent without requiring
// the optional TimescaleDB stats store.
func runContainerStatsCollector(ctx context.Context, be *bccontainer.Backend, mgr *bcagent.Manager) {
	const interval = 30 * time.Second
	const bytesToMB = 1024 * 1024

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			allStats, err := be.AllStats(ctx)
			if err != nil {
				log.Debug("container stats collection failed", "error", err)
				continue
			}
			for _, cs := range allStats {
				agentName := extractAgentName(cs.Name)
				rec := &bcagent.AgentStatsRecord{
					AgentName:    agentName,
					CollectedAt:  time.Now().UTC(),
					CPUPct:       cs.CPUPercent,
					MemUsedMB:    float64(cs.MemoryUsed) / bytesToMB,
					MemLimitMB:   float64(cs.MemoryLimit) / bytesToMB,
					NetRxMB:      float64(cs.NetRx) / bytesToMB,
					NetTxMB:      float64(cs.NetTx) / bytesToMB,
					BlockReadMB:  float64(cs.DiskRead) / bytesToMB,
					BlockWriteMB: float64(cs.DiskWrite) / bytesToMB,
				}
				if err := mgr.RecordAgentStats(rec); err != nil {
					log.Debug("failed to record container stats", "agent", agentName, "error", err)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

// parsePercent parses a percentage string like "5.00%" into a float64.
func parsePercent(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	v, _ := strconv.ParseFloat(s, 64) //nolint:errcheck // returns 0 on failure
	return v
}

// parseMemUsage splits "100MiB / 2GiB" into (used bytes, limit bytes).
func parseMemUsage(s string) (int64, int64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return parseBytes(strings.TrimSpace(parts[0])), parseBytes(strings.TrimSpace(parts[1]))
}

// parseIOPair splits an IO string like "1.5kB / 2.3kB" into (in, out) bytes.
func parseIOPair(s string) (int64, int64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return parseBytes(strings.TrimSpace(parts[0])), parseBytes(strings.TrimSpace(parts[1]))
}

// parseBytes converts a human-readable byte string (e.g. "100MiB", "1.5kB", "0B")
// into an int64 byte count.
func parseBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Find where the numeric part ends and the unit begins.
	unitIdx := len(s)
	for i, c := range s {
		if c != '.' && (c < '0' || c > '9') {
			unitIdx = i
			break
		}
	}

	numStr := s[:unitIdx]
	unit := strings.ToLower(s[unitIdx:])

	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}

	switch unit {
	case "b":
		return int64(val)
	case "kb":
		return int64(val * 1000)
	case "mb":
		return int64(val * 1000 * 1000)
	case "gb":
		return int64(val * 1000 * 1000 * 1000)
	case "tb":
		return int64(val * 1000 * 1000 * 1000 * 1000)
	case "kib":
		return int64(val * 1024)
	case "mib":
		return int64(val * 1024 * 1024)
	case "gib":
		return int64(val * 1024 * 1024 * 1024)
	case "tib":
		return int64(val * 1024 * 1024 * 1024 * 1024)
	default:
		return int64(val)
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
