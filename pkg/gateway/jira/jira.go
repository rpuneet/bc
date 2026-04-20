// Package jira implements the gateway.NotificationAdapter for Jira webhooks.
package jira

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

var commonEventTypes = []string{"issue_created", "issue_updated", "comment_created", "sprint_started"}

// Adapter implements gateway.NotificationAdapter for Jira webhooks.
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

func New(secret string) *Adapter             { return &Adapter{name: "jira", secret: secret} }
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
		// Jira uses URL-based secret: ?secret=<value>.
		if a.secret != "" && r.URL.Query().Get("secret") != a.secret {
			http.Error(w, "invalid secret", http.StatusUnauthorized)
			return
		}
		eventType := extractEventType(body)
		sender := extractSender(body)
		now := time.Now()
		a.mu.Lock()
		a.lastMessageAt = now
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()
		a.messageCount.Add(1)
		log.Info("jira: received webhook", "event", eventType, "sender", sender, "adapter", a.name)
		if a.handler != nil {
			a.handler(gateway.Notification{Channel: eventType, Platform: "jira", Sender: sender, Timestamp: now, Raw: body})
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

func (a *Adapter) Channels() []gateway.ChannelInfo {
	ch := make([]gateway.ChannelInfo, len(commonEventTypes))
	for i, e := range commonEventTypes {
		ch[i] = gateway.ChannelInfo{ID: e, Name: e, Platform: "jira"}
	}
	return ch
}

func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{Connected: a.connected, LastMessageAt: a.lastMessageAt, Error: a.lastError, MessageCount: a.messageCount.Load()}
}

func extractEventType(body []byte) string {
	var p struct {
		WebhookEvent string `json:"webhookEvent"`
	}
	if err := json.Unmarshal(body, &p); err == nil && p.WebhookEvent != "" {
		return p.WebhookEvent
	}
	return "unknown"
}

func extractSender(body []byte) string {
	var p struct {
		User struct {
			DisplayName string `json:"displayName"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &p); err == nil && p.User.DisplayName != "" {
		return p.User.DisplayName
	}
	return "jira"
}
