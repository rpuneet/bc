// Package discord implements the gateway.NotificationAdapter for Discord.
package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/rpuneet/bc/pkg/gateway"
	"github.com/rpuneet/bc/pkg/log"
)

// Adapter implements gateway.NotificationAdapter for Discord.
// It also supports outbound messaging via the Send method.
type Adapter struct {
	lastMessageAt time.Time
	session       *discordgo.Session
	handler       func(gateway.Notification)
	guildChannels map[string]string
	token         string
	lastError     string
	chatMu        sync.RWMutex
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a new Discord adapter.
func New(token string) *Adapter {
	return &Adapter{
		token:         token,
		guildChannels: make(map[string]string),
	}
}

func (a *Adapter) Name() string { return "discord" }

// Type returns AdapterSocket since Discord uses a WebSocket gateway.
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterSocket }

// Start connects to Discord and forwards events as Notifications.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler

	session, err := discordgo.New("Bot " + a.token)
	if err != nil {
		return fmt.Errorf("discord: failed to create session: %w", err)
	}
	a.session = session

	// Set intents: we need guild messages and message content
	session.Identify.Intents = discordgo.IntentGuilds |
		discordgo.IntentGuildMessages |
		discordgo.IntentMessageContent

	// Register message handler
	session.AddHandler(a.handleMessage)

	// Register ready handler to discover guilds/channels
	session.AddHandler(a.handleReady)

	if err := session.Open(); err != nil {
		return fmt.Errorf("discord: failed to connect: %w", err)
	}
	log.Info("discord: connected", "bot", session.State.User.Username)

	// Block until context is canceled
	<-ctx.Done()
	return session.Close()
}

// Stop gracefully disconnects.
func (a *Adapter) Stop() error {
	if a.session != nil {
		return a.session.Close()
	}
	return nil
}

// HTTPHandler returns nil since Discord uses a WebSocket gateway.
func (a *Adapter) HTTPHandler() http.Handler { return nil }

// Channels returns discovered channels.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	a.chatMu.RLock()
	defer a.chatMu.RUnlock()

	channels := make([]gateway.ChannelInfo, 0, len(a.guildChannels))
	for id, name := range a.guildChannels {
		channels = append(channels, gateway.ChannelInfo{
			ID:       id,
			Name:     name,
			Platform: "discord",
		})
	}
	return channels
}

// Status returns the current connection state.
func (a *Adapter) Status() gateway.AdapterStatus {
	a.chatMu.RLock()
	defer a.chatMu.RUnlock()
	return gateway.AdapterStatus{
		Connected:     a.connected,
		LastMessageAt: a.lastMessageAt,
		Error:         a.lastError,
		MessageCount:  a.messageCount.Load(),
	}
}

// --- MessageSender (outbound messaging) ---

// Send delivers a message to a Discord channel.
func (a *Adapter) Send(_ context.Context, channelID, sender, content string) error {
	if a.session == nil {
		return fmt.Errorf("discord: not connected")
	}

	// Format: **agent_name**: message
	text := fmt.Sprintf("**%s**: %s", sender, content)

	if _, err := a.session.ChannelMessageSend(channelID, text); err != nil {
		return fmt.Errorf("discord: send failed: %w", err)
	}

	log.Info("discord: sent message", "channel_id", channelID, "sender", sender)
	return nil
}

// Health returns nil if the adapter is connected and operational.
func (a *Adapter) Health(_ context.Context) error {
	if a.session == nil {
		return fmt.Errorf("discord: not connected")
	}
	if a.session.State == nil || a.session.State.User == nil {
		a.chatMu.Lock()
		a.connected = false
		a.lastError = "session not ready"
		a.chatMu.Unlock()
		return fmt.Errorf("discord: session not ready")
	}
	a.chatMu.Lock()
	a.connected = true
	a.lastError = ""
	a.chatMu.Unlock()
	return nil
}

// --- Internal handlers ---

// handleReady processes the Ready event to discover guilds and channels.
func (a *Adapter) handleReady(_ *discordgo.Session, r *discordgo.Ready) {
	log.Info("discord: ready", "guilds", len(r.Guilds))

	for _, guild := range r.Guilds {
		channels, err := a.session.GuildChannels(guild.ID)
		if err != nil {
			log.Warn("discord: failed to list channels", "guild", guild.ID, "error", err)
			continue
		}

		a.chatMu.Lock()
		for _, ch := range channels {
			if ch.Type == discordgo.ChannelTypeGuildText {
				a.guildChannels[ch.ID] = ch.Name
				log.Info("discord: discovered channel", "channel", ch.Name, "id", ch.ID, "guild", guild.ID)
			}
		}
		a.chatMu.Unlock()
	}
}

// handleMessage processes incoming Discord messages.
func (a *Adapter) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Skip bot's own messages
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Skip bot messages
	if m.Author.Bot {
		return
	}

	content := m.Content
	if content == "" {
		return
	}

	// Get channel name as "guild:channel" (resolve via API if not cached)
	a.chatMu.RLock()
	channelName, ok := a.guildChannels[m.ChannelID]
	a.chatMu.RUnlock()
	if !ok {
		chName := m.ChannelID
		if ch, err := s.Channel(m.ChannelID); err == nil && ch.Name != "" {
			chName = ch.Name
		}
		guildName := m.GuildID
		if guild, err := s.Guild(m.GuildID); err == nil && guild.Name != "" {
			guildName = guild.Name
		}
		channelName = guildName + ":" + chName
		a.chatMu.Lock()
		a.guildChannels[m.ChannelID] = channelName
		a.chatMu.Unlock()
	}

	sender := m.Author.Username
	if m.Member != nil && m.Member.Nick != "" {
		sender = m.Member.Nick
	}

	now := time.Now()

	a.chatMu.Lock()
	a.lastMessageAt = now
	a.chatMu.Unlock()

	a.messageCount.Add(1)

	log.Info("discord: received message",
		"channel", channelName,
		"sender", sender,
		"content", gateway.Truncate(content, 50))

	if a.handler != nil {
		// Marshal entire message — preserves embeds, attachments,
		// reactions, components, and all platform fields (raw passthrough).
		raw, err := json.Marshal(m)
		if err != nil {
			log.Warn("discord: failed to marshal event", "error", err)
			return
		}
		a.handler(gateway.Notification{
			Channel:   channelName,
			Platform:  "discord",
			Sender:    sender,
			Content:   content,
			Timestamp: now,
			Raw:       raw,
		})
	}
}
