// Package webhook implements a generic gateway.NotificationAdapter
// that receives arbitrary JSON payloads via HTTP POST.
package webhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

// Adapter implements gateway.NotificationAdapter for generic webhooks.
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

// New creates a new generic webhook adapter with the default name "webhook".
func New() *Adapter {
	return &Adapter{name: "webhook"}
}

// NewNamed creates a named webhook adapter (e.g. "webhook:deploy").
func NewNamed(name string) *Adapter {
	return &Adapter{name: name}
}

// NewWithSecret creates a named webhook adapter with shared-secret auth.
// The secret is validated against Authorization: Bearer <secret> or
// X-Webhook-Secret: <secret> headers.
func NewWithSecret(name, secret string) *Adapter {
	return &Adapter{
		name:   name,
		secret: secret,
	}
}

func (a *Adapter) Name() string { return a.name }

// Type returns AdapterWebhook.
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterWebhook }

// Start stores the handler. Webhook adapters do not maintain a connection.
func (a *Adapter) Start(_ context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	return nil
}

// Stop is a no-op for webhook adapters.
func (a *Adapter) Stop() error { return nil }

// HTTPHandler returns an http.Handler that receives and processes
// generic webhook payloads.
func (a *Adapter) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Validate secret if configured.
		if a.secret != "" && !a.validateAuth(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		sender := extractSender(body)
		now := time.Now()

		a.mu.Lock()
		a.lastMessageAt = now
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()

		a.messageCount.Add(1)

		log.Info("webhook: received payload",
			"adapter", a.name,
			"sender", sender)

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   a.name,
				Platform:  "webhook",
				Sender:    sender,
				Timestamp: now,
				Raw:       body,
			})
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

// Channels returns the adapter name as a single channel.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{
		{
			ID:       a.name,
			Name:     a.name,
			Platform: "webhook",
		},
	}
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

// validateAuth checks Authorization: Bearer <secret> or X-Webhook-Secret: <secret>.
func (a *Adapter) validateAuth(r *http.Request) bool {
	// Check Authorization: Bearer <secret>.
	if auth := r.Header.Get("Authorization"); auth != "" {
		const prefix = "Bearer "
		if strings.HasPrefix(auth, prefix) && subtle.ConstantTimeCompare([]byte(auth[len(prefix):]), []byte(a.secret)) == 1 {
			return true
		}
	}

	// Check X-Webhook-Secret header.
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Webhook-Secret")), []byte(a.secret)) == 1 {
		return true
	}

	return false
}

// extractSender tries common JSON fields for the sender identity.
func extractSender(body []byte) string {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return "webhook"
	}

	// Try common sender field names in order.
	for _, field := range []string{"sender", "user", "author", "from"} {
		raw, ok := payload[field]
		if !ok {
			continue
		}

		// Try as string first.
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && s != "" {
			return s
		}

		// Try as object with common sub-fields.
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		for _, sub := range []string{"login", "name", "username", "email"} {
			if v, ok := obj[sub]; ok {
				var name string
				if err := json.Unmarshal(v, &name); err == nil && name != "" {
					return name
				}
			}
		}
	}

	return "webhook"
}
