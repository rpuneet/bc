// Package googlechat implements a gateway.NotificationAdapter for
// Google Chat webhook events.
package googlechat

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

// Adapter implements gateway.NotificationAdapter for Google Chat webhooks.
type Adapter struct {
	lastMessageAt time.Time
	handler       func(gateway.Notification)
	name          string
	secret        string
	lastError     string
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a Google Chat webhook adapter with the default name.
func New(secret string) *Adapter {
	return &Adapter{name: "googlechat", secret: secret}
}

// NewNamed creates a named Google Chat adapter.
func NewNamed(name, secret string) *Adapter {
	return &Adapter{name: name, secret: secret}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType  { return gateway.AdapterWebhook }
func (a *Adapter) Stop() error                { return nil }

// Start stores the handler.
func (a *Adapter) Start(_ context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	return nil
}

// HTTPHandler returns an http.Handler that processes Google Chat events.
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

		eventType, sender := extractEvent(body)
		now := time.Now()

		a.mu.Lock()
		a.lastMessageAt = now
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()

		a.messageCount.Add(1)

		log.Info("googlechat: received webhook",
			"event", eventType,
			"sender", sender,
			"adapter", a.name)

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   eventType,
				Platform:  "googlechat",
				Sender:    sender,
				Timestamp: now,
				Raw:       body,
			})
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

// Channels returns common Google Chat event types.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	types := []string{"MESSAGE", "ADDED_TO_SPACE", "REMOVED_FROM_SPACE", "CARD_CLICKED"}
	channels := make([]gateway.ChannelInfo, len(types))
	for i, t := range types {
		channels[i] = gateway.ChannelInfo{ID: t, Name: t, Platform: "googlechat"}
	}
	return channels
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

// extractEvent pulls type and sender from a Google Chat event payload.
func extractEvent(body []byte) (string, string) {
	var event struct {
		Type   string `json:"type"`
		User   struct {
			DisplayName string `json:"displayName"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &event); err == nil {
		t := event.Type
		if t == "" {
			t = "unknown"
		}
		s := event.User.DisplayName
		if s == "" {
			s = "googlechat"
		}
		return t, s
	}
	return "unknown", "googlechat"
}
