// Package github implements the gateway.NotificationAdapter for GitHub webhooks.
package github

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

// commonEventTypes are the GitHub webhook events surfaced as channels.
var commonEventTypes = []string{
	"push",
	"pull_request",
	"issues",
	"issue_comment",
	"pull_request_review",
	"release",
	"deployment",
	"workflow_run",
}

// Adapter implements gateway.NotificationAdapter for GitHub webhooks.
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

// New creates a new GitHub webhook adapter with the given HMAC secret.
// If secret is empty, signature validation is skipped.
func New(secret string) *Adapter {
	return &Adapter{
		name:   "github",
		secret: secret,
	}
}

// NewNamed creates a named GitHub adapter for multi-repo setups
// (e.g. "github:bc", "github:trade").
func NewNamed(name, secret string) *Adapter {
	return &Adapter{
		name:   name,
		secret: secret,
	}
}

func (a *Adapter) Name() string { return a.name }

// Type returns AdapterWebhook since GitHub delivers events via HTTP POST.
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterWebhook }

// Start stores the handler. Webhook adapters do not maintain a connection.
func (a *Adapter) Start(_ context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	return nil
}

// Stop is a no-op for webhook adapters.
func (a *Adapter) Stop() error { return nil }

// HTTPHandler returns an http.Handler that validates and processes GitHub
// webhook payloads.
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

		// Validate HMAC signature if a secret is configured.
		if a.secret != "" {
			sig := r.Header.Get("X-Hub-Signature-256")
			if !validateSignature(a.secret, sig, body) {
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
		}

		eventType := r.Header.Get("X-GitHub-Event")
		if eventType == "" {
			eventType = "unknown"
		}

		// Extract sender.login from the JSON payload.
		sender := extractSender(body)

		now := time.Now()

		a.mu.Lock()
		a.lastMessageAt = now
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()

		a.messageCount.Add(1)

		log.Info("github: received webhook",
			"event", eventType,
			"sender", sender,
			"adapter", a.name)

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   eventType,
				Platform:  "github",
				Sender:    sender,
				Timestamp: now,
				Raw:       body,
			})
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

// Channels returns common GitHub event types as discoverable channels.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	channels := make([]gateway.ChannelInfo, len(commonEventTypes))
	for i, evt := range commonEventTypes {
		channels[i] = gateway.ChannelInfo{
			ID:       evt,
			Name:     evt,
			Platform: "github",
		}
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

// validateSignature checks X-Hub-Signature-256 against the HMAC of the body.
func validateSignature(secret, signature string, body []byte) bool {
	if signature == "" {
		return false
	}

	// GitHub sends "sha256=<hex>".
	const prefix = "sha256="
	if len(signature) <= len(prefix) {
		return false
	}
	sigHex := signature[len(prefix):]

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sigHex), []byte(expected))
}

// extractSender pulls sender.login from a GitHub webhook JSON payload.
func extractSender(body []byte) string {
	var payload struct {
		Sender struct {
			Login string `json:"login"`
		} `json:"sender"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Sender.Login != "" {
		return payload.Sender.Login
	}
	return "github"
}
