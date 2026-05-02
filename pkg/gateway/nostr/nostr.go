// Package nostr implements a placeholder gateway.NotificationAdapter
// for Nostr relay WebSocket connections. Real implementation requires
// a WebSocket client library.
package nostr

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

// Adapter implements gateway.NotificationAdapter for Nostr (placeholder).
type Adapter struct {
	lastMessageAt time.Time
	handler       func(gateway.Notification)
	name          string
	relayURL      string
	lastError     string
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a Nostr adapter with the default name.
func New(relayURL string) *Adapter {
	return &Adapter{name: "nostr", relayURL: relayURL}
}

// NewNamed creates a named Nostr adapter.
func NewNamed(name, relayURL string) *Adapter {
	return &Adapter{name: name, relayURL: relayURL}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterSocket }
func (a *Adapter) HTTPHandler() http.Handler { return nil }
func (a *Adapter) Stop() error               { return nil }

// Start is a placeholder that blocks until ctx is canceled.
// Real implementation requires a WebSocket/Nostr client library.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	log.Info("nostr: adapter registered (placeholder — real implementation requires Nostr client library)",
		"adapter", a.name, "relay", a.relayURL)
	<-ctx.Done()
	return ctx.Err()
}

// Channels returns a single placeholder channel.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{{ID: a.name, Name: a.name, Platform: "nostr"}}
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
