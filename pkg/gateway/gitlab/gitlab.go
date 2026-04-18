// Package gitlab implements the gateway.NotificationAdapter for GitLab webhooks.
package gitlab

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

var commonEventTypes = []string{"push", "merge_request", "pipeline", "issue", "note", "tag_push"}

// Adapter implements gateway.NotificationAdapter for GitLab webhooks.
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

func New(secret string) *Adapter              { return &Adapter{name: "gitlab", secret: secret} }
func NewNamed(name, secret string) *Adapter    { return &Adapter{name: name, secret: secret} }
func (a *Adapter) Name() string                { return a.name }
func (a *Adapter) Type() gateway.AdapterType   { return gateway.AdapterWebhook }
func (a *Adapter) Stop() error                 { return nil }
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
		// GitLab uses plain-text X-Gitlab-Token comparison.
		if a.secret != "" && r.Header.Get("X-Gitlab-Token") != a.secret {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		eventType := r.Header.Get("X-Gitlab-Event")
		if eventType == "" {
			eventType = "unknown"
		}
		sender := extractSender(body)
		now := time.Now()
		a.mu.Lock()
		a.lastMessageAt = now
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()
		a.messageCount.Add(1)
		log.Info("gitlab: received webhook", "event", eventType, "sender", sender, "adapter", a.name)
		if a.handler != nil {
			a.handler(gateway.Notification{Channel: eventType, Platform: "gitlab", Sender: sender, Timestamp: now, Raw: body})
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

func (a *Adapter) Channels() []gateway.ChannelInfo {
	ch := make([]gateway.ChannelInfo, len(commonEventTypes))
	for i, e := range commonEventTypes {
		ch[i] = gateway.ChannelInfo{ID: e, Name: e, Platform: "gitlab"}
	}
	return ch
}

func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{Connected: a.connected, LastMessageAt: a.lastMessageAt, Error: a.lastError, MessageCount: a.messageCount.Load()}
}

func extractSender(body []byte) string {
	var p struct {
		User struct {
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &p); err == nil {
		if p.User.Username != "" {
			return p.User.Username
		}
		if p.User.Name != "" {
			return p.User.Name
		}
	}
	return "gitlab"
}
