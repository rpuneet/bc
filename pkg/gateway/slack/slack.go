// Package slackgw implements the gateway.NotificationAdapter for Slack.
package slackgw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

// Adapter implements gateway.NotificationAdapter for Slack using Socket Mode.
// It also supports outbound messaging via Send and SendFile methods.
type Adapter struct {
	lastMessageAt time.Time
	api           *slack.Client
	sm            *socketmode.Client
	handler       func(gateway.Notification)
	channelMap    map[string]string
	userCache     map[string]string
	botToken      string
	appToken      string
	botUserID     string
	botName       string
	lastError     string
	chatMu        sync.RWMutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a new Slack adapter using Socket Mode.
func New(botToken, appToken string) *Adapter {
	return &Adapter{
		botToken:   botToken,
		appToken:   appToken,
		channelMap: make(map[string]string),
		userCache:  make(map[string]string),
	}
}

func (a *Adapter) Name() string { return "slack" }

// Type returns AdapterSocket since Slack uses WebSocket via Socket Mode.
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterSocket }

// Start connects to Slack Socket Mode and forwards events as Notifications.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler

	api := slack.New(
		a.botToken,
		slack.OptionAppLevelToken(a.appToken),
	)
	a.api = api

	// Get bot user ID
	authResp, err := api.AuthTestContext(ctx)
	if err != nil {
		return fmt.Errorf("slack: auth test failed: %w", err)
	}
	a.botUserID = authResp.UserID
	a.botName = authResp.User
	if a.botName == "" {
		a.botName = authResp.Team
	}
	a.chatMu.Lock()
	a.connected = true
	a.chatMu.Unlock()
	log.Info("slack: connected", "bot_user_id", a.botUserID, "bot_name", a.botName, "team", authResp.Team)

	// Discover channels the bot is in
	if err := a.discoverChannels(ctx); err != nil {
		log.Warn("slack: channel discovery failed", "error", err)
	}

	// Create socket mode client
	sm := socketmode.New(api)
	a.sm = sm

	// Handle events in a goroutine
	go a.handleEvents(ctx, sm)

	// Run socket mode (blocks until context canceled)
	return sm.RunContext(ctx)
}

// Stop gracefully disconnects.
func (a *Adapter) Stop() error {
	// Socket mode client stops when context is canceled in Start
	return nil
}

// HTTPHandler returns nil since Slack uses Socket Mode, not webhooks.
func (a *Adapter) HTTPHandler() http.Handler { return nil }

// Channels returns discovered channels.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	a.chatMu.RLock()
	defer a.chatMu.RUnlock()

	channels := make([]gateway.ChannelInfo, 0, len(a.channelMap))
	for id, name := range a.channelMap {
		channels = append(channels, gateway.ChannelInfo{
			ID:       id,
			Name:     name,
			Platform: "slack",
			Kind:     gateway.ChannelKindChannel,
		})
	}
	return channels
}

// Status returns the current connection state.
func (a *Adapter) Status() gateway.AdapterStatus {
	a.chatMu.RLock()
	defer a.chatMu.RUnlock()
	return gateway.AdapterStatus{
		Connected:     a.connected,
		LastMessageAt: a.lastMessageAt,
		Error:         a.lastError,
		BotName:       a.botName,
		MessageCount:  a.messageCount.Load(),
	}
}

// --- MessageSender + FileSender (outbound messaging) ---

// Send delivers a message to a Slack channel.
func (a *Adapter) Send(ctx context.Context, channelID, sender, content string) error {
	if a.api == nil {
		return fmt.Errorf("slack: not connected")
	}

	_, _, err := a.api.PostMessageContext(ctx, channelID,
		slack.MsgOptionText(content, false),
		slack.MsgOptionUsername(sender),
		slack.MsgOptionIconEmoji(":robot_face:"),
	)
	if err != nil {
		return fmt.Errorf("slack: send failed: %w", err)
	}

	log.Info("slack: sent message", "channel_id", channelID, "sender", sender)
	return nil
}

// SendFile uploads a file to a Slack channel.
func (a *Adapter) SendFile(ctx context.Context, channelID, sender, filename string, data []byte, _ string) error {
	if a.api == nil {
		return fmt.Errorf("slack: not connected")
	}

	_, err := a.api.UploadFileContext(ctx, slack.UploadFileParameters{
		Filename:       filename,
		Reader:         bytes.NewReader(data),
		FileSize:       len(data),
		Channel:        channelID,
		Title:          fmt.Sprintf("%s: %s", sender, filename),
		InitialComment: fmt.Sprintf("Shared by %s", sender),
	})
	if err != nil {
		return fmt.Errorf("slack: file upload failed: %w", err)
	}

	log.Info("slack: uploaded file", "channel_id", channelID, "sender", sender, "filename", filename)
	return nil
}

// --- Internal helpers ---

// discoverChannels lists channels the bot is a member of.
func (a *Adapter) discoverChannels(ctx context.Context) error {
	params := &slack.GetConversationsParameters{
		Types:           []string{"public_channel", "private_channel"},
		ExcludeArchived: true,
		Limit:           200,
	}

	var allChannels []slack.Channel
	for {
		channels, nextCursor, err := a.api.GetConversationsContext(ctx, params)
		if err != nil {
			return fmt.Errorf("list conversations: %w", err)
		}
		allChannels = append(allChannels, channels...)
		if nextCursor == "" {
			break
		}
		params.Cursor = nextCursor
	}

	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	for _, ch := range allChannels {
		if ch.IsMember {
			a.channelMap[ch.ID] = ch.Name
			log.Info("slack: discovered channel", "channel", ch.Name, "id", ch.ID)
		}
	}
	log.Info("slack: channel discovery complete", "count", len(a.channelMap))
	return nil
}

// handleEvents processes Socket Mode events.
func (a *Adapter) handleEvents(ctx context.Context, sm *socketmode.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-sm.Events:
			if !ok {
				return
			}
			a.processEvent(sm, evt)
		}
	}
}

// processEvent handles a single Socket Mode event.
func (a *Adapter) processEvent(sm *socketmode.Client, evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			log.Warn("slack: failed to cast EventsAPI event")
			return
		}
		sm.Ack(*evt.Request) //nolint:errcheck // best-effort ack
		// Pass raw payload so file metadata can be extracted from the
		// original JSON (the typed struct drops unknown fields like files).
		var rawPayload json.RawMessage
		if evt.Request != nil {
			rawPayload = evt.Request.Payload
		}
		a.handleEventsAPI(eventsAPIEvent, rawPayload)

	case socketmode.EventTypeConnecting:
		log.Info("slack: connecting via Socket Mode...")

	case socketmode.EventTypeConnected:
		log.Info("slack: Socket Mode connected")

	case socketmode.EventTypeConnectionError:
		log.Warn("slack: Socket Mode connection error")

	case socketmode.EventTypeHello:
		log.Info("slack: Socket Mode hello received")

	default:
		log.Info("slack: unhandled event type", "type", evt.Type)
		if evt.Request != nil {
			sm.Ack(*evt.Request) //nolint:errcheck // best-effort ack
		}
	}
}

// handleEventsAPI processes Events API payloads.
func (a *Adapter) handleEventsAPI(event slackevents.EventsAPIEvent, rawPayload json.RawMessage) {
	switch event.Type {
	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.MessageEvent:
			a.handleMessageEvent(ev, rawPayload)
		}
	}
}

// handleMessageEvent processes a single message event.
func (a *Adapter) handleMessageEvent(ev *slackevents.MessageEvent, rawPayload json.RawMessage) {
	// Bot-impersonation posts (subtype="bot_message" with a Username
	// override) are how mycel agents publish to Slack via the shared
	// bot token. We let those through so other agents — and the
	// channel feed — see them. The notify layer's self-skip
	// (pkg/notify/service.go) prevents the sender from receiving
	// their own message back.
	//
	// CRITICAL: only honor `Username` when the event also came from
	// our own bot user. Other apps in the same workspace can post
	// with any `username` override they like, and treating that as
	// agent identity would let any installed app impersonate
	// `zen-zebra` / `lucid-meerkat`. Pinning on `ev.User == botUserID`
	// makes the impersonation token-bound: only callers holding our
	// own bot token can route as an agent.
	if ev.SubType == "bot_message" {
		if ev.User != a.botUserID || ev.Username == "" {
			return
		}
	} else {
		// Skip messages the bot itself emitted with no impersonation
		// (legacy outbound echo) and messages with no user attribution.
		if ev.User == a.botUserID || ev.User == "" {
			return
		}
		// Allow file_share subtype for image sharing; drop other
		// edit/delete/system subtypes.
		if ev.SubType != "" && ev.SubType != "file_share" {
			return
		}
	}

	content := ev.Text
	if content == "" && ev.SubType != "file_share" {
		return
	}
	if content == "" && ev.SubType == "file_share" {
		content = "[shared a file]"
	}

	// Extract file metadata from raw payload and append to content.
	if ev.SubType == "file_share" {
		if files := extractSlackFiles(rawPayload); len(files) > 0 {
			for _, f := range files {
				content += "\n" + f
			}
		}
	}

	// Resolve channel name
	a.chatMu.RLock()
	channelName, ok := a.channelMap[ev.Channel]
	a.chatMu.RUnlock()
	if !ok {
		if a.api != nil {
			if chInfo, err := a.api.GetConversationInfo(&slack.GetConversationInfoInput{
				ChannelID: ev.Channel,
			}); err == nil && chInfo != nil {
				channelName = chInfo.Name
				a.chatMu.Lock()
				a.channelMap[ev.Channel] = channelName
				a.chatMu.Unlock()
			}
		}
		if channelName == "" {
			channelName = ev.Channel
		}
	}

	// Resolve user name. For bot_message posts the impersonation
	// Username is the sender — the gate above already verified the
	// event came from our own bot user, so Username is trusted.
	sender := ev.User
	if ev.SubType == "bot_message" && ev.User == a.botUserID && ev.Username != "" {
		sender = ev.Username
	} else if a.api != nil {
		a.chatMu.RLock()
		cachedName, cached := a.userCache[ev.User]
		a.chatMu.RUnlock()
		if cached {
			sender = cachedName
		} else if userInfo, err := a.api.GetUserInfo(ev.User); err == nil {
			sender = userInfo.RealName
			if sender == "" {
				sender = userInfo.Name
			}
			a.chatMu.Lock()
			if a.userCache == nil {
				a.userCache = make(map[string]string)
			}
			a.userCache[ev.User] = sender
			a.chatMu.Unlock()
		} else {
			log.Warn("slack: failed to resolve user", "user_id", ev.User, "error", err)
		}
	}

	now := time.Now()

	a.chatMu.Lock()
	a.lastMessageAt = now
	a.chatMu.Unlock()

	a.messageCount.Add(1)

	log.Info("slack: received message",
		"channel", channelName,
		"sender", sender,
		"content", gateway.Truncate(content, 50))

	if a.handler != nil {
		// Marshal the entire Slack event — preserves files, attachments,
		// blocks, reactions, and all platform fields (raw passthrough).
		raw, err := json.Marshal(ev)
		if err != nil {
			log.Warn("slack: failed to marshal event", "error", err)
			return
		}
		a.handler(gateway.Notification{
			Channel:   channelName,
			Platform:  "slack",
			Sender:    sender,
			Content:   content,
			Timestamp: now,
			Raw:       raw,
		})
	}
}

// extractSlackFiles parses file metadata from the raw Slack event payload.
// The Slack Events API includes a "files" array in file_share messages, but
// the slackevents.MessageEvent struct does not expose it, so we extract it
// from the raw JSON.  Returns lines like:
//
//	📎 Screenshot.png (159 KB) [image/png]
func extractSlackFiles(rawPayload json.RawMessage) []string {
	if len(rawPayload) == 0 {
		return nil
	}
	// The payload wraps the event: {"event": {"files": [...]}}
	var envelope struct {
		Event struct {
			Files []struct {
				Name     string `json:"name"`
				Mimetype string `json:"mimetype"`
				Size     int64  `json:"size"`
			} `json:"files"`
		} `json:"event"`
	}
	if err := json.Unmarshal(rawPayload, &envelope); err != nil {
		return nil
	}
	var lines []string
	for _, f := range envelope.Event.Files {
		name := f.Name
		if name == "" {
			name = "file"
		}
		lines = append(lines, fmt.Sprintf("\U0001F4CE %s (%s) [%s]", name, humanSize(f.Size), f.Mimetype))
	}
	return lines
}

// humanSize formats bytes into a human-readable string.
func humanSize(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%d KB", b/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
