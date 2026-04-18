// Package grafana implements the gateway.NotificationAdapter for Grafana alert webhooks.
package grafana

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

	"github.com/rpuneet/bc/pkg/gateway"
	"github.com/rpuneet/bc/pkg/log"
)

var commonEventTypes = []string{"alert"}

// Adapter implements gateway.NotificationAdapter for Grafana alert webhooks.
type Adapter struct {
	lastMessageAt time.Time
	handler       func(gateway.Notification)
	name          string
	token         string
	lastError     string
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

func New(token string) *Adapter              { return &Adapter{name: "grafana", token: token} }
func NewNamed(name, token string) *Adapter    { return &Adapter{name: name, token: token} }
func (a *Adapter) Name() string               { return a.name }
func (a *Adapter) Type() gateway.AdapterType  { return gateway.AdapterWebhook }
func (a *Adapter) Stop() error                { return nil }
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
		if a.token != "" {
			auth := r.Header.Get("Authorization")
			expected := "Bearer " + a.token
			if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		eventType := extractStatus(body)
		now := time.Now()
		a.mu.Lock()
		a.lastMessageAt = now
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()
		a.messageCount.Add(1)
		log.Info("grafana: received webhook", "event", eventType, "adapter", a.name)
		if a.handler != nil {
			a.handler(gateway.Notification{Channel: eventType, Platform: "grafana", Sender: "grafana", Timestamp: now, Raw: body})
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

func (a *Adapter) Channels() []gateway.ChannelInfo {
	ch := make([]gateway.ChannelInfo, len(commonEventTypes))
	for i, e := range commonEventTypes {
		ch[i] = gateway.ChannelInfo{ID: e, Name: e, Platform: "grafana"}
	}
	return ch
}

func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{Connected: a.connected, LastMessageAt: a.lastMessageAt, Error: a.lastError, MessageCount: a.messageCount.Load()}
}

func extractStatus(body []byte) string {
	var p struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &p); err == nil && p.Status != "" {
		return strings.ToLower(p.Status)
	}
	return "alert"
}
