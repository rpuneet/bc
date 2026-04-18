// Package whatsapp implements a gateway.NotificationAdapter for
// Meta Cloud API (WhatsApp Business) webhooks.
package whatsapp

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

// Adapter implements gateway.NotificationAdapter for WhatsApp webhooks.
type Adapter struct {
	lastMessageAt time.Time
	handler       func(gateway.Notification)
	name          string
	verifyToken   string
	lastError     string
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a WhatsApp webhook adapter with the default name.
func New(verifyToken string) *Adapter {
	return &Adapter{name: "whatsapp", verifyToken: verifyToken}
}

// NewNamed creates a named WhatsApp adapter.
func NewNamed(name, verifyToken string) *Adapter {
	return &Adapter{name: name, verifyToken: verifyToken}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType  { return gateway.AdapterWebhook }
func (a *Adapter) Stop() error                { return nil }

// Start stores the handler. Webhook adapters do not maintain a connection.
func (a *Adapter) Start(_ context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	return nil
}

// HTTPHandler returns an http.Handler that handles WhatsApp webhook
// verification (GET) and event delivery (POST).
func (a *Adapter) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Webhook verification: Meta sends GET with hub.verify_token.
		if r.Method == http.MethodGet {
			if r.URL.Query().Get("hub.verify_token") == a.verifyToken {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, r.URL.Query().Get("hub.challenge")) //nolint:errcheck
				return
			}
			http.Error(w, "invalid verify token", http.StatusForbidden)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
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

		log.Info("whatsapp: received webhook", "sender", sender, "adapter", a.name)

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   "messages",
				Platform:  "whatsapp",
				Sender:    sender,
				Timestamp: now,
				Raw:       body,
			})
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

// Channels returns a single "messages" channel.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{{ID: "messages", Name: "messages", Platform: "whatsapp"}}
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

// extractSender pulls the sender phone number from a WhatsApp webhook payload.
func extractSender(body []byte) string {
	var payload struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Messages []struct {
						From string `json:"from"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		for _, e := range payload.Entry {
			for _, c := range e.Changes {
				if len(c.Value.Messages) > 0 && c.Value.Messages[0].From != "" {
					return c.Value.Messages[0].From
				}
			}
		}
	}
	return "whatsapp"
}
