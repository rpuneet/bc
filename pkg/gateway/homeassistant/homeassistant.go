// Package homeassistant implements a placeholder gateway.NotificationAdapter
// for the Home Assistant WebSocket API. Real implementation requires a
// WebSocket client library.
package homeassistant

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rpuneet/bc/pkg/gateway"
	"github.com/rpuneet/bc/pkg/log"
)

// Adapter implements gateway.NotificationAdapter for Home Assistant (placeholder).
type Adapter struct {
	lastMessageAt time.Time
	handler       func(gateway.Notification)
	name          string
	url           string
	token         string
	lastError     string
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a Home Assistant adapter with the default name.
func New(url, token string) *Adapter {
	return &Adapter{name: "homeassistant", url: url, token: token}
}

// NewNamed creates a named Home Assistant adapter.
func NewNamed(name, url, token string) *Adapter {
	return &Adapter{name: name, url: url, token: token}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterSocket }
func (a *Adapter) HTTPHandler() http.Handler { return nil }
func (a *Adapter) Stop() error               { return nil }

// Start is a placeholder that blocks until ctx is canceled.
// Real implementation requires a WebSocket client library for
// the Home Assistant WebSocket API.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	log.Info("homeassistant: adapter registered (placeholder — real implementation requires WebSocket client)",
		"adapter", a.name, "url", a.url)
	<-ctx.Done()
	return ctx.Err()
}

// Channels returns common Home Assistant event channels.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	types := []string{"state_changed", "automation_triggered", "script_started"}
	channels := make([]gateway.ChannelInfo, len(types))
	for i, t := range types {
		channels[i] = gateway.ChannelInfo{ID: t, Name: t, Platform: "homeassistant"}
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
