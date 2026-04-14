package handlers

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rpuneet/bc/pkg/agent"
)

// computedStatsResponse is the JSON payload returned by the computed stats endpoint.
// All fields are computed from hook events in the SQLite event store — no TimescaleDB required.
// Token and cost fields are populated from the cost store when available.
// CPU and memory are sampled live via ps aux (fallback when TimescaleDB is unavailable).
type computedStatsResponse struct { //nolint:govet // field order matches JSON contract
	ToolBreakdown      map[string]int `json:"tool_breakdown"`
	LastActive         string         `json:"last_active,omitempty"`
	NetworkNote        string         `json:"network_note,omitempty"`
	TotalEvents        int            `json:"total_events"`
	ToolCalls          int            `json:"tool_calls"`
	ChannelSent        int            `json:"channel_sent"`
	ChannelReceived    int            `json:"channel_received"`
	SessionDurationSec int64          `json:"session_duration_sec"`
	DiskBytes          int64          `json:"disk_bytes"`
	InputTokens        int64          `json:"input_tokens"`
	OutputTokens       int64          `json:"output_tokens"`
	Tokens             int            `json:"tokens"`
	CostUSD            float64        `json:"cost_usd"`
	CPUPercent         float64        `json:"cpu_percent"`
	MemUsedBytes       int64          `json:"mem_used_bytes"`
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

	// Compute disk usage from the agent's worktree regardless of events.
	wtPath := h.svc.Manager().WorktreePath(name)
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

	var sessionDurationSec int64
	if !firstTime.IsZero() && !lastTime.IsZero() {
		sessionDurationSec = int64(lastTime.Sub(firstTime).Seconds())
	}

	lastActive := ""
	if !lastTime.IsZero() {
		lastActive = lastTime.UTC().Format(time.RFC3339)
	}

	// Query cost store for token/cost data for this agent.
	var inputTokens, outputTokens int64
	var costUSD float64
	if h.costs != nil {
		if summary, err := h.costs.AgentSummary(r.Context(), name); err == nil && summary != nil {
			inputTokens = summary.InputTokens
			outputTokens = summary.OutputTokens
			costUSD = summary.TotalCostUSD
		}
	}

	// Live CPU/mem from ps aux — fallback when TimescaleDB is unavailable.
	cpuPercent, memUsedBytes := liveAgentCPUMem(r.Context(), name, h.svc)

	writeJSON(w, http.StatusOK, computedStatsResponse{
		TotalEvents:        totalEvents,
		ToolCalls:          toolCalls,
		ToolBreakdown:      toolBreakdown,
		SessionDurationSec: sessionDurationSec,
		LastActive:         lastActive,
		InputTokens:        inputTokens,
		OutputTokens:       outputTokens,
		Tokens:             int(inputTokens + outputTokens),
		CostUSD:            costUSD,
		DiskBytes:          diskBytes,
		ChannelSent:        channelSent,
		ChannelReceived:    channelReceived,
		NetworkNote:        "Network tracking requires container runtime",
		CPUPercent:         cpuPercent,
		MemUsedBytes:       memUsedBytes,
	})
}

// liveAgentCPUMem samples CPU% and resident memory for the processes belonging
// to a named agent. Uses tmux list-panes to find the pane PID, then walks the
// process tree to find the actual claude/provider process.
// Returns (cpuPercent, memUsedBytes).
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

	// Step 1: Get pane PID from tmux. Try the session name as-is first,
	// then try the bc-prefixed variant (bc-<hash>-<name>).
	var panePIDOut []byte
	tmuxCmd := exec.CommandContext(ctx, "tmux", "list-panes", "-t", sessionName, "-F", "#{pane_pid}") //nolint:gosec
	panePIDOut, tmuxErr := tmuxCmd.Output()
	if tmuxErr != nil {
		// Session name might be just the agent name; actual tmux session is bc-<hash>-<name>
		// List all sessions and find the matching one
		listCmd := exec.CommandContext(ctx, "tmux", "list-sessions", "-F", "#{session_name}") //nolint:gosec
		listOut, listErr := listCmd.Output()
		if listErr != nil {
			return 0, 0
		}
		var fullSession string
		for _, line := range strings.Split(strings.TrimSpace(string(listOut)), "\n") {
			if strings.Contains(line, sessionName) {
				fullSession = strings.TrimSpace(line)
				break
			}
		}
		if fullSession == "" {
			return 0, 0
		}
		retryCmd := exec.CommandContext(ctx, "tmux", "list-panes", "-t", fullSession, "-F", "#{pane_pid}") //nolint:gosec
		panePIDOut, tmuxErr = retryCmd.Output()
		if tmuxErr != nil {
			return 0, 0
		}
	}
	panePIDStr := strings.TrimSpace(string(panePIDOut))
	if panePIDStr == "" {
		return 0, 0
	}

	// Step 2: Find all child PIDs recursively (pane → shell → claude → subprocesses)
	var allPIDs []string
	allPIDs = append(allPIDs, panePIDStr)
	// Walk up to 3 levels deep
	for level := 0; level < 3; level++ {
		var newPIDs []string
		for _, pid := range allPIDs {
			pgrepCmd := exec.CommandContext(ctx, "pgrep", "-P", pid) //nolint:gosec
			pgrepOut, pgrepErr := pgrepCmd.Output()
			if pgrepErr != nil {
				continue
			}
			for _, line := range strings.Split(strings.TrimSpace(string(pgrepOut)), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					newPIDs = append(newPIDs, line)
				}
			}
		}
		if len(newPIDs) == 0 {
			break
		}
		allPIDs = append(allPIDs, newPIDs...)
	}

	if len(allPIDs) <= 1 {
		return 0, 0 // only the pane shell, no child processes
	}

	// Step 3: Get CPU% and RSS for all PIDs via ps
	pidList := strings.Join(allPIDs, ",")
	psCmd := exec.CommandContext(ctx, "ps", "-p", pidList, "-o", "pid,%cpu,rss") //nolint:gosec
	psOut, psErr := psCmd.Output()
	if psErr != nil {
		return 0, 0
	}

	var totalCPU float64
	var totalRSSKB int64

	scanner := bufio.NewScanner(bytes.NewReader(psOut))
	scanner.Scan() // skip header (PID %CPU RSS)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		// Fields: PID %CPU RSS (in KB)
		if cpu, parseErr := strconv.ParseFloat(fields[1], 64); parseErr == nil {
			totalCPU += cpu
		}
		if rss, parseErr := strconv.ParseInt(fields[2], 10, 64); parseErr == nil {
			totalRSSKB += rss
		}
	}

	return totalCPU, totalRSSKB * 1024
}
