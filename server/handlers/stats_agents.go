package handlers

import (
	"net/http"
	"strings"

	"github.com/rpuneet/bc/pkg/stats"
)

// RegisterAgentStats mounts agent stats routes on the mux.
func (h *StatsHandler) RegisterAgentStats(mux *http.ServeMux) {
	mux.HandleFunc("/api/agents/stats/latest", h.agentLatest)
	mux.HandleFunc("/api/agents/stats/cpu", h.agentCPU)
	mux.HandleFunc("/api/agents/stats/mem", h.agentMem)
	mux.HandleFunc("/api/agents/stats/disk", h.agentDisk)
	mux.HandleFunc("/api/agents/stats/net", h.agentNet)
	mux.HandleFunc("/api/agents/stats/tokens", h.agentTokens)
	mux.HandleFunc("/api/agents/stats/cost", h.agentCost)
	mux.HandleFunc("/api/agents/stats/summary/", h.agentSummary)
}

// agentSummary returns a combined resource + token + cost summary for a single agent.
// GET /api/agents/stats/summary/{name}?from=...&to=...&interval=...
func (h *StatsHandler) agentSummary(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/agents/stats/summary/")
	if name == "" {
		httpError(w, "agent name required", http.StatusBadRequest)
		return
	}

	if h.statsStore == nil {
		httpError(w, "stats unavailable", http.StatusServiceUnavailable)
		return
	}

	sq := parseStatsQuery(r)
	summary, err := h.statsStore.QueryAgentSummary(r.Context(), name, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query agent summary", err)
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// agentLatest returns the most recent metric sample for each agent.
func (h *StatsHandler) agentLatest(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.AgentMetric{})
		return
	}
	metrics, err := h.statsStore.QueryLatestAgentMetrics(r.Context())
	if err != nil {
		httpInternalError(w, "query latest agent metrics", err)
		return
	}
	if metrics == nil {
		metrics = []stats.AgentMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

// parseAgentFilter builds an AgentFilter from the parsed stats query.
func parseAgentFilter(sq statsQuery) stats.AgentFilter {
	f := stats.AgentFilter{
		Agent: sq.Filters["agent"],
	}
	if roles := sq.Filters["role"]; len(roles) > 0 {
		f.Role = roles[0]
	}
	if tools := sq.Filters["tool"]; len(tools) > 0 {
		f.Tool = tools[0]
	}
	if runtimes := sq.Filters["runtime"]; len(runtimes) > 0 {
		f.Runtime = runtimes[0]
	}
	return f
}

func (h *StatsHandler) agentCPU(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "agent", "role", "tool", "runtime")
	f := parseAgentFilter(sq)

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.AgentMetric{})
		return
	}

	metrics, err := h.statsStore.QueryAgentCPU(r.Context(), f, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query agent cpu", err)
		return
	}
	if metrics == nil {
		metrics = []stats.AgentMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (h *StatsHandler) agentMem(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "agent", "role", "tool", "runtime")
	f := parseAgentFilter(sq)

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.AgentMetric{})
		return
	}

	metrics, err := h.statsStore.QueryAgentMem(r.Context(), f, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query agent mem", err)
		return
	}
	if metrics == nil {
		metrics = []stats.AgentMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (h *StatsHandler) agentDisk(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "agent", "role", "tool", "runtime")
	f := parseAgentFilter(sq)

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.AgentMetric{})
		return
	}

	metrics, err := h.statsStore.QueryAgentDisk(r.Context(), f, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query agent disk", err)
		return
	}
	if metrics == nil {
		metrics = []stats.AgentMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (h *StatsHandler) agentNet(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "agent", "role", "tool", "runtime")
	f := parseAgentFilter(sq)

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.AgentMetric{})
		return
	}

	metrics, err := h.statsStore.QueryAgentNet(r.Context(), f, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query agent net", err)
		return
	}
	if metrics == nil {
		metrics = []stats.AgentMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (h *StatsHandler) agentTokens(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "agent", "model")
	f := stats.AgentFilter{
		Agent: sq.Filters["agent"],
	}

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.TokenMetric{})
		return
	}

	metrics, err := h.statsStore.QueryAgentTokens(r.Context(), f, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query agent tokens", err)
		return
	}
	if metrics == nil {
		metrics = []stats.TokenMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (h *StatsHandler) agentCost(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "agent", "team")
	f := stats.AgentFilter{
		Agent: sq.Filters["agent"],
	}

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.TokenMetric{})
		return
	}

	metrics, err := h.statsStore.QueryAgentCost(r.Context(), f, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query agent cost", err)
		return
	}
	if metrics == nil {
		metrics = []stats.TokenMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}
