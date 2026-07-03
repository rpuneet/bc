// Package discord implements the gateway.NotificationAdapter for Discord.
package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

// slugify converts a guild or channel name to a stable, colon-safe slug:
// lowercase, with runs of spaces, colons, and hyphens collapsed into a single
// '-', other punctuation dropped, and no leading or trailing separators.
func slugify(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	pendingSep := false
	for _, r := range name {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_':
			if pendingSep && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingSep = false
			b.WriteRune(r)
		case r == ' ' || r == ':' || r == '-':
			pendingSep = true
		}
		// All other runes are dropped.
	}
	return b.String()
}

// channelKey builds the canonical channel name "<guild>:<channel>" from raw
// Discord names. The gateway manager prepends the "discord:" platform prefix,
// yielding the canonical bc channel "discord:<guild>:<channel>".
//
// The guild is always included (even for single-guild bots) so keys stay
// stable when the bot joins additional guilds, and the sidebar can show
// server and channel separately. Returns "" when no usable channel slug can
// be built; callers fall back to the raw channel ID.
func channelKey(guildName, channelName string) string {
	guild, channel := slugify(guildName), slugify(channelName)
	if channel == "" {
		return ""
	}
	if guild == "" {
		return channel
	}
	return guild + ":" + channel
}

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

// --- Internal handlers ---

// handleReady processes the Ready event to discover guilds and channels.
func (a *Adapter) handleReady(_ *discordgo.Session, r *discordgo.Ready) {
	log.Info("discord: ready", "guilds", len(r.Guilds))

	for _, guild := range r.Guilds {
		guildName := guild.ID
		if g, err := a.session.Guild(guild.ID); err == nil && g.Name != "" {
			guildName = g.Name
		}

		channels, err := a.session.GuildChannels(guild.ID)
		if err != nil {
			log.Warn("discord: failed to list channels", "guild", guildName, "error", err)
			continue
		}

		a.chatMu.Lock()
		for _, ch := range channels {
			if ch.Type == discordgo.ChannelTypeGuildText {
				// Canonical form "guild:channel" (slugified) — the gateway
				// manager keeps the separator so the sidebar shows server +
				// channel separately.
				key := channelKey(guildName, ch.Name)
				if key == "" {
					key = ch.ID
				}
				a.guildChannels[ch.ID] = key
				log.Info("discord: discovered channel", "channel", key, "id", ch.ID)
			}
		}
		a.chatMu.Unlock()
	}
}

// resolveChannelKey builds the canonical key for a channel missing from the
// discovery cache. It resolves channel and guild names via session state
// (falling back to the REST API), caches the result, and returns the raw
// channel ID if resolution fails so the message is never dropped.
func (a *Adapter) resolveChannelKey(s *discordgo.Session, channelID string) string {
	ch, err := s.State.Channel(channelID)
	if err != nil {
		if ch, err = s.Channel(channelID); err != nil {
			return channelID
		}
	}
	guildName := ch.GuildID
	if ch.GuildID != "" {
		if g, err := s.State.Guild(ch.GuildID); err == nil && g.Name != "" {
			guildName = g.Name
		} else if g, err := s.Guild(ch.GuildID); err == nil && g.Name != "" {
			guildName = g.Name
		}
	}
	key := channelKey(guildName, ch.Name)
	if key == "" {
		return channelID
	}
	a.chatMu.Lock()
	a.guildChannels[channelID] = key
	a.chatMu.Unlock()
	return key
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

	// Get canonical channel name; on cache miss (e.g., channel created after
	// startup), resolve it so inbound keys match the discovery scheme.
	a.chatMu.RLock()
	channelName, ok := a.guildChannels[m.ChannelID]
	a.chatMu.RUnlock()
	if !ok {
		channelName = a.resolveChannelKey(s, m.ChannelID)
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
