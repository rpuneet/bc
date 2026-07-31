// Package mattermost implements a gateway.NotificationAdapter using the
// Mattermost WebSocket API for real-time message events.
package mattermost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

// Config holds Mattermost connection parameters.
type Config struct {
	URL   string // e.g. "https://mattermost.example.com"
	Token string // personal access token or bot token
}

// Adapter implements gateway.NotificationAdapter for Mattermost.
type Adapter struct {
	cfg           Config
	conn          *websocket.Conn
	handler       func(gateway.Notification)
	lastMessageAt time.Time
	name          string
	lastError     string
	botUserID     string // the bot's own user id, resolved via /users/me; used to skip its own posts
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a Mattermost adapter.
func New(name string, cfg Config) *Adapter {
	return &Adapter{name: name, cfg: cfg}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterSocket }
func (a *Adapter) HTTPHandler() http.Handler { return nil }

// Start connects via WebSocket and listens for posted messages.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler

	wsURL := strings.Replace(a.cfg.URL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/v4/websocket"

	header := http.Header{"Authorization": {"Bearer " + a.cfg.Token}}

	conn, wsResp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if wsResp != nil && wsResp.Body != nil {
		wsResp.Body.Close() //nolint:errcheck
	}
	if err != nil {
		return fmt.Errorf("mattermost: connect: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	// Authenticate via WebSocket challenge.
	authMsg := map[string]interface{}{
		"seq":    1,
		"action": "authentication_challenge",
		"data":   map[string]string{"token": a.cfg.Token},
	}
	if err := conn.WriteJSON(authMsg); err != nil {
		return fmt.Errorf("mattermost: auth: %w", err)
	}

	// Resolve the bot's own user id so we never echo its own posts.
	botID := a.fetchBotUserID(ctx)

	a.mu.Lock()
	a.conn = conn
	a.connected = true
	a.lastError = ""
	a.botUserID = botID
	a.mu.Unlock()
	log.Info("mattermost: connected", "url", a.cfg.URL, "bot_user_id", botID)

	// Close conn when context is canceled (unblocks ReadMessage).
	go func() {
		<-ctx.Done()
		conn.Close() //nolint:errcheck
	}()

	// Read loop.
	for {
		_, msg, readErr := conn.ReadMessage()
		if readErr != nil {
			a.mu.Lock()
			a.connected = false
			a.lastError = readErr.Error()
			a.conn = nil
			a.mu.Unlock()
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			return fmt.Errorf("mattermost: read: %w", readErr)
		}
		a.handleRaw(msg)
	}
}

func (a *Adapter) Stop() error {
	a.mu.Lock()
	if a.conn != nil {
		a.conn.Close() //nolint:errcheck //nolint:errcheck
		a.conn = nil
	}
	a.connected = false
	a.mu.Unlock()
	return nil
}

func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{{ID: "messages", Name: "messages", Platform: "mattermost"}}
}

func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{
		Connected:     a.connected,
		LastMessageAt: a.lastMessageAt,
		Error:         a.lastError,
		MessageCount:  a.messageCount.Load(),
	}
}

// fetchBotUserID resolves the bot's own user id via GET /api/v4/users/me.
// Best effort: on any failure it returns "" and self-filtering is skipped.
func (a *Adapter) fetchBotUserID(ctx context.Context) string {
	url := strings.TrimRight(a.cfg.URL, "/") + "/api/v4/users/me"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.Token)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close() //nolint:errcheck
	var me struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return ""
	}
	return me.ID
}

func (a *Adapter) handleRaw(msg []byte) {
	var evt struct {
		Event string `json:"event"`
		Data  struct {
			Post        string `json:"post"`
			ChannelName string `json:"channel_name"`
			SenderName  string `json:"sender_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(msg, &evt); err != nil {
		return
	}
	if evt.Event != "posted" {
		return
	}

	var post struct {
		Message   string `json:"message"`
		ChannelID string `json:"channel_id"`
		UserID    string `json:"user_id"`
	}
	if err := json.Unmarshal([]byte(evt.Data.Post), &post); err != nil {
		return
	}

	// Skip the bot's own posts to avoid feedback loops.
	a.mu.Lock()
	botID := a.botUserID
	a.mu.Unlock()
	if botID != "" && post.UserID == botID {
		return
	}

	now := time.Now()
	a.mu.Lock()
	a.lastMessageAt = now
	a.mu.Unlock()
	a.messageCount.Add(1)

	sender := evt.Data.SenderName
	channel := evt.Data.ChannelName
	if channel == "" {
		channel = post.ChannelID
	}

	log.Info("mattermost: message", "sender", sender, "channel", channel)

	if a.handler != nil {
		a.handler(gateway.Notification{
			Channel:   channel,
			Platform:  "mattermost",
			Sender:    sender,
			Content:   post.Message,
			Timestamp: now,
			Raw:       msg,
		})
	}
}
