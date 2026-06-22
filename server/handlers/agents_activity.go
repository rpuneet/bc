package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/rpuneet/mycel/pkg/events"
)

// activityItem is one row in the agent activity timeline response.
type activityItem struct { //nolint:govet // field order matches JSON contract
	Data      map[string]any `json:"data,omitempty"`
	Timestamp string         `json:"timestamp"`
	Event     string         `json:"event"`
	Message   string         `json:"message,omitempty"`
	Agent     string         `json:"agent,omitempty"`
}

// toActivityItem normalizes an events.Event into the timeline DTO consumed by
// both the per-agent activity timeline and the cross-agent Live hydration
// endpoint. The mapping rules (tool_name + tool_input.command > message, plus
// hook./agent. prefix stripping) live here so both surfaces stay in sync.
func toActivityItem(e events.Event, includeAgent bool) activityItem {
	msg := ""
	if e.Data != nil {
		if toolName, ok := e.Data["tool_name"].(string); ok && toolName != "" {
			msg = toolName
			if toolInput, ok2 := e.Data["tool_input"].(map[string]any); ok2 {
				if cmd, ok3 := toolInput["command"].(string); ok3 && cmd != "" {
					msg += ": " + cmd
				}
			}
		} else if m, ok := e.Data["message"].(string); ok && m != "" {
			msg = m
		}
	}
	out := activityItem{
		Timestamp: e.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z"),
		Event:     strings.TrimPrefix(strings.TrimPrefix(string(e.Type), "hook."), "agent."),
		Message:   msg,
		Data:      e.Data,
	}
	if includeAgent {
		out.Agent = e.Agent
	}
	return out
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

	// Reverse chronological (newest first), cap at requested limit (default 50)
	// to keep the timeline readable. The UI handles ordering client-side.
	maxItems := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 1000 {
			maxItems = n
		}
	}
	out := make([]activityItem, 0, len(evts))
	for i := len(evts) - 1; i >= 0 && len(out) < maxItems; i-- {
		out = append(out, toActivityItem(evts[i], false))
	}
	writeJSON(w, http.StatusOK, out)
}

// activity returns the most recent activity events across ALL agents,
// used by the Live page to hydrate its multi-agent stream on mount so
// the page is never empty on reload (#3138).
// GET /api/agents/activity?limit=N (default 200, max 2000)
func (h *AgentHandler) activity(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if h.events == nil {
		writeJSON(w, http.StatusOK, []activityItem{})
		return
	}

	maxItems := 200
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 2000 {
			maxItems = n
		}
	}

	evts, err := h.events.ReadLast(maxItems)
	if err != nil {
		httpInternalError(w, "read activity", err)
		return
	}

	// Newest-first to match per-agent activity ordering.
	out := make([]activityItem, 0, len(evts))
	for i := len(evts) - 1; i >= 0; i-- {
		e := evts[i]
		if e.Agent == "" {
			continue // skip non-agent events
		}
		out = append(out, toActivityItem(e, true))
	}
	writeJSON(w, http.StatusOK, out)
}
