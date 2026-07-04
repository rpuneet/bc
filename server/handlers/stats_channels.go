package handlers

import (
	"net/http"

	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/stats"
)

// statsChannels serves GET /api/stats/channels — per-channel notification
// activity (message counts, member counts, last activity, top senders)
// aggregated from the notify store. Powers the Metrics page notification
// panel in the web UI.
func (h *StatsHandler) statsChannels(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if h.notifySvc == nil {
		writeJSON(w, http.StatusOK, []notify.ChannelStat{})
		return
	}
	channelStats, err := h.notifySvc.ChannelStats(r.Context())
	if err != nil {
		httpInternalError(w, "channel stats", err)
		return
	}
	if channelStats == nil {
		channelStats = []notify.ChannelStat{}
	}
	writeJSON(w, http.StatusOK, channelStats)
}

// RegisterChannelStats mounts channel stats routes on the mux.
func (h *StatsHandler) RegisterChannelStats(mux *http.ServeMux) {
	mux.HandleFunc("/api/channels/stats/messages", h.channelMessages)
	mux.HandleFunc("/api/channels/stats/members", h.channelMembers)
	mux.HandleFunc("/api/channels/stats/reactions", h.channelReactions)
}

func (h *StatsHandler) channelMessages(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "channel")
	f := stats.ChannelFilter{
		Channel: sq.Filters["channel"],
	}

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.ChannelMetric{})
		return
	}

	metrics, err := h.statsStore.QueryChannelMessages(r.Context(), f, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query channel messages", err)
		return
	}
	if metrics == nil {
		metrics = []stats.ChannelMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (h *StatsHandler) channelMembers(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "channel")
	f := stats.ChannelFilter{
		Channel: sq.Filters["channel"],
	}

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.ChannelMetric{})
		return
	}

	metrics, err := h.statsStore.QueryChannelMembers(r.Context(), f, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query channel members", err)
		return
	}
	if metrics == nil {
		metrics = []stats.ChannelMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (h *StatsHandler) channelReactions(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	sq := parseStatsQuery(r, "channel")
	f := stats.ChannelFilter{
		Channel: sq.Filters["channel"],
	}

	if h.statsStore == nil {
		writeJSON(w, http.StatusOK, []stats.ChannelMetric{})
		return
	}

	metrics, err := h.statsStore.QueryChannelReactions(r.Context(), f, sq.TimeRange)
	if err != nil {
		httpInternalError(w, "query channel reactions", err)
		return
	}
	if metrics == nil {
		metrics = []stats.ChannelMetric{}
	}
	writeJSON(w, http.StatusOK, metrics)
}
