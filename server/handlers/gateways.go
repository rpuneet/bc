package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/rpuneet/mycel/pkg/gateway"
	bctelegram "github.com/rpuneet/mycel/pkg/gateway/telegram"
	bcwhatsapp "github.com/rpuneet/mycel/pkg/gateway/whatsapp"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// GatewayHandler handles /api/gateways routes.
type GatewayHandler struct {
	gw        *gateway.Manager
	ws        *workspace.Workspace
	notifySvc *notify.Service
	vault     *secret.Store
}

// NewGatewayHandler creates a GatewayHandler.
func NewGatewayHandler(gw *gateway.Manager, ws *workspace.Workspace) *GatewayHandler {
	return &GatewayHandler{gw: gw, ws: ws}
}

// SetNotifyService sets the notification service for subscription management.
func (h *GatewayHandler) SetNotifyService(svc *notify.Service) {
	h.notifySvc = svc
}

// SetSecretStore wires the secrets vault so that gateway credentials are
// mirrored into the vault under canonical env-var names whenever a platform
// is connected or updated. The vault write is additive — preferences.json
// continues to be written for backward compatibility.
func (h *GatewayHandler) SetSecretStore(s *secret.Store) {
	h.vault = s
}

// writeVaultSecret stores key=value in the vault (no-op when no vault is
// wired or when value is empty). Values are intentionally not logged.
func (h *GatewayHandler) writeVaultSecret(key, value string) {
	if h.vault == nil || value == "" {
		return
	}
	if err := h.vault.Set(key, value, "gateway credential"); err != nil {
		log.Warn("gateway: failed to write secret to vault", "key", key, "error", err)
		return
	}
	log.Debug("gateway: wrote credential to vault", "key", key)
}

// Register mounts gateway routes.
func (h *GatewayHandler) Register(mux *http.ServeMux) {
	// Channel list/history endpoints — primary API for the web UI and TUI.
	mux.HandleFunc("/api/channels", h.legacyChannelList)
	mux.HandleFunc("/api/channels/send", h.channelSend)
	mux.HandleFunc("/api/channels/", h.legacyChannelHistory)
	mux.HandleFunc("/api/gateways/activity", h.activity)
	mux.HandleFunc("/api/gateways", h.list)

	// Notify subscription endpoints
	mux.HandleFunc("/api/notify/subscriptions", h.notifySubscriptions)
	mux.HandleFunc("/api/notify/subscriptions/", h.notifySubscriptionByChannel)
	mux.HandleFunc("/api/notify/activity/", h.notifyActivity)

	// Notifications home: connected apps + channels with resolved identities.
	mux.HandleFunc("/api/notifications/overview", h.notificationsOverview)
	// Manual re-resolution of channel display metadata (names, kinds).
	mux.HandleFunc("/api/gateways/channels/refresh", h.refreshChannelMeta)

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
	case rest == "pair" || rest == "pair/status":
		h.gatewayPair(w, r, platform, rest)
	case rest == "channels" || strings.HasPrefix(rest, "channels/"):
		h.gatewayChannels(w, r, platform, strings.TrimPrefix(rest, "channels"))
	case rest == "api" || strings.HasPrefix(rest, "api/"):
		h.gatewayAPIProxy(w, r, platform, strings.TrimPrefix(rest, "api"))
	case rest == "react":
		h.gatewayReact(w, r, platform)
	default:
		// Existing: PATCH /api/gateways/{platform}
		h.byPlatform(w, r)
	}
}

// gatewayAPIProxy forwards requests to /api/gateways/{platform}/api/* to the adapter's HTTP handler.
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
func (h *GatewayHandler) gatewayHealth(w http.ResponseWriter, r *http.Request, platform string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if h.gw == nil {
		serviceUnavailable(w, r, "gateway", "gateway manager not available")
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

	// /api/gateways/{platform}/channels/{channel}/...
	channelParts := strings.SplitN(subpath, "/", 2)
	// Defensive against callers who pre-prefix the channel with the
	// platform (e.g. `POST /api/gateways/slack/channels/slack:general/agents`).
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
		// GET /api/gateways/{platform}/channels/{channel} — channel detail
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
		writeJSON(w, http.StatusOK, map[string]any{
			"channel_key":   channelName,
			"name":          channelParts[0],
			"platform":      platform,
			"subscriptions": subs,
		})
	}
}

// gatewayChannelAgents handles /api/gateways/{platform}/channels/{channel}/agents
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

	// Enrich with bot name and discovered channels from adapter status
	if h.gw != nil {
		discovered := h.gw.DiscoveredSources()
		for i := range platforms {
			status := h.gw.AdapterStatus(platforms[i].Platform)
			if status.BotName != "" {
				platforms[i].BotName = status.BotName
			}
			// Populate channels from adapter discovery
			prefix := platforms[i].Platform + ":"
			for _, ch := range discovered {
				if strings.HasPrefix(ch, prefix) {
					platforms[i].Channels = append(platforms[i].Channels, ch)
				}
			}
		}
	}

	// Include dynamically registered adapters not in config (e.g., WhatsApp via QR pairing).
	if h.gw != nil {
		configSet := make(map[string]bool)
		for _, p := range platforms {
			configSet[p.Platform] = true
		}
		for _, name := range h.gw.AdapterNames() {
			if !configSet[name] {
				status := h.gw.AdapterStatus(name)
				platforms = append(platforms, gatewayStatus{
					Platform: name,
					Enabled:  true,
					BotName:  status.BotName,
				})
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
	switch {
	case platform == "telegram":
		if cfg.Gateways.Telegram == nil {
			cfg.Gateways.Telegram = &workspace.TelegramGatewayConfig{}
		}
		if err := json.Unmarshal(body, cfg.Gateways.Telegram); err != nil {
			httpError(w, "invalid telegram config: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Keep Telegrams map in sync — buildGatewayManager / hot-start iterate it.
		if cfg.Gateways.Telegrams == nil {
			cfg.Gateways.Telegrams = make(map[string]*workspace.TelegramGatewayConfig)
		}
		cfg.Gateways.Telegrams[""] = cfg.Gateways.Telegram
	case strings.HasPrefix(platform, "telegram:"):
		label := strings.TrimPrefix(platform, "telegram:")
		if cfg.Gateways.Telegrams == nil {
			cfg.Gateways.Telegrams = make(map[string]*workspace.TelegramGatewayConfig)
		}
		tc := cfg.Gateways.Telegrams[label]
		if tc == nil {
			tc = &workspace.TelegramGatewayConfig{}
		}
		if err := json.Unmarshal(body, tc); err != nil {
			httpError(w, "invalid telegram config: "+err.Error(), http.StatusBadRequest)
			return
		}
		cfg.Gateways.Telegrams[label] = tc
	case platform == "discord":
		if cfg.Gateways.Discord == nil {
			cfg.Gateways.Discord = &workspace.DiscordGatewayConfig{}
		}
		if err := json.Unmarshal(body, cfg.Gateways.Discord); err != nil {
			httpError(w, "invalid discord config: "+err.Error(), http.StatusBadRequest)
			return
		}
	case platform == "slack":
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

	if err := cfg.Save(h.ws.SettingsFile()); err != nil {
		httpInternalError(w, "save config", err)
		return
	}

	// Mirror credentials into the secrets vault under canonical env names so
	// injectVaultSecrets can pick them up when spinning up a new agent.
	// Vault write is additive — preferences.json is still the authoritative
	// source for the gateway manager.
	switch {
	case platform == "telegram":
		if cfg.Gateways.Telegram != nil {
			h.writeVaultSecret("TELEGRAM_BOT_TOKEN", cfg.Gateways.Telegram.BotToken)
		}
		if err := h.ensureTelegramAdapter(""); err != nil {
			log.Warn("gateway: failed to hot-start telegram adapter", "error", err)
			// Config is saved; surface a soft warning rather than failing the
			// PATCH — operator can still restart if start fails (bad token etc).
		}
	case strings.HasPrefix(platform, "telegram:"):
		label := strings.TrimPrefix(platform, "telegram:")
		if tc := cfg.Gateways.Telegrams[label]; tc != nil {
			key := "TELEGRAM_BOT_TOKEN_" + strings.ToUpper(strings.ReplaceAll(label, "-", "_"))
			h.writeVaultSecret(key, tc.BotToken)
		}
		if err := h.ensureTelegramAdapter(label); err != nil {
			log.Warn("gateway: failed to hot-start telegram adapter", "label", label, "error", err)
		}
	case platform == "discord":
		if cfg.Gateways.Discord != nil {
			h.writeVaultSecret("DISCORD_BOT_TOKEN", cfg.Gateways.Discord.BotToken)
		}
	case platform == "slack":
		if cfg.Gateways.Slack != nil {
			h.writeVaultSecret("SLACK_BOT_TOKEN", cfg.Gateways.Slack.BotToken)
			h.writeVaultSecret("SLACK_APP_TOKEN", cfg.Gateways.Slack.AppToken)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "platform": platform})
}

// ensureTelegramAdapter registers and starts a Telegram long-poll adapter
// when credentials are saved while the daemon is already running.
// label "" is the default bot (adapter name "telegram").
func (h *GatewayHandler) ensureTelegramAdapter(label string) error {
	if h.gw == nil {
		return nil // no manager (should not happen after always-on manager)
	}
	if h.ws == nil || h.ws.Config == nil {
		return nil
	}

	var tc *workspace.TelegramGatewayConfig
	if label == "" {
		tc = h.ws.Config.Gateways.Telegram
		if tc == nil && h.ws.Config.Gateways.Telegrams != nil {
			tc = h.ws.Config.Gateways.Telegrams[""]
		}
	} else if h.ws.Config.Gateways.Telegrams != nil {
		tc = h.ws.Config.Gateways.Telegrams[label]
	}
	if tc == nil || !tc.Enabled || tc.BotToken == "" {
		return nil
	}

	adapterName := "telegram"
	if label != "" {
		adapterName = "telegram:" + label
	}
	// Already registered and (hopefully) running — StartAdapter is idempotent.
	if existing := h.gw.GetAdapter(adapterName); existing != nil {
		return h.gw.StartAdapter(existing)
	}

	adapter := bctelegram.NewNamed(adapterName, tc.BotToken, tc.Mode)
	if err := adapter.DiscoverViaUpdate(); err != nil {
		log.Warn("telegram: discovery failed during hot-start", "adapter", adapterName, "error", err)
	}
	if err := h.gw.StartAdapter(adapter); err != nil {
		return err
	}
	log.Info("gateway: telegram adapter registered", "name", adapterName)
	return nil
}

// legacyChannelList returns gateway channels in the old Channel format
// so the frontend's listChannels() call still works after pkg/channel deletion.
// channelSend handles POST /api/channels/send — routes a text message from a
// named sender through the gateway to an external platform channel.
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
		gwChannelNames = h.gw.DiscoveredSources()
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

// gatewayReact handles POST /api/gateways/{platform}/react — sends an emoji
// reaction to a specific message via the gateway adapter.
//
// Request body:
//
//	{
//	  "channel":    "whatsapp:family",  // bc channel key
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

// gatewayPair handles QR-code-based pairing for adapters that support it
// (currently WhatsApp). POST /api/gateways/{platform}/pair starts pairing
// and returns a QR code image. GET /api/gateways/{platform}/pair/status
// returns the current pairing state.
func (h *GatewayHandler) gatewayPair(w http.ResponseWriter, r *http.Request, platform, route string) {
	if h.gw == nil {
		serviceUnavailable(w, r, "gateway", "gateway manager not available")
		return
	}

	switch platform {
	case "whatsapp":
		h.whatsappPair(w, r, route)
	default:
		httpError(w, "pairing not supported for "+platform, http.StatusBadRequest)
	}
}

func (h *GatewayHandler) whatsappPair(w http.ResponseWriter, r *http.Request, route string) {
	adapter := h.gw.GetAdapter("whatsapp")

	// If no adapter registered yet, create one on the fly.
	if adapter == nil {
		stateDir := ""
		if h.ws != nil {
			stateDir = h.ws.StateDir() + "/gateways/whatsapp"
		}
		if stateDir == "" {
			httpError(w, "workspace not available", http.StatusServiceUnavailable)
			return
		}
		wa := bcwhatsapp.New(stateDir)
		h.gw.Register(wa)
		adapter = wa
	}

	wa, ok := adapter.(*bcwhatsapp.Adapter)
	if !ok {
		httpError(w, "whatsapp adapter type mismatch", http.StatusInternalServerError)
		return
	}

	// Always wire handler so messages flow into the notification system —
	// needed both for fresh pairing and reconnection from saved session.
	wa.SetHandler(func(n gateway.Notification) {
		if h.gw != nil {
			h.gw.HandleNotification("whatsapp", n)
		}
	})

	switch {
	case route == "pair" && r.Method == http.MethodPost:
		status, err := wa.StartPairing(r.Context())
		if err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// When pairing completes synchronously (device already paired / reconnect),
		// write the authenticated JID (phone number) into the vault so agents can
		// reference the session. Values are never logged.
		if status.State == "connected" && status.Phone != "" {
			h.writeVaultSecret("WHATSAPP_SESSION", status.Phone)
		}
		writeJSON(w, http.StatusOK, status)

	case route == "pair/status" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, wa.GetPairStatus())

	default:
		methodNotAllowed(w)
	}
}
