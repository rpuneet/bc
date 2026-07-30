package handlers

import (
	"math"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/stats"
	"github.com/rpuneet/mycel/pkg/tool"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// systemMetrics holds platform-dependent system resource metrics.
type systemMetrics struct {
	MemoryTotalBytes uint64
	MemoryUsedBytes  uint64
	MemoryPercent    float64
	DiskTotalBytes   uint64
	DiskUsedBytes    uint64
	DiskPercent      float64
	CPUUsagePercent  float64
}

// serverStartTime is used to compute uptime.
var serverStartTime = time.Now() //nolint:gochecknoglobals // intentional: tracks server start

// StatsHandler handles /api/stats routes.
type StatsHandler struct {
	agents     *agent.AgentService
	costs      *cost.Service
	tools      *tool.Store
	ws         *workspace.Workspace
	statsStore *stats.Store
	gw         *gateway.Manager
	notifySvc  *notify.Service
}

// NewStatsHandler creates a StatsHandler.
func NewStatsHandler(
	agents *agent.AgentService,
	costs *cost.Service,
	tools *tool.Store,
	ws *workspace.Workspace,
	statsStore *stats.Store,
) *StatsHandler {
	return &StatsHandler{
		agents:     agents,
		costs:      costs,
		tools:      tools,
		ws:         ws,
		statsStore: statsStore,
	}
}

// Register mounts stats routes on mux.
// SetGateway sets the gateway manager for channel count.
func (h *StatsHandler) SetGateway(gw *gateway.Manager) { h.gw = gw }

// SetNotify sets the notify service for subscription count.
func (h *StatsHandler) SetNotify(svc *notify.Service) { h.notifySvc = svc }

func (h *StatsHandler) Register(mux *http.ServeMux) {
	// Legacy summary endpoints
	mux.HandleFunc("/api/stats/system", h.system)
	mux.HandleFunc("/api/stats/summary", h.summary)
	mux.HandleFunc("/api/stats/channels", h.statsChannels)

	// System info endpoint
	mux.HandleFunc("/api/system/info", h.systemInfo)

	// New per-resource timeseries endpoints
	h.RegisterSystemStats(mux)
	h.RegisterAgentStats(mux)
	h.RegisterChannelStats(mux)
}

func (h *StatsHandler) system(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	hostname, _ := os.Hostname() //nolint:errcheck // best-effort

	rootDir := "/"
	if h.ws != nil {
		rootDir = h.ws.RootDir
	}

	metrics := getSystemMetrics(r.Context(), rootDir)

	writeJSON(w, http.StatusOK, map[string]any{
		"hostname":             hostname,
		"os":                   runtime.GOOS,
		"arch":                 runtime.GOARCH,
		"cpus":                 runtime.NumCPU(),
		"cpu_usage_percent":    metrics.CPUUsagePercent,
		"memory_total_bytes":   metrics.MemoryTotalBytes,
		"memory_used_bytes":    metrics.MemoryUsedBytes,
		"memory_usage_percent": metrics.MemoryPercent,
		"disk_total_bytes":     metrics.DiskTotalBytes,
		"disk_used_bytes":      metrics.DiskUsedBytes,
		"disk_usage_percent":   metrics.DiskPercent,
		"go_version":           runtime.Version(),
		"uptime_seconds":       int64(time.Since(serverStartTime).Seconds()),
		"goroutines":           runtime.NumGoroutine(),
	})
}

func (h *StatsHandler) systemInfo(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	hostname, _ := os.Hostname() //nolint:errcheck // best-effort

	writeJSON(w, http.StatusOK, map[string]any{
		"hostname": hostname,
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
	})
}

func (h *StatsHandler) summary(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	ctx := r.Context()

	var agentsTotal, agentsRunning, agentsStopped int
	if h.agents != nil {
		agents, err := h.agents.List(ctx, agent.ListOptions{})
		if err != nil {
			httpInternalError(w, "list agents", err)
			return
		}
		agentsTotal = len(agents)
		for _, a := range agents {
			if a.State == agent.StateStopped || a.State == agent.StateError {
				agentsStopped++
			} else {
				agentsRunning++
			}
		}
	}

	var totalCostUSD float64
	if h.costs != nil {
		summary, err := h.costs.WorkspaceSummary(ctx)
		if err != nil {
			httpInternalError(w, "cost summary", err)
			return
		}
		if summary != nil {
			totalCostUSD = summary.TotalCostUSD
		}
	}

	var rolesTotal int
	if h.ws != nil {
		roles, err := h.ws.RoleManager.LoadAllRoles()
		if err == nil {
			rolesTotal = len(roles)
		}
	}

	var toolsTotal int
	if h.tools != nil {
		tools, err := h.tools.List(ctx)
		if err == nil {
			toolsTotal = len(tools)
		}
	}

	// Channel stats from gateway + notify subscriptions
	var channelsTotal, messagesTotal int
	if h.gw != nil {
		channelsTotal = len(h.gw.DiscoveredSources())
	}
	if h.notifySvc != nil {
		if subs, err := h.notifySvc.AllSubscriptions(ctx); err == nil {
			chSet := make(map[string]bool)
			for _, s := range subs {
				chSet[s.Channel] = true
			}
			if len(chSet) > channelsTotal {
				channelsTotal = len(chSet)
			}
		}
		if count, err := h.notifySvc.Store().TotalMessageCount(ctx); err == nil {
			messagesTotal = count
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"agents_total":   agentsTotal,
		"agents_running": agentsRunning,
		"agents_stopped": agentsStopped,
		"channels_total": channelsTotal,
		"messages_total": messagesTotal,
		"total_cost_usd": totalCostUSD,
		"roles_total":    rolesTotal,
		"tools_total":    toolsTotal,
		"uptime_seconds": int64(time.Since(serverStartTime).Seconds()),
	})
}

// roundTo rounds f to the given number of decimal places.
func roundTo(f float64, places int) float64 {
	shift := math.Pow(10, float64(places))
	return math.Round(f*shift) / shift
}
