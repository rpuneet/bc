// Package feishu implements a gateway.NotificationAdapter for
// Feishu (Lark) event subscription webhooks.
package feishu

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

// Adapter implements gateway.NotificationAdapter for Feishu webhooks.
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

// New creates a Feishu webhook adapter with the default name.
func New(secret string) *Adapter {
	return &Adapter{name: "feishu", secret: secret}
}

// NewNamed creates a named Feishu adapter.
func NewNamed(name, secret string) *Adapter {
	return &Adapter{name: name, secret: secret}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterWebhook }
func (a *Adapter) Stop() error               { return nil }

// Start stores the handler.
func (a *Adapter) Start(_ context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	return nil
}

// HTTPHandler returns an http.Handler that processes Feishu event
// subscriptions, including URL verification challenges.
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

		// Handle URL verification challenge.
		var challenge struct {
			Challenge string `json:"challenge"`
			Type      string `json:"type"`
		}
		if err := json.Unmarshal(body, &challenge); err == nil && challenge.Type == "url_verification" {
			w.Header().Set("Content-Type", "application/json")
			resp, _ := json.Marshal(map[string]string{"challenge": challenge.Challenge}) //nolint:errcheck
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(resp) //nolint:errcheck
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

		log.Info("feishu: received webhook",
			"event", eventType,
			"sender", sender,
			"adapter", a.name)

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   eventType,
				Platform:  "feishu",
				Sender:    sender,
				Timestamp: now,
				Raw:       body,
			})
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

// Channels returns common Feishu event types.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	types := []string{"im.message.receive_v1", "im.message.reaction.created_v1"}
	channels := make([]gateway.ChannelInfo, len(types))
	for i, t := range types {
		channels[i] = gateway.ChannelInfo{ID: t, Name: t, Platform: "feishu"}
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

// extractEvent pulls the event type and sender from a Feishu event payload.
func extractEvent(body []byte) (string, string) {
	var event struct {
		Header struct {
			EventType string `json:"event_type"`
		} `json:"header"`
		Event struct {
			Sender struct {
				SenderID struct {
					OpenID string `json:"open_id"`
				} `json:"sender_id"`
			} `json:"sender"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &event); err == nil {
		t := event.Header.EventType
		if t == "" {
			t = "unknown"
		}
		s := event.Event.Sender.SenderID.OpenID
		if s == "" {
			s = "feishu"
		}
		return t, s
	}
	return "unknown", "feishu"
}
