// stats_collector.go — background metric collectors.
//
// Split out of internal/cmd/serve.go in phase M2 so the factory in
// build_services.go can start these goroutines as part of a
// Services bundle lifecycle. Behavior is identical to the prior
// implementation; only the package boundary changed.
package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"time"

	agentpkg "github.com/rpuneet/mycel/pkg/agent"
	containerpkg "github.com/rpuneet/mycel/pkg/container"
	"github.com/rpuneet/mycel/pkg/log"
	statspkg "github.com/rpuneet/mycel/pkg/stats"
)

// tmuxSampler is shared across sampling ticks so the one-time
// "no per-process network stats on this platform" warning stays
// one-time across the life of the collector.
var tmuxSampler = statspkg.NewTmuxSampler(statspkg.DefaultTmuxProcRunner{})

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
//
//nolint:gocyclo // Single pass over docker stats output; splitting obscures flow.
func runStatsCollector(ctx context.Context, ss *statspkg.Store, agents *agentpkg.AgentService) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	agentLookup := func() map[string]*agentpkg.Agent {
		if agents == nil {
			return nil
		}
		list, err := agents.List(ctx, agentpkg.ListOptions{})
		if err != nil {
			log.Debug("stats: agent list failed", "error", err)
			return nil
		}
		m := make(map[string]*agentpkg.Agent, len(list))
		for _, a := range list {
			m[a.Name] = a
		}
		return m
	}

	for {
		select {
		case <-ticker.C:
			now := time.Now()

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
					if err := ss.RecordSystem(ctx, statspkg.SystemMetric{
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
					if err := ss.RecordAgent(ctx, statspkg.AgentMetric{
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

			for agentName, a := range agentsByName {
				if a.RuntimeBackend != "tmux" || (a.State != "working" && a.State != "idle") {
					continue
				}
				metric, psErr := collectTmuxAgentStats(ctx, agentName, a)
				if psErr != nil {
					log.Debug("stats: tmux ps failed", "agent", agentName, "error", psErr)
					continue
				}
				if err := ss.RecordAgent(ctx, *metric); err != nil {
					log.Debug("stats: record tmux agent metric", "agent", agentName, "error", err)
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

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

// collectTmuxAgentStats samples CPU% and RSS for a tmux-backed agent by
// walking the PID tree rooted at the tmux pane.
//
// Previous implementation grepped `ps aux` for the session name, which
// both over-matched (any command line containing the name) and
// under-matched (agents whose session string differed from the hashed
// tmux name). We now use pkg/stats.TmuxSampler which resolves the pane
// PID via `tmux list-panes` and walks children via `pgrep -P`.
//
// Network bytes remain zero for tmux agents: macOS has no per-process
// network counters without DTrace privileges, and linux /proc net stats
// are container-wide rather than per-process. The UI renders "Network
// tracking requires container runtime" in that case. We log a one-time
// warning so operators know why the chart is flat.
func collectTmuxAgentStats(ctx context.Context, agentName string, a *agentpkg.Agent) (*statspkg.AgentMetric, error) {
	sessionName := a.Session
	if sessionName == "" {
		sessionName = agentName
	}

	sample, err := tmuxSampler.Sample(ctx, sessionName, agentName)
	if err != nil {
		return nil, err
	}

	tmuxSampler.WarnNoNetworkOnce(func() {
		log.Debug("stats: tmux agents have no per-process network counters; NetRx/NetTx will be 0")
	})

	return &statspkg.AgentMetric{
		Time:         time.Now().UTC(),
		AgentName:    agentName,
		Role:         string(a.Role),
		Tool:         a.Tool,
		Runtime:      "tmux",
		State:        string(a.State),
		CPUPercent:   sample.CPUPercent,
		MemUsedBytes: sample.MemBytes,
	}, nil
}

// runContainerStatsCollector samples Docker container metrics via the
// backend API and persists them in SQLite agent_stats (for /api/agents/{name}/stats).
func runContainerStatsCollector(ctx context.Context, be *containerpkg.Backend, mgr *agentpkg.Manager) {
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
				rec := &agentpkg.AgentStatsRecord{
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

// agentContainerPrefixes are the container-name prefixes that identify an
// agent container.
var agentContainerPrefixes = []string{"mycel-"}

// systemContainerNames identifies infrastructure containers that should be
// recorded via RecordSystem rather than RecordAgent.
func isSystemContainer(name string) bool {
	switch name {
	case "mycel-db", "mycel-playwright":
		return true
	}
	return strings.Contains(name, "-daemon")
}

func isAgentContainer(name string) bool {
	for _, p := range agentContainerPrefixes {
		if !strings.HasPrefix(name, p) {
			continue
		}
		rest := name[len(p):]
		// Require a `<hash>-<agent>` suffix — a bare "mycel-" isn't an agent.
		if i := strings.Index(rest, "-"); i > 0 && i < len(rest)-1 {
			return !isSystemContainer(name)
		}
		return false
	}
	return false
}

// extractAgentName strips the "<prefix><hash>-" segment from a full container
// name (e.g. "mycel-13c6e9-zen-zebra" → "zen-zebra"). Falls back to the raw
// container name when the shape is unexpected.
func extractAgentName(containerName string) string {
	for _, p := range agentContainerPrefixes {
		if !strings.HasPrefix(containerName, p) {
			continue
		}
		rest := containerName[len(p):]
		if i := strings.Index(rest, "-"); i > 0 && i < len(rest)-1 {
			return rest[i+1:]
		}
	}
	return containerName
}

func parsePercent(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	v, _ := strconv.ParseFloat(s, 64) //nolint:errcheck // returns 0 on failure
	return v
}

func parseMemUsage(s string) (int64, int64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return parseBytes(strings.TrimSpace(parts[0])), parseBytes(strings.TrimSpace(parts[1]))
}

func parseIOPair(s string) (int64, int64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return parseBytes(strings.TrimSpace(parts[0])), parseBytes(strings.TrimSpace(parts[1]))
}

func parseBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

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
