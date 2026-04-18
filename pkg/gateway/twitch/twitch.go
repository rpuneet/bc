// Package twitch implements a gateway.NotificationAdapter for
// Twitch EventSub webhooks with HMAC signature validation.
package twitch

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

// Adapter implements gateway.NotificationAdapter for Twitch EventSub webhooks.
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

// New creates a Twitch webhook adapter with the default name.
func New(secret string) *Adapter {
	return &Adapter{name: "twitch", secret: secret}
}

// NewNamed creates a named Twitch adapter.
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

// HTTPHandler returns an http.Handler that validates Twitch EventSub
// signatures and processes events.
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

		// Validate HMAC signature if secret is configured.
		if a.secret != "" {
			msgID := r.Header.Get("Twitch-Eventsub-Message-Id")
			msgTS := r.Header.Get("Twitch-Eventsub-Message-Timestamp")
			sig := r.Header.Get("Twitch-Eventsub-Message-Signature")
			if !validateSignature(a.secret, msgID, msgTS, sig, body) {
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
		}

		// Handle webhook callback verification.
		msgType := r.Header.Get("Twitch-Eventsub-Message-Type")
		if msgType == "webhook_callback_verification" {
			var challenge struct {
				Challenge string `json:"challenge"`
			}
			if err := json.Unmarshal(body, &challenge); err == nil {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, challenge.Challenge) //nolint:errcheck
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

		log.Info("twitch: received webhook",
			"event", eventType,
			"adapter", a.name)

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   eventType,
				Platform:  "twitch",
				Sender:    "twitch",
				Timestamp: now,
				Raw:       body,
			})
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

// Channels returns common Twitch EventSub event types.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	types := []string{"channel.follow", "channel.subscribe", "stream.online", "stream.offline"}
	channels := make([]gateway.ChannelInfo, len(types))
	for i, t := range types {
		channels[i] = gateway.ChannelInfo{ID: t, Name: t, Platform: "twitch"}
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

// validateSignature checks Twitch-Eventsub-Message-Signature.
// The HMAC is computed over: message_id + message_timestamp + body.
func validateSignature(secret, msgID, msgTS, signature string, body []byte) bool {
	if signature == "" {
		return false
	}
	const prefix = "sha256="
	if len(signature) <= len(prefix) {
		return false
	}
	sigHex := signature[len(prefix):]

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msgID))
	mac.Write([]byte(msgTS))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sigHex), []byte(expected))
}

// extractEventType pulls subscription.type from an EventSub payload.
func extractEventType(body []byte) string {
	var payload struct {
		Subscription struct {
			Type string `json:"type"`
		} `json:"subscription"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Subscription.Type != "" {
		return payload.Subscription.Type
	}
	return "unknown"
}
