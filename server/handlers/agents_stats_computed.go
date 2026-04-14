package handlers

import (
	"net/http"
	"strings"
	"time"
)

// computedStatsResponse is the JSON payload returned by the computed stats endpoint.
// All fields are computed from hook events in the SQLite event store — no TimescaleDB required.
type computedStatsResponse struct { //nolint:govet // field order matches JSON contract
	ToolBreakdown     map[string]int `json:"tool_breakdown"`
	LastActive        string         `json:"last_active,omitempty"`
	TotalEvents       int            `json:"total_events"`
	ToolCalls         int            `json:"tool_calls"`
	SessionDurationSec int64         `json:"session_duration_sec"`
	Tokens            int            `json:"tokens"`
	CostUSD           float64        `json:"cost_usd"`
}

// agentComputedStats computes activity statistics from hook events for a single agent.
// GET /api/agents/{name}/stats-computed
//
// This endpoint works without TimescaleDB: it reads the SQLite event store
// and aggregates tool call counts, session duration, and last-active timestamp.
func (h *AgentHandler) agentComputedStats(w http.ResponseWriter, r *http.Request, name string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	zero := computedStatsResponse{ToolBreakdown: map[string]int{}}

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
		totalEvents       int
		toolCalls         int
		toolBreakdown     = make(map[string]int)
		firstTime         time.Time
		lastTime          time.Time
	)

	for _, e := range evts {
		// Only count hook.* events.
		if !strings.HasPrefix(string(e.Type), "hook.") {
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
		eventName := strings.TrimPrefix(string(e.Type), "hook.")
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

	writeJSON(w, http.StatusOK, computedStatsResponse{
		TotalEvents:        totalEvents,
		ToolCalls:          toolCalls,
		ToolBreakdown:      toolBreakdown,
		SessionDurationSec: sessionDurationSec,
		LastActive:         lastActive,
		Tokens:             0,
		CostUSD:            0,
	})
}
