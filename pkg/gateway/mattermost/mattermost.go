// Package mattermost implements a gateway.NotificationAdapter for
// Mattermost outgoing webhooks with token validation.
package mattermost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rpuneet/bc/pkg/gateway"
	"github.com/rpuneet/bc/pkg/log"
)

// Adapter implements gateway.NotificationAdapter for Mattermost webhooks.
type Adapter struct {
	lastMessageAt time.Time
	handler       func(gateway.Notification)
	name          string
	token         string
	lastError     string
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a Mattermost webhook adapter with the default name.
func New(token string) *Adapter {
	return &Adapter{name: "mattermost", token: token}
}

// NewNamed creates a named Mattermost adapter.
func NewNamed(name, token string) *Adapter {
	return &Adapter{name: name, token: token}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType  { return gateway.AdapterWebhook }
func (a *Adapter) Stop() error                { return nil }

// Start stores the handler.
func (a *Adapter) Start(_ context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	return nil
}

// HTTPHandler returns an http.Handler that validates the outgoing webhook
// token and processes Mattermost payloads.
func (a *Adapter) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		// Validate outgoing webhook token.
		if a.token != "" {
			token := extractToken(body)
			if token != a.token {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
		}

		sender, channel := extractSenderChannel(body)
		now := time.Now()

		a.mu.Lock()
		a.lastMessageAt = now
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()

		a.messageCount.Add(1)

		log.Info("mattermost: received webhook",
			"sender", sender,
			"channel", channel,
			"adapter", a.name)

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   channel,
				Platform:  "mattermost",
				Sender:    sender,
				Timestamp: now,
				Raw:       body,
			})
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

// Channels returns a single channel for the adapter.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{{ID: a.name, Name: a.name, Platform: "mattermost"}}
}

// Status returns the adapter's connection state.
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

// extractToken pulls the token from a Mattermost outgoing webhook payload.
func extractToken(body []byte) string {
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		return payload.Token
	}
	return ""
}

// extractSenderChannel pulls sender and channel from the payload.
func extractSenderChannel(body []byte) (string, string) {
	var payload struct {
		UserName    string `json:"user_name"`
		ChannelName string `json:"channel_name"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		s := payload.UserName
		if s == "" {
			s = "mattermost"
		}
		c := payload.ChannelName
		if c == "" {
			c = "messages"
		}
		return s, c
	}
	return "mattermost", "messages"
}
