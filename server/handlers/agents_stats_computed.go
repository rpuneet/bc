package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/mycel/pkg/agent"
	bcstats "github.com/rpuneet/mycel/pkg/stats"
)

// computedStatsSampler is the shared TmuxSampler used by the computed-stats
// endpoint. Shared instance keeps the "no per-process network stats"
// warning to once per process, matching the behavior of the collector.
var (
	computedStatsSampler     *bcstats.TmuxSampler
	computedStatsSamplerOnce sync.Once
)

func getComputedStatsSampler() *bcstats.TmuxSampler {
	computedStatsSamplerOnce.Do(func() {
		computedStatsSampler = bcstats.NewTmuxSampler(bcstats.DefaultTmuxProcRunner{})
	})
	return computedStatsSampler
}

// computedStatsResponse is the JSON payload returned by the computed stats endpoint.
// All fields are computed from hook events in the SQLite event store — no TimescaleDB required.
// Token and cost fields are populated from the cost store when available.
// CPU and memory are sampled live via ps aux (fallback when TimescaleDB is unavailable).
type computedStatsResponse struct { //nolint:govet // field order matches JSON contract
	ToolBreakdown         map[string]int `json:"tool_breakdown"`
	LastActive            string         `json:"last_active,omitempty"`
	NetworkNote           string         `json:"network_note,omitempty"`
	TotalEvents           int            `json:"total_events"`
	ToolCalls             int            `json:"tool_calls"`
	WebCalls              int            `json:"web_calls"`
	ChannelSent           int            `json:"channel_sent"`
	ChannelReceived       int            `json:"channel_received"`
	SessionDurationSec    int64          `json:"session_duration_sec"`
	DiskBytes             int64          `json:"disk_bytes"`
	EstimatedNetworkBytes int64          `json:"estimated_network_bytes"`
	InputTokens           int64          `json:"input_tokens"`
	OutputTokens          int64          `json:"output_tokens"`
	Tokens                int            `json:"tokens"`
	CostUSD               float64        `json:"cost_usd"`
	CPUPercent            float64        `json:"cpu_percent"`
	MemUsedBytes          int64          `json:"mem_used_bytes"`
}

// agentComputedStats computes activity statistics from hook events for a single agent.
// GET /api/agents/{name}/stats-computed
//
// This endpoint works without TimescaleDB: it reads the SQLite event store
// and aggregates tool call counts, session duration, and last-active timestamp.
// It also walks the agent's worktree to compute disk usage, and counts
// channel-related events for sent/received message activity.
func (h *AgentHandler) agentComputedStats(w http.ResponseWriter, r *http.Request, name string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	svc := h.svc

	// Compute disk usage from the agent's worktree regardless of events.
	wtPath := svc.Manager().WorktreePath(name)
	var diskBytes int64
	if wtPath != "" {
		_ = filepath.Walk(wtPath, func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				diskBytes += info.Size()
			}
			return nil
		})
	}

	zero := computedStatsResponse{
		ToolBreakdown: map[string]int{},
		DiskBytes:     diskBytes,
		NetworkNote:   "Network tracking requires container runtime",
	}

	if h.events == nil {
		writeJSON(w, http.StatusOK, zero)
		return
	}

	evts, err := h.events.ReadByAgent(name)
	if err != nil {
		httpInternalError(w, "read events", err)
		return
	}

	if len(evts) == 0 {
		writeJSON(w, http.StatusOK, zero)
		return
	}

	var (
		totalEvents     int
		toolCalls       int
		toolBreakdown   = make(map[string]int)
		firstTime       time.Time
		lastTime        time.Time
		channelSent     int
		channelReceived int
	)

	for _, e := range evts {
		// Count channel activity from all events (not just hook.*).
		eType := string(e.Type)
		if strings.HasPrefix(eType, "channel.") || eType == "ChannelMessage" {
			channelReceived++
		}
		if eType == "ChannelSent" || eType == "hook.ChannelSent" {
			channelSent++
		}

		// Only count hook.* events for session/tool stats.
		if !strings.HasPrefix(eType, "hook.") {
			continue
		}
		totalEvents++

		// Track time range for session duration.
		if firstTime.IsZero() || e.Timestamp.Before(firstTime) {
			firstTime = e.Timestamp
		}
		if e.Timestamp.After(lastTime) {
			lastTime = e.Timestamp
		}

		// Count PreToolUse events and accumulate tool breakdown.
		eventName := strings.TrimPrefix(eType, "hook.")
		if eventName == "PreToolUse" {
			toolCalls++
			if e.Data != nil {
				if toolName, ok := e.Data["tool_name"].(string); ok && toolName != "" {
					toolBreakdown[toolName]++
				}
			}
		}
	}

	// Count web tool calls (WebFetch / WebSearch) from PostToolUse events to estimate network I/O.
	var webCalls int
	var estimatedNetworkBytes int64
	for _, e := range evts {
		eType := string(e.Type)
		if !strings.HasSuffix(eType, "PostToolUse") {
			continue
		}
		toolName := ""
		if e.Data != nil {
			toolName, _ = e.Data["tool_name"].(string)
		}
		if toolName == "" && e.Message != "" {
			var msgData map[string]any
			if json.Unmarshal([]byte(e.Message), &msgData) == nil {
				toolName, _ = msgData["tool_name"].(string)
			}
		}
		if toolName == "WebFetch" || toolName == "WebSearch" {
			webCalls++
			// Estimate ~50KB per web request
			estimatedNetworkBytes += 50 * 1024
		}
	}

	var sessionDurationSec int64
	if !firstTime.IsZero() && !lastTime.IsZero() {
		sessionDurationSec = int64(lastTime.Sub(firstTime).Seconds())
	}

	// "Last active" must reflect the newest signal available. Hook events
	// can lag or be pruned, so take the later of the newest hook event and
	// the agent record's UpdatedAt (which moves on every state transition).
	if a, aErr := svc.Get(r.Context(), name); aErr == nil && a != nil && a.UpdatedAt.After(lastTime) {
		lastTime = a.UpdatedAt
	}

	lastActive := ""
	if !lastTime.IsZero() {
		lastActive = lastTime.UTC().Format(time.RFC3339)
	}

	// Query cost store for token/cost data for this agent.
	// Cost records may use different agent ID formats:
	// "clever-urial" (short), "bc-trade-clever-urial" (worktree name), etc.
	var inputTokens, outputTokens int64
	var costUSD float64
	if h.costs != nil {
		// Try the agent name as-is first
		if summary, err := h.costs.AgentSummary(r.Context(), name); err == nil && summary != nil {
			inputTokens = summary.InputTokens
			outputTokens = summary.OutputTokens
			costUSD = summary.TotalCostUSD
		}
		// If no results, try common worktree-prefixed name patterns
		if inputTokens == 0 && outputTokens == 0 {
			wtPath := svc.Manager().WorktreePath(name)
			if wtPath != "" {
				// Extract the worktree directory name (e.g. "bc-trade-clever-urial")
				wtDir := filepath.Base(wtPath)
				if wtDir != name && wtDir != "." {
					if summary, err := h.costs.AgentSummary(r.Context(), wtDir); err == nil && summary != nil {
						inputTokens = summary.InputTokens
						outputTokens = summary.OutputTokens
						costUSD = summary.TotalCostUSD
					}
				}
			}
		}
	}

	// Live CPU/mem from ps aux — fallback when TimescaleDB is unavailable.
	cpuPercent, memUsedBytes := liveAgentCPUMem(r.Context(), name, svc)

	networkNote := "Network tracking requires container runtime"
	if webCalls > 0 {
		networkNote = "~" + strconv.Itoa(webCalls) + " web requests, est. " + strconv.FormatInt(estimatedNetworkBytes/1024, 10) + "KB"
	}

	writeJSON(w, http.StatusOK, computedStatsResponse{
		TotalEvents:           totalEvents,
		ToolCalls:             toolCalls,
		WebCalls:              webCalls,
		ToolBreakdown:         toolBreakdown,
		SessionDurationSec:    sessionDurationSec,
		LastActive:            lastActive,
		InputTokens:           inputTokens,
		OutputTokens:          outputTokens,
		Tokens:                int(inputTokens + outputTokens),
		CostUSD:               costUSD,
		DiskBytes:             diskBytes,
		EstimatedNetworkBytes: estimatedNetworkBytes,
		ChannelSent:           channelSent,
		ChannelReceived:       channelReceived,
		NetworkNote:           networkNote,
		CPUPercent:            cpuPercent,
		MemUsedBytes:          memUsedBytes,
	})
}

// liveAgentCPUMem samples CPU% and resident memory for the processes
// belonging to a named agent. It delegates the PID-tree walk to
// pkg/stats.TmuxSampler so the endpoint and the background collector
// share one implementation.
//
// Returns (cpuPercent, memUsedBytes). Non-tmux agents (docker) return 0
// from here — their live metrics are served from agent_stats rows
// written by runContainerStatsCollector.
func liveAgentCPUMem(ctx context.Context, name string, svc *agent.AgentService) (float64, int64) {
	if svc == nil {
		return 0, 0
	}

	a, err := svc.Get(ctx, name)
	if err != nil || a == nil {
		return 0, 0
	}

	if a.RuntimeBackend != "" && a.RuntimeBackend != "tmux" {
		return 0, 0
	}

	sessionName := a.Session
	if sessionName == "" {
		sessionName = name
	}

	sample, err := getComputedStatsSampler().Sample(ctx, sessionName, name)
	if err != nil {
		return 0, 0
	}
	return sample.CPUPercent, sample.MemBytes
}
