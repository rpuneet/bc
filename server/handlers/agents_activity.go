package handlers

import (
	"net/http"
	"strings"
)

// activityItem is one row in the agent activity timeline response.
type activityItem struct { //nolint:govet // field order matches JSON contract
	Data      map[string]any `json:"data,omitempty"`
	Timestamp string         `json:"timestamp"`
	Event     string         `json:"event"`
	Message   string         `json:"message,omitempty"`
}

// agentActivity returns the most recent activity events for an agent, built
// from the append-only event store. Used by the InfoTab Activity timeline.
// GET /api/agents/{name}/activity
func (h *AgentHandler) agentActivity(w http.ResponseWriter, r *http.Request, name string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if h.events == nil {
		// Fall back to empty list rather than erroring — the InfoTab degrades
		// to timestamp-derived timeline if the store is unavailable.
		writeJSON(w, http.StatusOK, []activityItem{})
		return
	}

	evts, err := h.events.ReadByAgent(name)
	if err != nil {
		httpInternalError(w, "read activity", err)
		return
	}

	// Reverse chronological (newest first), cap at 50 entries to keep the
	// timeline readable. The UI handles ordering client-side.
	const maxItems = 50
	out := make([]activityItem, 0, len(evts))
	for i := len(evts) - 1; i >= 0 && len(out) < maxItems; i-- {
		e := evts[i]
		out = append(out, activityItem{
			Timestamp: e.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z"),
			Event:     strings.TrimPrefix(string(e.Type), "agent."),
			Message:   e.Message,
			Data:      e.Data,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
