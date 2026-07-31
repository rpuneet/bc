package handlers

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"
)

// overviewApp summarizes one gateway adapter for the notifications home page.
type overviewApp struct {
	LastActivity     time.Time `json:"last_activity"`
	Name             string    `json:"name"`
	Platform         string    `json:"platform"`
	DisconnectReason string    `json:"disconnect_reason"`
	ChannelCount     int       `json:"channel_count"`
	Connected        bool      `json:"connected"`
}

// overviewChannel is one gateway channel with resolved identity and activity.
type overviewChannel struct {
	LastActivity     time.Time `json:"last_activity"`
	Channel        string    `json:"channel"`
	Platform         string    `json:"platform"`
	DisplayName      string    `json:"display_name"`
	Kind             string    `json:"kind"`
	ParticipantCount int       `json:"participant_count"`
	SubscriberCount  int       `json:"subscriber_count"`
	MessageCount     int       `json:"message_count"`
}

// notificationsOverview handles GET /api/notifications/overview — the data
// source for the notifications home page: connected apps (gateway adapter
// status) and all known channels with resolved display metadata, message
// counts, subscriber counts, and last activity.
func (h *GatewayHandler) notificationsOverview(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if h.notifySvc == nil {
		serviceUnavailable(w, r, "notify", "notify service not available")
		return
	}

	// Persisted channel mappings carry the resolved identity metadata.
	persisted, err := h.notifySvc.Channels(r.Context())
	if err != nil {
		httpInternalError(w, "load channels", err)
		return
	}
	byName := make(map[string]*overviewChannel, len(persisted))
	for _, c := range persisted {
		byName[c.Channel] = &overviewChannel{
			Channel:        c.Channel,
			Platform:         c.Platform,
			DisplayName:      c.DisplayName,
			Kind:             c.Kind,
			ParticipantCount: c.ParticipantCount,
		}
	}

	// In-memory discovered channels not yet persisted.
	if h.gw != nil {
		for _, name := range h.gw.DiscoveredSources() {
			if _, ok := byName[name]; ok {
				continue
			}
			platform := name
			if i := strings.Index(name, ":"); i > 0 {
				platform = name[:i]
			}
			byName[name] = &overviewChannel{Channel: name, Platform: platform}
		}
	}

	// Enrich with message counts, subscriber counts, and last activity.
	stats, err := h.notifySvc.ChannelStats(r.Context())
	if err != nil {
		httpInternalError(w, "channel stats", err)
		return
	}
	for _, st := range stats {
		ch, ok := byName[st.Name]
		if !ok {
			continue // not a gateway channel
		}
		ch.MessageCount = st.MessageCount
		ch.SubscriberCount = st.MemberCount
		ch.LastActivity = st.LastActivity
	}

	channels := make([]overviewChannel, 0, len(byName))
	for _, ch := range byName {
		if ch.DisplayName == "" {
			// Never show a blank name: fall back to the mycel channel suffix.
			ch.DisplayName = strings.TrimPrefix(ch.Channel, ch.Platform+":")
		}
		channels = append(channels, *ch)
	}
	sort.Slice(channels, func(i, j int) bool {
		if channels[i].MessageCount != channels[j].MessageCount {
			return channels[i].MessageCount > channels[j].MessageCount
		}
		return channels[i].Channel < channels[j].Channel
	})

	apps := []overviewApp{}
	if h.gw != nil {
		channelCount := make(map[string]int)
		lastActivity := make(map[string]time.Time)
		for _, ch := range channels {
			channelCount[ch.Platform]++
			if ch.LastActivity.After(lastActivity[ch.Platform]) {
				lastActivity[ch.Platform] = ch.LastActivity
			}
		}
		for _, name := range h.gw.AdapterNames() {
			status := h.gw.AdapterStatus(name)
			last := status.LastMessageAt
			if lastActivity[name].After(last) {
				last = lastActivity[name]
			}
			apps = append(apps, overviewApp{
				Name:             name,
				Platform:         name,
				Connected:        status.Connected,
				DisconnectReason: status.Error,
				ChannelCount:     channelCount[name],
				LastActivity:     last,
			})
		}
		sort.Slice(apps, func(i, j int) bool { return apps[i].Name < apps[j].Name })
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"apps":     apps,
		"channels": channels,
	})
}

// refreshChannelMeta handles POST /api/apps/channels/refresh — re-resolves
// display metadata (names, kinds, participant counts) for all known gateway
// channels via adapters that support identity resolution.
func (h *GatewayHandler) refreshChannelMeta(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if h.gw == nil {
		serviceUnavailable(w, r, "gateway", "gateway manager not available")
		return
	}
	// Bound the whole refresh — a slow adapter must not pin the request
	// past a sane ceiling even with per-channel timeouts underneath.
	refreshCtx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	n, err := h.gw.RefreshChannelMeta(refreshCtx)
	if err != nil {
		httpInternalError(w, "refresh channel meta", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"refreshed": n})
}
