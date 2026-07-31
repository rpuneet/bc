// Package netlify implements the gateway.NotificationAdapter for Netlify webhooks.
package netlify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

var commonEventTypes = []string{"deploy", "submission", "split_test"}

// Adapter implements gateway.NotificationAdapter for Netlify webhooks.
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

func New(secret string) *Adapter             { return &Adapter{name: "netlify", secret: secret} }
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
		// Netlify signs outgoing webhooks with a JWS (JWT, HS256) in the
		// X-Webhook-Signature header, keyed by the configured secret. The
		// JWT's "sha256" claim is the hex SHA-256 of the request body, so we
		// verify both the signature and that the body hash matches.
		if a.secret != "" {
			sig := r.Header.Get("X-Webhook-Signature")
			if !validateSignature(a.secret, sig, body) {
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
		}
		eventType := r.Header.Get("X-Netlify-Event")
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
		log.Info("netlify: received webhook", "event", eventType, "sender", sender, "adapter", a.name)
		if a.handler != nil {
			a.handler(gateway.Notification{Channel: eventType, Platform: "netlify", Sender: sender, Timestamp: now, Raw: body})
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok") //nolint:errcheck
	})
}

func (a *Adapter) Channels() []gateway.ChannelInfo {
	ch := make([]gateway.ChannelInfo, len(commonEventTypes))
	for i, e := range commonEventTypes {
		ch[i] = gateway.ChannelInfo{ID: e, Name: e, Platform: "netlify"}
	}
	return ch
}

func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{Connected: a.connected, LastMessageAt: a.lastMessageAt, Error: a.lastError, MessageCount: a.messageCount.Load()}
}

// validateSignature verifies Netlify's X-Webhook-Signature JWS (a compact JWT
// signed with HS256 using the shared secret). It checks the HMAC over the
// signing input and, when present, that the token's "sha256" claim equals the
// hex SHA-256 of the request body. Any malformed token fails closed.
func validateSignature(secret, token string, body []byte) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 { //nolint:mnd // a compact JWS always has exactly 3 dot-separated parts
		return false
	}
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return false
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return false
	}
	// The body-hash claim is optional; when set it must match the payload.
	if claims.SHA256 != "" {
		sum := sha256.Sum256(body)
		if !hmac.Equal([]byte(claims.SHA256), []byte(hex.EncodeToString(sum[:]))) {
			return false
		}
	}
	return true
}

func extractSender(body []byte) string {
	var p struct {
		Committer string `json:"committer"`
	}
	if err := json.Unmarshal(body, &p); err == nil && p.Committer != "" {
		return p.Committer
	}
	return "netlify"
}
