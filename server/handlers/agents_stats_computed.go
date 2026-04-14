package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// computedStatsResponse is the JSON payload returned by the computed stats endpoint.
// All fields are computed from hook events in the SQLite event store — no TimescaleDB required.
// Token and cost fields are populated from the cost store when available.
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
	})
}
