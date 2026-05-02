// Package pagerduty implements the gateway.NotificationAdapter for PagerDuty webhooks.
package pagerduty

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

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

var commonEventTypes = []string{"incident", "service", "escalation"}

// Adapter implements gateway.NotificationAdapter for PagerDuty webhooks (v2/v3).
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

func New(secret string) *Adapter             { return &Adapter{name: "pagerduty", secret: secret} }
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
		if a.secret != "" {
			sig := r.Header.Get("X-PagerDuty-Signature")
			if !validateSignature(a.secret, sig, body) {
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
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
		log.Info("pagerduty: received webhook", "event", eventType, "sender", sender, "adapter", a.name)
		if a.handler != nil {
			a.handler(gateway.Notification{Channel: eventType, Platform: "pagerduty", Sender: sender, Timestamp: now, Raw: body})
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

func (a *Adapter) Channels() []gateway.ChannelInfo {
	ch := make([]gateway.ChannelInfo, len(commonEventTypes))
	for i, e := range commonEventTypes {
		ch[i] = gateway.ChannelInfo{ID: e, Name: e, Platform: "pagerduty"}
	}
	return ch
}

func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{Connected: a.connected, LastMessageAt: a.lastMessageAt, Error: a.lastError, MessageCount: a.messageCount.Load()}
}

func validateSignature(secret, signature string, body []byte) bool {
	if signature == "" {
		return false
	}
	// PagerDuty v3 sends "v1=<hex>".
	const prefix = "v1="
	sigHex := signature
	if len(signature) > len(prefix) && signature[:len(prefix)] == prefix {
		sigHex = signature[len(prefix):]
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sigHex), []byte(expected))
}

func extractEventType(body []byte) string {
	var p struct {
		Event struct {
			EventType string `json:"event_type"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &p); err == nil && p.Event.EventType != "" {
		return p.Event.EventType
	}
	return "unknown"
}

func extractSender(body []byte) string {
	var p struct {
		Event struct {
			Agent struct {
				Summary string `json:"summary"`
			} `json:"agent"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &p); err == nil && p.Event.Agent.Summary != "" {
		return p.Event.Agent.Summary
	}
	return "pagerduty"
}
