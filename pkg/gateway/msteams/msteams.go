// Package msteams implements a gateway.NotificationAdapter for
// Microsoft Teams Bot Framework webhooks.
package msteams

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

// Adapter implements gateway.NotificationAdapter for MS Teams webhooks.
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

// New creates an MS Teams webhook adapter with the default name.
func New(secret string) *Adapter {
	return &Adapter{name: "msteams", secret: secret}
}

// NewNamed creates a named MS Teams adapter.
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

// HTTPHandler returns an http.Handler that processes Bot Framework activities.
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

		activityType, sender := extractActivity(body)
		now := time.Now()

		a.mu.Lock()
		a.lastMessageAt = now
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()

		a.messageCount.Add(1)

		log.Info("msteams: received webhook",
			"activity", activityType,
			"sender", sender,
			"adapter", a.name)

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   activityType,
				Platform:  "msteams",
				Sender:    sender,
				Timestamp: now,
				Raw:       body,
			})
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

// Channels returns common Bot Framework activity types.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	types := []string{"message", "conversationUpdate", "messageReaction"}
	channels := make([]gateway.ChannelInfo, len(types))
	for i, t := range types {
		channels[i] = gateway.ChannelInfo{ID: t, Name: t, Platform: "msteams"}
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

// extractActivity pulls activity.type and from.name from a Bot Framework payload.
func extractActivity(body []byte) (string, string) {
	var activity struct {
		Type string `json:"type"`
		From struct {
			Name string `json:"name"`
		} `json:"from"`
	}
	if err := json.Unmarshal(body, &activity); err == nil {
		t := activity.Type
		if t == "" {
			t = "unknown"
		}
		s := activity.From.Name
		if s == "" {
			s = "msteams"
		}
		return t, s
	}
	return "unknown", "msteams"
}
