// Package irc implements a placeholder gateway.NotificationAdapter
// for IRC. Real implementation requires an IRC client library.
package irc

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rpuneet/bc/pkg/gateway"
	"github.com/rpuneet/bc/pkg/log"
)

// Adapter implements gateway.NotificationAdapter for IRC (placeholder).
type Adapter struct {
	lastMessageAt time.Time
	handler       func(gateway.Notification)
	name          string
	server        string
	lastError     string
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates an IRC adapter with the default name.
func New(server string) *Adapter {
	return &Adapter{name: "irc", server: server}
}

// NewNamed creates a named IRC adapter.
func NewNamed(name, server string) *Adapter {
	return &Adapter{name: name, server: server}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType  { return gateway.AdapterSocket }
func (a *Adapter) HTTPHandler() http.Handler  { return nil }
func (a *Adapter) Stop() error                { return nil }

// Start is a placeholder that blocks until ctx is canceled.
// Real implementation requires an IRC client library.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	log.Info("irc: adapter registered (placeholder — real implementation requires IRC client library)",
		"adapter", a.name, "server", a.server)
	<-ctx.Done()
	return ctx.Err()
}

// Channels returns a single placeholder channel.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{{ID: a.name, Name: a.name, Platform: "irc"}}
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
