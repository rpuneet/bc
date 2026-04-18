package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/rpuneet/bc/pkg/gateway"
	"github.com/rpuneet/bc/pkg/notify"
	"github.com/rpuneet/bc/pkg/workspace"
)

// GatewayHandler handles /api/gateways routes.
type GatewayHandler struct {
	gw        *gateway.Manager
	ws        *workspace.Workspace
	notifySvc *notify.Service
}

// NewGatewayHandler creates a GatewayHandler.
func NewGatewayHandler(gw *gateway.Manager, ws *workspace.Workspace) *GatewayHandler {
	return &GatewayHandler{gw: gw, ws: ws}
}

// SetNotifyService sets the notification service for subscription management.
func (h *GatewayHandler) SetNotifyService(svc *notify.Service) {
	h.notifySvc = svc
}

// Register mounts gateway routes.
func (h *GatewayHandler) Register(mux *http.ServeMux) {
	// Legacy channel list endpoint — returns gateway channels in old format
	mux.HandleFunc("/api/channels", h.legacyChannelList)
	mux.HandleFunc("/api/channels/", h.legacyChannelHistory)
	mux.HandleFunc("/api/gateways/activity", h.activity)
	mux.HandleFunc("/api/gateways", h.list)

	// Notify subscription endpoints
	mux.HandleFunc("/api/notify/subscriptions", h.notifySubscriptions)
	mux.HandleFunc("/api/notify/subscriptions/", h.notifySubscriptionByChannel)
	mux.HandleFunc("/api/notify/activity/", h.notifyActivity)

	// Gateway-scoped routes (proposal-aligned)
	mux.HandleFunc("/api/gateways/", h.gatewayRouter)
}

// gatewayRouter dispatches /api/gateways/{platform}/... routes.
func (h *GatewayHandler) gatewayRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/gateways/")
	if path == "" {
		httpError(w, "platform required", http.StatusBadRequest)
		return
	}

	// Split: platform / rest...
	parts := strings.SplitN(path, "/", 2)
	platform := parts[0]
	rest := ""
	if len(parts) > 1 {
		rest = parts[1]
	}

	switch {
	case rest == "health":
		h.gatewayHealth(w, r, platform)
	case rest == "channels" || strings.HasPrefix(rest, "channels/"):
		h.gatewayChannels(w, r, platform, strings.TrimPrefix(rest, "channels"))
	default:
		// Existing: PATCH /api/gateways/{platform}
		h.byPlatform(w, r)
	}
}

// gatewayHealth returns live health status for a gateway adapter.
func (h *GatewayHandler) gatewayHealth(w http.ResponseWriter, r *http.Request, platform string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if h.gw == nil {
		httpError(w, "gateway manager not available", http.StatusServiceUnavailable)
		return
	}

	status := h.gw.AdapterStatus(platform)

	writeJSON(w, http.StatusOK, map[string]any{
		"platform":        platform,
		"connected":       status.Connected,
		"status":          map[bool]string{true: "ok", false: "disconnected"}[status.Connected],
		"error":           status.Error,
		"last_message_at": status.LastMessageAt,
	})
}

// gatewayChannels handles /api/gateways/{platform}/channels and sub-routes.
func (h *GatewayHandler) gatewayChannels(w http.ResponseWriter, r *http.Request, platform, subpath string) {
	subpath = strings.TrimPrefix(subpath, "/")

	if subpath == "" {
		// GET /api/gateways/{platform}/channels — list channels for this gateway
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if h.gw == nil {
			writeJSON(w, http.StatusOK, []string{})
			return
		}
		extChannels := h.gw.ExternalChannels()
		prefix := platform + ":"
		var channels []map[string]string
		for _, ch := range extChannels {
			if strings.HasPrefix(ch, prefix) {
				name := strings.TrimPrefix(ch, prefix)
				channels = append(channels, map[string]string{
					"channel_key": ch,
					"name":        name,
					"platform":    platform,
				})
			}
		}
		if channels == nil {
			channels = []map[string]string{}
		}
		writeJSON(w, http.StatusOK, channels)
		return
	}

	// /api/gateways/{platform}/channels/{channel}/...
	channelParts := strings.SplitN(subpath, "/", 2)
	channelName := platform + ":" + channelParts[0]
	channelRest := ""
	if len(channelParts) > 1 {
		channelRest = channelParts[1]
	}

	switch {
	case channelRest == "agents" || strings.HasPrefix(channelRest, "agents"):
		h.gatewayChannelAgents(w, r, channelName, strings.TrimPrefix(channelRest, "agents"))
	case channelRest == "activity":
		// Delegate to existing activity handler
		r.URL.Path = "/api/notify/activity/" + channelName
		h.notifyActivity(w, r)
	case channelRest == "send":
		h.gatewayChannelSend(w, r, channelName)
	default:
		// GET /api/gateways/{platform}/channels/{channel} — channel detail
		if h.notifySvc == nil {
			httpError(w, "notify service not available", http.StatusServiceUnavailable)
			return
		}
		subs, err := h.notifySvc.ChannelSubscriptions(r.Context(), channelName)
		if err != nil {
			httpInternalError(w, "channel subscriptions", err)
			return
		}
		if subs == nil {
			subs = []notify.Subscription{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"channel_key":   channelName,
			"name":          channelParts[0],
			"platform":      platform,
			"subscriptions": subs,
		})
	}
}

// gatewayChannelSend handles POST /api/gateways/{platform}/channels/{channel}/send
func (h *GatewayHandler) gatewayChannelSend(w http.ResponseWriter, r *http.Request, channel string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if h.gw == nil {
		httpError(w, "gateway manager not available", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpError(w, "read body", http.StatusBadRequest)
		return
	}

	var req struct {
		Sender  string `json:"sender"`
		Content string `json:"content"`
	}
	if unmarshalErr := json.Unmarshal(body, &req); unmarshalErr != nil {
		httpError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		httpError(w, "content is required", http.StatusBadRequest)
		return
	}
	if req.Sender == "" {
		req.Sender = "api"
	}

	sent, err := h.gw.Send(r.Context(), channel, req.Sender, req.Content)
	if err != nil {
		httpInternalError(w, "send message", err)
		return
	}
	if !sent {
		httpError(w, "channel not found or not a gateway channel", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"channel": channel,
		"sender":  req.Sender,
	})
}

// gatewayChannelAgents handles /api/gateways/{platform}/channels/{channel}/agents
func (h *GatewayHandler) gatewayChannelAgents(w http.ResponseWriter, r *http.Request, channel, subpath string) {
	if h.notifySvc == nil {
		httpError(w, "notify service not available", http.StatusServiceUnavailable)
		return
	}

	subpath = strings.TrimPrefix(subpath, "/")

	switch r.Method {
	case http.MethodGet:
		subs, err := h.notifySvc.ChannelSubscriptions(r.Context(), channel)
		if err != nil {
			httpInternalError(w, "list agents", err)
			return
		}
		if subs == nil {
			subs = []notify.Subscription{}
		}
		writeJSON(w, http.StatusOK, subs)

	case http.MethodPost:
		var req struct {
			Agent       string `json:"agent"`
			MentionOnly bool   `json:"mention_only"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid body", http.StatusBadRequest)
			return
		}
		if req.Agent == "" {
			httpError(w, "agent required", http.StatusBadRequest)
			return
		}
		if err := h.notifySvc.Subscribe(r.Context(), channel, req.Agent, req.MentionOnly); err != nil {
			httpInternalError(w, "subscribe", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"status": "subscribed", "channel": channel, "agent": req.Agent})

	case http.MethodDelete:
		// DELETE /api/gateways/{gw}/channels/{ch}/agents/{agent}
		if subpath == "" {
			agent := r.URL.Query().Get("agent")
			if agent == "" {
				httpError(w, "agent required (path or query param)", http.StatusBadRequest)
				return
			}
			subpath = agent
		}
		if err := h.notifySvc.Unsubscribe(r.Context(), channel, subpath); err != nil {
			httpInternalError(w, "unsubscribe", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed", "channel": channel, "agent": subpath})

	case http.MethodPatch:
		// PATCH /api/gateways/{gw}/channels/{ch}/agents/{agent}
		var req struct {
			MentionOnly *bool `json:"mention_only"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid body", http.StatusBadRequest)
			return
		}
		agent := subpath
		if agent == "" {
			httpError(w, "agent required in path", http.StatusBadRequest)
			return
		}
		if req.MentionOnly != nil {
			if err := h.notifySvc.SetMentionOnly(r.Context(), channel, agent, *req.MentionOnly); err != nil {
				httpInternalError(w, "set mention_only", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "channel": channel, "agent": agent})

	default:
		methodNotAllowed(w)
	}
}

// gatewayStatus represents a gateway platform's config and runtime state.

type gatewayStatus struct { //nolint:govet // field order matches JSON/API contract
	Config   any      `json:"config,omitempty"`
	Platform string   `json:"platform"`
	BotName  string   `json:"bot_name,omitempty"`
	Channels []string `json:"channels"`
	Enabled  bool     `json:"enabled"`
}

func (h *GatewayHandler) list(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	platforms := []gatewayStatus{}

	if h.ws != nil && h.ws.Config != nil {
		gw := h.ws.Config.Gateways

		// Multi-Telegram: iterate the Telegrams map (includes the legacy
		// single "telegram" key as label="").
		for label, tc := range gw.Telegrams {
			name := "telegram"
			if label != "" {
				name = "telegram:" + label
			}
			platforms = append(platforms, gatewayStatus{
				Platform: name,
				Enabled:  tc.Enabled,
				Config: map[string]any{
					"mode":      tc.Mode,
					"has_token": tc.BotToken != "",
				},
			})
		}
		// Fallback for legacy single-telegram config
		if len(gw.Telegrams) == 0 && gw.Telegram != nil {
			platforms = append(platforms, gatewayStatus{
				Platform: "telegram",
				Enabled:  gw.Telegram.Enabled,
				Config: map[string]any{
					"mode":      gw.Telegram.Mode,
					"has_token": gw.Telegram.BotToken != "",
				},
			})
		}
		if gw.Discord != nil {
			platforms = append(platforms, gatewayStatus{
				Platform: "discord",
				Enabled:  gw.Discord.Enabled,
				Config: map[string]any{
					"has_token": gw.Discord.BotToken != "",
				},
			})
		}
		if gw.Slack != nil {
			platforms = append(platforms, gatewayStatus{
				Platform: "slack",
				Enabled:  gw.Slack.Enabled,
				Config: map[string]any{
					"mode":          gw.Slack.Mode,
					"has_bot_token": gw.Slack.BotToken != "",
					"has_app_token": gw.Slack.AppToken != "",
				},
			})
		}
	}

	// Enrich with discovered channels and bot name from adapter status
	if h.gw != nil {
		extChannels := h.gw.ExternalChannels()
		for i := range platforms {
			prefix := platforms[i].Platform + ":"
			for _, ch := range extChannels {
				if strings.HasPrefix(ch, prefix) {
					platforms[i].Channels = append(platforms[i].Channels, ch)
				}
			}
			status := h.gw.AdapterStatus(platforms[i].Platform)
			if status.BotName != "" {
				platforms[i].BotName = status.BotName
			}
		}
	}

	// Ensure channels is never null in JSON output
	for i := range platforms {
		if platforms[i].Channels == nil {
			platforms[i].Channels = []string{}
		}
	}

	writeJSON(w, http.StatusOK, platforms)
}

func (h *GatewayHandler) byPlatform(w http.ResponseWriter, r *http.Request) {
	platform := strings.TrimPrefix(r.URL.Path, "/api/gateways/")
	if platform == "" {
		httpError(w, "platform name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		h.updatePlatform(w, r, platform)
	default:
		methodNotAllowed(w)
	}
}

func (h *GatewayHandler) updatePlatform(w http.ResponseWriter, r *http.Request, platform string) {
	if h.ws == nil || h.ws.Config == nil {
		httpError(w, "workspace not available", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpError(w, "failed to read body", http.StatusBadRequest)
		return
	}

	cfg := h.ws.Config
	switch platform {
	case "telegram":
		if cfg.Gateways.Telegram == nil {
			cfg.Gateways.Telegram = &workspace.TelegramGatewayConfig{}
		}
		if err := json.Unmarshal(body, cfg.Gateways.Telegram); err != nil {
			httpError(w, "invalid telegram config: "+err.Error(), http.StatusBadRequest)
			return
		}
	case "discord":
		if cfg.Gateways.Discord == nil {
			cfg.Gateways.Discord = &workspace.DiscordGatewayConfig{}
		}
		if err := json.Unmarshal(body, cfg.Gateways.Discord); err != nil {
			httpError(w, "invalid discord config: "+err.Error(), http.StatusBadRequest)
			return
		}
	case "slack":
		if cfg.Gateways.Slack == nil {
			cfg.Gateways.Slack = &workspace.SlackGatewayConfig{}
		}
		if err := json.Unmarshal(body, cfg.Gateways.Slack); err != nil {
			httpError(w, "invalid slack config: "+err.Error(), http.StatusBadRequest)
			return
		}
	default:
		httpError(w, "unknown platform: "+platform, http.StatusBadRequest)
		return
	}

	if err := cfg.Save(workspace.ConfigPath(h.ws.RootDir)); err != nil {
		httpInternalError(w, "save config", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "platform": platform})
}

// legacyChannelList returns gateway channels in the old Channel format
// so the frontend's listChannels() call still works after pkg/channel deletion.
func (h *GatewayHandler) legacyChannelList(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	type legacyChannel struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Members     []string `json:"members"`
		MemberCount int      `json:"member_count"`
	}

	seen := make(map[string]bool)
	var channels []legacyChannel

	// From gateway manager (discovered channels)
	if h.gw != nil {
		for _, ch := range h.gw.ExternalChannels() {
			seen[ch] = true
			channels = append(channels, legacyChannel{
				Name:        ch,
				Description: "Gateway channel",
			})
		}
	}

	// Also include channels that have notify subscriptions
	if h.notifySvc != nil {
		subs, err := h.notifySvc.AllSubscriptions(r.Context())
		if err == nil {
			for _, sub := range subs {
				if !seen[sub.Channel] {
					seen[sub.Channel] = true
					channels = append(channels, legacyChannel{
						Name:        sub.Channel,
						Description: "Gateway channel",
					})
				}
			}
		}
	}

	if channels == nil {
		channels = []legacyChannel{}
	}
	writeJSON(w, http.StatusOK, channels)
}

// legacyChannelHistory returns message history from notify_messages.
func (h *GatewayHandler) legacyChannelHistory(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	// Extract channel name: /api/channels/{name}/history
	path := strings.TrimPrefix(r.URL.Path, "/api/channels/")
	path = strings.TrimSuffix(path, "/history")
	path = strings.TrimSuffix(path, "/messages")
	channelName := path

	if channelName == "" || h.notifySvc == nil {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}

	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	var before int64
	if s := r.URL.Query().Get("before"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			before = n
		}
	}

	msgs, err := h.notifySvc.ChannelMessages(r.Context(), channelName, limit, before)
	if err != nil {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}

	// Convert to legacy format
	type legacyMessage struct {
		Sender    string `json:"sender"`
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
		ID        int64  `json:"id"`
	}
	result := make([]legacyMessage, len(msgs))
	for i, m := range msgs {
		result[i] = legacyMessage{
			ID:        m.ID,
			Sender:    m.Sender,
			Content:   m.Content,
			CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// activity returns recent activity from notify delivery log.
// GET /api/gateways/activity?limit=50
func (h *GatewayHandler) activity(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	if h.notifySvc == nil {
		writeJSON(w, http.StatusOK, []notify.DeliveryEntry{})
		return
	}

	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	limit = clampInt(limit, 1, 200)

	// Aggregate activity across all gateway channels
	var gwChannelNames []string
	if h.gw != nil {
		gwChannelNames = h.gw.ExternalChannels()
	}
	if len(gwChannelNames) == 0 {
		writeJSON(w, http.StatusOK, []notify.DeliveryEntry{})
		return
	}

	var allEntries []notify.DeliveryEntry
	for _, ch := range gwChannelNames {
		entries, err := h.notifySvc.ChannelActivity(r.Context(), ch, limit)
		if err != nil {
			continue
		}
		allEntries = append(allEntries, entries...)
	}

	if len(allEntries) > limit {
		allEntries = allEntries[:limit]
	}

	writeJSON(w, http.StatusOK, allEntries)
}

// --- Notify-powered subscription endpoints ---

// notifySubscriptions handles GET/POST /api/notify/subscriptions
func (h *GatewayHandler) notifySubscriptions(w http.ResponseWriter, r *http.Request) {
	if h.notifySvc == nil {
		httpError(w, "notify service not available", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// List all subscriptions
		subs, err := h.notifySvc.AllSubscriptions(r.Context())
		if err != nil {
			httpInternalError(w, "list subscriptions", err)
			return
		}
		if subs == nil {
			subs = []notify.Subscription{}
		}
		writeJSON(w, http.StatusOK, subs)

	case http.MethodPost:
		// Subscribe: {"channel": "slack:eng", "agent": "eng-01", "mention_only": false}
		var req struct {
			Channel     string `json:"channel"`
			Agent       string `json:"agent"`
			MentionOnly bool   `json:"mention_only"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Channel == "" || req.Agent == "" {
			httpError(w, "channel and agent are required", http.StatusBadRequest)
			return
		}
		if err := h.notifySvc.Subscribe(r.Context(), req.Channel, req.Agent, req.MentionOnly); err != nil {
			httpInternalError(w, "subscribe", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"status": "subscribed", "channel": req.Channel, "agent": req.Agent})

	default:
		methodNotAllowed(w)
	}
}

// notifySubscriptionByChannel handles operations on /api/notify/subscriptions/{channel}
// GET    — list subscribers for a channel
// DELETE — unsubscribe: ?agent=eng-01
// PATCH  — update: {"agent": "eng-01", "mention_only": true}
func (h *GatewayHandler) notifySubscriptionByChannel(w http.ResponseWriter, r *http.Request) {
	if h.notifySvc == nil {
		httpError(w, "notify service not available", http.StatusServiceUnavailable)
		return
	}

	// Extract channel from path: /api/notify/subscriptions/slack:eng
	channel := strings.TrimPrefix(r.URL.Path, "/api/notify/subscriptions/")
	if channel == "" {
		httpError(w, "channel name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		subs, err := h.notifySvc.ChannelSubscriptions(r.Context(), channel)
		if err != nil {
			httpInternalError(w, "list channel subscriptions", err)
			return
		}
		if subs == nil {
			subs = []notify.Subscription{}
		}
		writeJSON(w, http.StatusOK, subs)

	case http.MethodDelete:
		agent := r.URL.Query().Get("agent")
		if agent == "" {
			httpError(w, "agent query param required", http.StatusBadRequest)
			return
		}
		if err := h.notifySvc.Unsubscribe(r.Context(), channel, agent); err != nil {
			httpInternalError(w, "unsubscribe", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed", "channel": channel, "agent": agent})

	case http.MethodPatch:
		var req struct {
			MentionOnly *bool  `json:"mention_only"`
			Agent       string `json:"agent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Agent == "" {
			httpError(w, "agent is required", http.StatusBadRequest)
			return
		}
		if req.MentionOnly != nil {
			if err := h.notifySvc.SetMentionOnly(r.Context(), channel, req.Agent, *req.MentionOnly); err != nil {
				httpInternalError(w, "set mention_only", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "channel": channel, "agent": req.Agent})

	default:
		methodNotAllowed(w)
	}
}

// notifyActivity handles GET /api/notify/activity/{channel}
func (h *GatewayHandler) notifyActivity(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if h.notifySvc == nil {
		httpError(w, "notify service not available", http.StatusServiceUnavailable)
		return
	}

	channel := strings.TrimPrefix(r.URL.Path, "/api/notify/activity/")
	if channel == "" {
		httpError(w, "channel name required", http.StatusBadRequest)
		return
	}

	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	limit = clampInt(limit, 1, 200)

	entries, err := h.notifySvc.ChannelActivity(r.Context(), channel, limit)
	if err != nil {
		httpInternalError(w, "channel activity", err)
		return
	}
	if entries == nil {
		entries = []notify.DeliveryEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}
