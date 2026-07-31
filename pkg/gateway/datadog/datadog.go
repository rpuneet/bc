// Package datadog implements the gateway.NotificationAdapter for Datadog webhooks.
package datadog

import (
	"context"
	"crypto/subtle"
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

var commonEventTypes = []string{"alert", "monitor"}

// Adapter implements gateway.NotificationAdapter for Datadog webhooks.
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

func New(secret string) *Adapter             { return &Adapter{name: "datadog", secret: secret} }
func NewNamed(name, secret string) *Adapter  { return &Adapter{name: name, secret: secret} }
func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterWebhook }
func (a *Adapter) Stop() error               { return nil }
func (a *Adapter) Start(_ context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	return nil
}

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
		// Datadog webhooks have NO built-in HMAC signature. The real
		// mechanism is a shared secret placed in the webhook URL query
		// (?secret=...) or the custom payload template ("secret": "..."),
		// which we compare in constant time when a secret is configured.
		if a.secret != "" {
			if !validateSecret(a.secret, r.URL.Query().Get("secret"), body) {
				http.Error(w, "invalid secret", http.StatusUnauthorized)
				return
			}
		}
		eventType := extractEventType(body)
		now := time.Now()
		a.mu.Lock()
		a.lastMessageAt = now
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()
		a.messageCount.Add(1)
		log.Info("datadog: received webhook", "event", eventType, "adapter", a.name)
		if a.handler != nil {
			a.handler(gateway.Notification{Channel: eventType, Platform: "datadog", Sender: "datadog", Timestamp: now, Raw: body})
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

func (a *Adapter) Channels() []gateway.ChannelInfo {
	ch := make([]gateway.ChannelInfo, len(commonEventTypes))
	for i, e := range commonEventTypes {
		ch[i] = gateway.ChannelInfo{ID: e, Name: e, Platform: "datadog"}
	}
	return ch
}

func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{Connected: a.connected, LastMessageAt: a.lastMessageAt, Error: a.lastError, MessageCount: a.messageCount.Load()}
}

// validateSecret reports whether the configured shared secret matches the
// secret supplied by the request — either the ?secret= query value or a
// top-level "secret" field in the JSON payload. Comparison is constant time.
func validateSecret(secret, querySecret string, body []byte) bool {
	if querySecret != "" && subtle.ConstantTimeCompare([]byte(querySecret), []byte(secret)) == 1 {
		return true
	}
	var p struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(body, &p); err == nil && p.Secret != "" &&
		subtle.ConstantTimeCompare([]byte(p.Secret), []byte(secret)) == 1 {
		return true
	}
	return false
}

func extractEventType(body []byte) string {
	var p struct {
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(body, &p); err == nil && p.EventType != "" {
		return p.EventType
	}
	return "alert"
}
