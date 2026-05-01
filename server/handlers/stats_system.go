package handlers

import (
	"net/http"

	"github.com/rpuneet/mycel/pkg/stats"
)

// RegisterSystemStats mounts system stats routes on the mux.
func (h *StatsHandler) RegisterSystemStats(mux *http.ServeMux) {
	mux.HandleFunc("/api/system/stats/cpu", h.systemCPU)
	mux.HandleFunc("/api/system/stats/mem", h.systemMem)
	mux.HandleFunc("/api/system/stats/disk", h.systemDisk)
	mux.HandleFunc("/api/system/stats/net", h.systemNet)
}

func (h *StatsHandler) systemCPU(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "system")
	systems := sq.Filters["system"]

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.SystemMetric{})
		return
	}

	metrics, err := h.statsStore.QuerySystemCPU(r.Context(), systems, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query system cpu", err)
		return
	}
	if metrics == nil {
		metrics = []stats.SystemMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (h *StatsHandler) systemMem(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "system")
	systems := sq.Filters["system"]

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.SystemMetric{})
		return
	}

	metrics, err := h.statsStore.QuerySystemMem(r.Context(), systems, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query system mem", err)
		return
	}
	if metrics == nil {
		metrics = []stats.SystemMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (h *StatsHandler) systemDisk(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "system")
	systems := sq.Filters["system"]

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.SystemMetric{})
		return
	}

	metrics, err := h.statsStore.QuerySystemDisk(r.Context(), systems, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query system disk", err)
		return
	}
	if metrics == nil {
		metrics = []stats.SystemMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (h *StatsHandler) systemNet(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "system")
	systems := sq.Filters["system"]

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.SystemMetric{})
		return
	}

	metrics, err := h.statsStore.QuerySystemNet(r.Context(), systems, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query system net", err)
		return
	}
	if metrics == nil {
		metrics = []stats.SystemMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}
