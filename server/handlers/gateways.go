package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/notify"
)

// GatewayHandler handles the channel, subscription, and activity
// surface of the apps platform: the global channel list/history under
// /api/apps/channels, the notify subscription endpoints, and the
// per-instance routes (health, channels, adapter proxy, reactions)
// that AppsHandler delegates here for /api/apps/{name}/...
type GatewayHandler struct {
	gw        *gateway.Manager
	h         *home.Home
	notifySvc *notify.Service
}

// NewGatewayHandler creates a GatewayHandler.
func NewGatewayHandler(gw *gateway.Manager, h *home.Home) *GatewayHandler {
	return &GatewayHandler{gw: gw, h: h}
}

// SetNotifyService sets the notification service for subscription management.
func (h *GatewayHandler) SetNotifyService(svc *notify.Service) {
	h.notifySvc = svc
}

// Register mounts gateway routes.
func (h *GatewayHandler) Register(mux *http.ServeMux) {
	// Channel surface under the apps namespace — the web UI's primary
	// paths. Longest-pattern matching keeps these ahead of the generic
	// /api/apps/{name} instance router.
	mux.HandleFunc("/api/apps/channels", h.channelList)
	mux.HandleFunc("/api/apps/channels/send", h.channelSend)
	mux.HandleFunc("/api/apps/channels/", h.channelHistory)

	// Same handlers at the historical paths — the Go CLI client still calls /api/channels*.
	mux.HandleFunc("/api/channels", h.channelList)
	mux.HandleFunc("/api/channels/send", h.channelSend)
	mux.HandleFunc("/api/channels/", h.channelHistory)

	// Notify subscription endpoints
	mux.HandleFunc("/api/notify/subscriptions", h.notifySubscriptions)
	mux.HandleFunc("/api/notify/subscriptions/", h.notifySubscriptionByChannel)
	mux.HandleFunc("/api/notify/activity/", h.notifyActivity)

	// Notifications home: connected apps + channels with resolved identities.
	mux.HandleFunc("/api/notifications/overview", h.notificationsOverview)
	// Manual re-resolution of channel display metadata (names, kinds).
	mux.HandleFunc("/api/apps/channels/refresh", h.refreshChannelMeta)
	// Loopback-guarded image proxy for platform avatars (never leaks tokens).
	mux.HandleFunc("/api/apps/avatar", h.avatarProxy)
}

// appScopedRoute serves the per-instance routes AppsHandler delegates
// here for /api/apps/{name}/...: health, channel listing/subscriptions,
// the adapter API proxy, and reactions.
func (h *GatewayHandler) appScopedRoute(w http.ResponseWriter, r *http.Request, platform, rest string) {
	switch {
	case rest == "health":
		h.gatewayHealth(w, r, platform)
	case rest == "channels" || strings.HasPrefix(rest, "channels/"):
		h.gatewayChannels(w, r, platform, strings.TrimPrefix(rest, "channels"))
	case rest == "api" || strings.HasPrefix(rest, "api/"):
		h.gatewayAPIProxy(w, r, platform, strings.TrimPrefix(rest, "api"))
	case rest == "react":
		h.gatewayReact(w, r, platform)
	case rest == "status":
		h.gatewayCommitStatus(w, r, platform)
	default:
		httpError(w, "not found", http.StatusNotFound)
	}
}

// gatewayAPIProxy forwards requests to /api/apps/{name}/api/* to the adapter's HTTP handler.
func (h *GatewayHandler) gatewayAPIProxy(w http.ResponseWriter, r *http.Request, platform, subpath string) {
	if h.gw == nil {
		serviceUnavailable(w, r, "gateway", "gateway manager not available")
		return
	}
	adapter := h.gw.GetAdapter(platform)
	if adapter == nil {
		httpError(w, "adapter not found: "+platform, http.StatusNotFound)
		return
	}
	handler := adapter.HTTPHandler()
	if handler == nil {
		httpError(w, "adapter does not support API proxy: "+platform, http.StatusNotImplemented)
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = subpath
	handler.ServeHTTP(w, r2)
}

// gatewayHealth returns live health status for a gateway adapter.
// The payload mirrors gateway.AdapterStatus (bot_name, message_count,
// last_message_at, error) plus a derived status string. Unregistered
// adapters return 404 rather than 200 with an error string (#3693).
func (h *GatewayHandler) gatewayHealth(w http.ResponseWriter, r *http.Request, platform string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if h.gw == nil {
		serviceUnavailable(w, r, "gateway", "gateway manager not available")
		return
	}
	if h.gw.GetAdapter(platform) == nil {
		httpError(w, "adapter not found: "+platform, http.StatusNotFound)
		return
	}

	status := h.gw.AdapterStatus(platform)
	body := map[string]any{
		"platform":      platform,
		"connected":     status.Connected,
		"status":        map[bool]string{true: "ok", false: "disconnected"}[status.Connected],
		"message_count": status.MessageCount,
	}
	if status.BotName != "" {
		body["bot_name"] = status.BotName
	}
	if status.Error != "" {
		body["error"] = status.Error
	}
	if !status.LastMessageAt.IsZero() {
		body["last_message_at"] = status.LastMessageAt
	}
	writeJSON(w, http.StatusOK, body)
}

// gatewayChannels handles /api/apps/{name}/channels and sub-routes.
func (h *GatewayHandler) gatewayChannels(w http.ResponseWriter, r *http.Request, platform, subpath string) {
	subpath = strings.TrimPrefix(subpath, "/")

	if subpath == "" {
		// GET /api/apps/{name}/channels — list channels for this instance
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if h.gw == nil {
			writeJSON(w, http.StatusOK, []string{})
			return
		}
		extChannels := h.gw.DiscoveredSources()
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

	// /api/apps/{name}/channels/{channel}/...
	channelParts := strings.SplitN(subpath, "/", 2)
	// Defensive against callers who pre-prefix the channel with the
	// platform (e.g. `POST /api/apps/slack/channels/slack:general/agents`).
	// Without this guard the channel key gets written as
	// `slack:slack:general` — silently indexed on the wrong key so
	// inbound messages on `slack:general` bypass the subscription lookup
	// and never reach the agent. That's how the notify subscriptions
	// "vanished" after they were added earlier this session.
	rawChannel := channelParts[0]
	rawChannel = strings.TrimPrefix(rawChannel, platform+":")
	channelName := platform + ":" + rawChannel
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
	case channelRest != "":
		// Unknown sub-route (e.g. the removed "send" endpoint).
		httpError(w, "not found", http.StatusNotFound)
	default:
		// GET /api/apps/{name}/channels/{channel} — channel detail
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if h.notifySvc == nil {
			serviceUnavailable(w, r, "notify", "notify service not available")
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
		var last *notify.DeliveryEntry
		if activity, actErr := h.notifySvc.ChannelActivity(r.Context(), channelName, 1, 0); actErr != nil {
			log.Warn("channel detail: last delivery", "channel", channelName, "error", actErr)
		} else if len(activity) > 0 {
			last = &activity[0]
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"channel_key":   channelName,
			"name":          channelParts[0],
			"platform":      platform,
			"subscriptions": subs,
			"last_delivery": last,
		})
	}
}

// gatewayChannelAgents handles /api/apps/{name}/channels/{channel}/agents
func (h *GatewayHandler) gatewayChannelAgents(w http.ResponseWriter, r *http.Request, channel, subpath string) {
	if h.notifySvc == nil {
		serviceUnavailable(w, r, "notify", "notify service not available")
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
		// DELETE /api/apps/{name}/channels/{ch}/agents/{agent}
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
		// PATCH /api/apps/{name}/channels/{ch}/agents/{agent}
		var req struct {
			MentionOnly      *bool `json:"mention_only"`
			DeliverAutomated *bool `json:"deliver_automated"`
			Muted            *bool `json:"muted"`
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
		if req.DeliverAutomated != nil {
			if err := h.notifySvc.SetDeliverAutomated(r.Context(), channel, agent, *req.DeliverAutomated); err != nil {
				httpInternalError(w, "set deliver_automated", err)
				return
			}
		}
		if req.Muted != nil {
			if err := h.notifySvc.SetMuted(r.Context(), channel, agent, *req.Muted); err != nil {
				httpInternalError(w, "set muted", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "channel": channel, "agent": agent})

	default:
		methodNotAllowed(w)
	}
}

// channelSend handles POST /api/apps/channels/send — routes a text message
// from a named sender through the gateway to an external platform channel.
// Body: {"channel": "...", "message": "...", "sender": "..."} (sender defaults to "api").
// Returns 200 {"sent":true} when the gateway delivered the message, or
// 200 {"sent":false} when no route is configured (not an error).
func (h *GatewayHandler) channelSend(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if h.gw == nil {
		httpError(w, "gateway not available", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Channel string `json:"channel"`
		Message string `json:"message"`
		Sender  string `json:"sender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Channel == "" {
		httpError(w, "channel is required", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		httpError(w, "message is required", http.StatusBadRequest)
		return
	}
	if req.Sender == "" {
		req.Sender = "api"
	}

	sent, err := h.gw.Send(r.Context(), req.Channel, req.Sender, req.Message)
	if err != nil {
		log.Warn("channel send failed", "channel", req.Channel, "sender", req.Sender, "error", err)
		httpError(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sent": sent})
}

// channelList handles GET /api/apps/channels — every known mycel channel
// (discovered gateway channels plus channels with notify subscriptions).
func (h *GatewayHandler) channelList(w http.ResponseWriter, r *http.Request) {
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
		for _, ch := range h.gw.DiscoveredSources() {
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

// channelHistory returns message history from notify_messages.
// GET /api/apps/channels/{name}/history (and the /api/channels alias).
func (h *GatewayHandler) channelHistory(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	// Extract channel name: .../channels/{name}/history
	path := strings.TrimPrefix(r.URL.Path, "/api/apps/channels/")
	path = strings.TrimPrefix(path, "/api/channels/")
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
	limit = clampInt(limit, 1, 200)
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

	// Convert to legacy format. AvatarURL is wrapped in the loopback image
	// proxy so the browser loads it without seeing the raw platform CDN URL.
	type legacyMessage struct {
		Sender    string `json:"sender"`
		AvatarURL string `json:"avatar_url,omitempty"`
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
		ID        int64  `json:"id"`
	}
	result := make([]legacyMessage, len(msgs))
	for i, m := range msgs {
		result[i] = legacyMessage{
			ID:        m.ID,
			Sender:    m.Sender,
			AvatarURL: avatarProxyPath(m.AvatarURL),
			Content:   m.Content,
			CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// --- Notify-powered subscription endpoints ---

// notifySubscriptions handles GET/POST /api/notify/subscriptions
func (h *GatewayHandler) notifySubscriptions(w http.ResponseWriter, r *http.Request) {
	if h.notifySvc == nil {
		serviceUnavailable(w, r, "notify", "notify service not available")
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
		serviceUnavailable(w, r, "notify", "notify service not available")
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
			MentionOnly      *bool  `json:"mention_only"`
			DeliverAutomated *bool  `json:"deliver_automated"`
			Muted            *bool  `json:"muted"`
			Agent            string `json:"agent"`
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
		if req.DeliverAutomated != nil {
			if err := h.notifySvc.SetDeliverAutomated(r.Context(), channel, req.Agent, *req.DeliverAutomated); err != nil {
				httpInternalError(w, "set deliver_automated", err)
				return
			}
		}
		if req.Muted != nil {
			if err := h.notifySvc.SetMuted(r.Context(), channel, req.Agent, *req.Muted); err != nil {
				httpInternalError(w, "set muted", err)
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
		serviceUnavailable(w, r, "notify", "notify service not available")
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
	var before int64
	if s := r.URL.Query().Get("before"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			before = n
		}
	}

	entries, err := h.notifySvc.ChannelActivity(r.Context(), channel, limit, before)
	if err != nil {
		httpInternalError(w, "channel activity", err)
		return
	}
	if entries == nil {
		entries = []notify.DeliveryEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// gatewayReact handles POST /api/apps/{name}/react — sends an emoji
// reaction to a specific message via the gateway adapter.
//
// Request body:
//
//	{
//	  "channel":    "whatsapp:family",  // mycel channel key
//	  "message_id": "<platform_msg_id>",
//	  "sender_jid": "<platform_sender>", // required by WhatsApp; omit for other platforms
//	  "emoji":      "👍"                 // empty string removes the reaction
//	}
func (h *GatewayHandler) gatewayReact(w http.ResponseWriter, r *http.Request, platform string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if h.gw == nil {
		serviceUnavailable(w, r, "gateway", "gateway manager not available")
		return
	}

	var req struct {
		Channel   string `json:"channel"`
		MessageID string `json:"message_id"`
		SenderJID string `json:"sender_jid"`
		Emoji     string `json:"emoji"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Channel == "" {
		httpError(w, "channel is required", http.StatusBadRequest)
		return
	}
	// Validate that the channel belongs to the platform in the URL path.
	if !strings.HasPrefix(req.Channel, platform+":") {
		httpError(w, "channel does not belong to platform "+platform, http.StatusBadRequest)
		return
	}
	if req.MessageID == "" {
		httpError(w, "message_id is required", http.StatusBadRequest)
		return
	}
	// Note: empty emoji is intentional — it removes an existing reaction
	// (whatsmeow BuildReaction("", ...) sends a removal).

	sent, err := h.gw.SendReaction(r.Context(), req.Channel, req.SenderJID, req.MessageID, req.Emoji)
	if err != nil {
		httpInternalError(w, "send reaction", err)
		return
	}
	if !sent {
		httpError(w, "channel not found or adapter does not support reactions", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "channel": req.Channel})
}

// commitStatusSetter is checked at runtime for adapters that can set a
// commit status/check (currently only the GitHub adapter). It doesn't fit
// the generic channel-Send model — a status targets a repo+sha, not a
// mycel channel — so it's a focused capability behind its own route
// instead of being routed through gateway.Manager.
type commitStatusSetter interface {
	SetStatus(ctx context.Context, owner, repo, sha, state, description, targetURL, statusContext string) error
}

// gatewayCommitStatus handles POST /api/apps/{name}/status — sets a commit
// status (e.g. GitHub's POST /repos/{owner}/{repo}/statuses/{sha}) via the
// gateway adapter's outbound capability.
//
// Request body:
//
//	{
//	  "owner":       "rpuneet",
//	  "repo":        "mycel",
//	  "sha":         "abc123",
//	  "state":       "success",           // error | failure | pending | success
//	  "description": "all checks passed", // optional
//	  "target_url":  "https://...",       // optional
//	  "context":     "mycel-ci"           // optional
//	}
func (h *GatewayHandler) gatewayCommitStatus(w http.ResponseWriter, r *http.Request, platform string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if h.gw == nil {
		serviceUnavailable(w, r, "gateway", "gateway manager not available")
		return
	}
	adapter := h.gw.GetAdapter(platform)
	if adapter == nil {
		httpError(w, "adapter not found: "+platform, http.StatusNotFound)
		return
	}
	setter, ok := adapter.(commitStatusSetter)
	if !ok {
		httpError(w, "adapter "+platform+" does not support commit statuses", http.StatusNotImplemented)
		return
	}

	var req struct {
		Owner       string `json:"owner"`
		Repo        string `json:"repo"`
		SHA         string `json:"sha"`
		State       string `json:"state"`
		Description string `json:"description"`
		TargetURL   string `json:"target_url"`
		Context     string `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Owner == "" || req.Repo == "" || req.SHA == "" || req.State == "" {
		httpError(w, "owner, repo, sha, and state are required", http.StatusBadRequest)
		return
	}

	if err := setter.SetStatus(r.Context(), req.Owner, req.Repo, req.SHA, req.State, req.Description, req.TargetURL, req.Context); err != nil {
		httpInternalError(w, "set commit status", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
