// Package irc implements a gateway.NotificationAdapter for IRC channels
// using the ergochat/irc-go library.
package irc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ergochat/irc-go/ircevent"
	"github.com/ergochat/irc-go/ircmsg"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

// Config holds IRC connection parameters.
type Config struct { //nolint:govet
	Server   string   // e.g. "irc.libera.chat:6697"
	Nick     string   // bot nickname
	Channels []string // channels to join (e.g. ["#mycel", "#dev"])
	UseTLS   bool
	Password string // optional server password
}

// Adapter implements gateway.NotificationAdapter for IRC.
type Adapter struct {
	cfg           Config
	conn          *ircevent.Connection
	handler       func(gateway.Notification)
	lastMessageAt time.Time
	name          string
	lastError     string
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates an IRC adapter with the given config.
func New(name string, cfg Config) *Adapter {
	return &Adapter{name: name, cfg: cfg}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterSocket }
func (a *Adapter) HTTPHandler() http.Handler { return nil }

// Start connects to IRC and joins configured channels. Blocks until ctx done.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler

	conn := ircevent.Connection{
		Server:        a.cfg.Server,
		Nick:          a.cfg.Nick,
		UseTLS:        a.cfg.UseTLS,
		Password:      a.cfg.Password,
		QuitMessage:   "mycel agent disconnecting",
		KeepAlive:     90 * time.Second, // send PING every 90s to detect dead connections (must be ≥ library Timeout of 60s)
		ReconnectFreq: 10 * time.Second, // reconnect after 10s on drop
	}

	conn.AddConnectCallback(func(_ ircmsg.Message) {
		for _, ch := range a.cfg.Channels {
			conn.Join(ch) //nolint:errcheck
		}
		a.mu.Lock()
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()
		log.Info("irc: connected and joined channels", "server", a.cfg.Server)
	})

	conn.AddCallback("PRIVMSG", func(msg ircmsg.Message) {
		nuh, _ := ircmsg.ParseNUH(msg.Source)
		nick := nuh.Name
		if nick == a.cfg.Nick {
			return
		}
		a.handleMessage(msg, nick)
	})

	conn.AddCallback("DISCONNECTED", func(_ ircmsg.Message) {
		a.mu.Lock()
		a.connected = false
		a.mu.Unlock()
	})

	if err := conn.Connect(); err != nil {
		return fmt.Errorf("irc: connect to %s: %w", a.cfg.Server, err)
	}
	a.conn = &conn

	<-ctx.Done()
	conn.Quit()
	return nil
}

// Stop disconnects from IRC.
func (a *Adapter) Stop() error {
	if a.conn != nil {
		a.conn.Quit()
	}
	return nil
}

// Channels returns the configured IRC channels.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	channels := make([]gateway.ChannelInfo, 0, len(a.cfg.Channels))
	for _, ch := range a.cfg.Channels {
		channels = append(channels, gateway.ChannelInfo{ID: ch, Name: ch, Platform: "irc"})
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

func (a *Adapter) handleMessage(msg ircmsg.Message, nick string) {
	now := time.Now()
	a.mu.Lock()
	a.lastMessageAt = now
	a.mu.Unlock()
	a.messageCount.Add(1)

	channel := ""
	if len(msg.Params) > 0 {
		channel = msg.Params[0]
	}
	content := ""
	if len(msg.Params) > 1 {
		content = msg.Params[len(msg.Params)-1]
	}

	log.Info("irc: message", "sender", nick, "channel", channel,
		"content", gateway.Truncate(content, 50))

	if a.handler != nil {
		raw, _ := json.Marshal(map[string]string{ //nolint:errcheck
			"nick":    nick,
			"source":  msg.Source,
			"target":  channel,
			"message": content,
		})
		a.handler(gateway.Notification{
			Channel:   channel,
			Platform:  "irc",
			Sender:    nick,
			Content:   content,
			Timestamp: now,
			Raw:       raw,
		})
	}
}
