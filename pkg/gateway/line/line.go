// Package line implements a gateway.NotificationAdapter for
// LINE Messaging API webhooks with HMAC-SHA256 signature validation.
package line

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

// Adapter implements gateway.NotificationAdapter for LINE webhooks.
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

// New creates a LINE webhook adapter with the default name.
func New(secret string) *Adapter {
	return &Adapter{name: "line", secret: secret}
}

// NewNamed creates a named LINE adapter.
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

// HTTPHandler returns an http.Handler that validates X-Line-Signature
// and processes LINE webhook events.
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

		// Validate HMAC-SHA256 signature if secret is configured.
		if a.secret != "" {
			sig := r.Header.Get("X-Line-Signature")
			if !validateSignature(a.secret, sig, body) {
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
		}

		eventType, sender, content := extractEvent(body)
		now := time.Now()

		a.mu.Lock()
		a.lastMessageAt = now
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()

		a.messageCount.Add(1)

		log.Info("line: received webhook",
			"event", eventType,
			"sender", sender,
			"adapter", a.name)

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   eventType,
				Platform:  "line",
				Sender:    sender,
				Content:   content,
				Timestamp: now,
				Raw:       body,
			})
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

// Channels returns common LINE event types.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	types := []string{"message", "follow", "unfollow", "postback"}
	channels := make([]gateway.ChannelInfo, len(types))
	for i, t := range types {
		channels[i] = gateway.ChannelInfo{ID: t, Name: t, Platform: "line"}
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

// validateSignature checks X-Line-Signature (base64 HMAC-SHA256).
func validateSignature(secret, signature string, body []byte) bool {
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

// extractEvent pulls the first event's type, source user, and message text
// from a LINE payload. For message events the text lives in event.message.text;
// for non-message events (follow, postback, …) the content is empty.
func extractEvent(body []byte) (eventType, sender, content string) {
	var payload struct {
		Events []struct {
			Type   string `json:"type"`
			Source struct {
				UserID string `json:"userId"`
			} `json:"source"`
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && len(payload.Events) > 0 {
		t := payload.Events[0].Type
		if t == "" {
			t = "unknown"
		}
		s := payload.Events[0].Source.UserID
		if s == "" {
			s = "line"
		}
		return t, s, payload.Events[0].Message.Text
	}
	return "unknown", "line", ""
}
