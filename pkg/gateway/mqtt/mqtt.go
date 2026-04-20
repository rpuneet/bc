// Package mqtt implements a gateway.NotificationAdapter for MQTT topics
// using the Eclipse Paho MQTT client.
package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/rpuneet/bc/pkg/gateway"
	"github.com/rpuneet/bc/pkg/log"
)

// Config holds MQTT connection parameters.
type Config struct {
	Broker   string   // e.g. "tcp://localhost:1883"
	ClientID string   // MQTT client ID
	Topics   []string // topics to subscribe (e.g. ["home/sensors/#", "alerts/+"])
	Username string
	Password string
}

// Adapter implements gateway.NotificationAdapter for MQTT.
type Adapter struct {
	cfg           Config
	client        pahomqtt.Client
	handler       func(gateway.Notification)
	lastMessageAt time.Time
	name          string
	lastError     string
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates an MQTT adapter.
func New(name string, cfg Config) *Adapter {
	return &Adapter{name: name, cfg: cfg}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterSocket }
func (a *Adapter) HTTPHandler() http.Handler { return nil }

// Start connects to the MQTT broker and subscribes to topics. Blocks until ctx done.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler

	opts := pahomqtt.NewClientOptions().
		AddBroker(a.cfg.Broker).
		SetClientID(a.cfg.ClientID).
		SetAutoReconnect(true).
		SetOnConnectHandler(func(_ pahomqtt.Client) {
			a.mu.Lock()
			a.connected = true
			a.lastError = ""
			a.mu.Unlock()
			log.Info("mqtt: connected", "broker", a.cfg.Broker)
			a.subscribe()
		}).
		SetConnectionLostHandler(func(_ pahomqtt.Client, err error) {
			a.mu.Lock()
			a.connected = false
			a.lastError = err.Error()
			a.mu.Unlock()
			log.Warn("mqtt: connection lost", "error", err)
		})

	if a.cfg.Username != "" {
		opts.SetUsername(a.cfg.Username)
	}
	if a.cfg.Password != "" {
		opts.SetPassword(a.cfg.Password)
	}

	client := pahomqtt.NewClient(opts)
	a.client = client

	token := client.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("mqtt: connect timeout to %s", a.cfg.Broker)
	}
	if token.Error() != nil {
		return fmt.Errorf("mqtt: connect to %s: %w", a.cfg.Broker, token.Error())
	}

	<-ctx.Done()
	client.Disconnect(1000)
	return nil
}

// subscribe subscribes to all configured topics.
func (a *Adapter) subscribe() {
	for _, topic := range a.cfg.Topics {
		t := topic
		token := a.client.Subscribe(t, 0, func(_ pahomqtt.Client, msg pahomqtt.Message) {
			a.handleMessage(t, msg)
		})
		if token.WaitTimeout(10 * time.Second) && token.Error() != nil {
			log.Warn("mqtt: subscribe failed", "topic", t, "error", token.Error())
			a.mu.Lock()
			a.lastError = "subscribe " + t + ": " + token.Error().Error()
			a.mu.Unlock()
			continue
		}
		log.Info("mqtt: subscribed", "topic", t)
	}
}

// Stop disconnects from the broker.
func (a *Adapter) Stop() error {
	if a.client != nil && a.client.IsConnected() {
		a.client.Disconnect(1000)
	}
	return nil
}

// Channels returns the configured topics as channels.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	channels := make([]gateway.ChannelInfo, 0, len(a.cfg.Topics))
	for _, t := range a.cfg.Topics {
		channels = append(channels, gateway.ChannelInfo{ID: t, Name: t, Platform: "mqtt"})
	}
	return channels
}

// Status returns the connection state.
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

func (a *Adapter) handleMessage(topic string, msg pahomqtt.Message) {
	now := time.Now()
	a.mu.Lock()
	a.lastMessageAt = now
	a.mu.Unlock()
	a.messageCount.Add(1)

	payload := msg.Payload()
	content := string(payload)

	log.Info("mqtt: message", "topic", topic, "content", gateway.Truncate(content, 50))

	if a.handler != nil {
		raw, _ := json.Marshal(map[string]interface{}{ //nolint:errcheck
			"topic":     msg.Topic(),
			"payload":   content,
			"qos":       msg.Qos(),
			"retained":  msg.Retained(),
			"messageId": msg.MessageID(),
		})
		a.handler(gateway.Notification{
			Channel:   topic,
			Platform:  "mqtt",
			Sender:    "mqtt",
			Content:   content,
			Timestamp: now,
			Raw:       raw,
		})
	}
}
