// Package mqtt implements a placeholder gateway.NotificationAdapter
// for MQTT broker connections. Real implementation requires an MQTT
// client library.
package mqtt

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rpuneet/bc/pkg/gateway"
	"github.com/rpuneet/bc/pkg/log"
)

// Adapter implements gateway.NotificationAdapter for MQTT (placeholder).
type Adapter struct {
	lastMessageAt time.Time
	handler       func(gateway.Notification)
	name          string
	brokerURL     string
	topic         string
	lastError     string
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates an MQTT adapter with the default name.
func New(brokerURL, topic string) *Adapter {
	return &Adapter{name: "mqtt", brokerURL: brokerURL, topic: topic}
}

// NewNamed creates a named MQTT adapter.
func NewNamed(name, brokerURL, topic string) *Adapter {
	return &Adapter{name: name, brokerURL: brokerURL, topic: topic}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType  { return gateway.AdapterSocket }
func (a *Adapter) HTTPHandler() http.Handler  { return nil }
func (a *Adapter) Stop() error                { return nil }

// Start is a placeholder that blocks until ctx is canceled.
// Real implementation requires an MQTT client library.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	log.Info("mqtt: adapter registered (placeholder — real implementation requires MQTT client library)",
		"adapter", a.name, "broker", a.brokerURL, "topic", a.topic)
	<-ctx.Done()
	return ctx.Err()
}

// Channels returns the configured topic as a channel.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	t := a.topic
	if t == "" {
		t = a.name
	}
	return []gateway.ChannelInfo{{ID: t, Name: t, Platform: "mqtt"}}
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
