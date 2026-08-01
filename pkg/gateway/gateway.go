// Package gateway provides external messaging platform integrations.
// It bridges mycel channels to platforms like Telegram, Discord, and Slack.
package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// AdapterType identifies the connection pattern for a NotificationAdapter.
type AdapterType string

const (
	// AdapterSocket is a long-lived connection (WebSocket, polling loop).
	AdapterSocket AdapterType = "socket"
	// AdapterWebhook is an HTTP endpoint where the platform POSTs events to mycel.
	AdapterWebhook AdapterType = "webhook"
	// AdapterPoll is timer-based polling where mycel fetches new events.
	AdapterPoll AdapterType = "poll"
)

// NotificationAdapter handles the platform connection lifecycle.
// This is the new interface that all adapters should implement.
type NotificationAdapter interface {
	// Name returns the adapter identifier ("slack", "github", "telegram").
	Name() string

	// Type returns the connection pattern (socket, webhook, or poll).
	Type() AdapterType

	// Start connects to the platform and begins receiving notifications.
	// Calls handler for each inbound event with raw JSON payload.
	// Blocks until ctx is canceled. For webhook adapters, this is a no-op.
	Start(ctx context.Context, handler func(Notification)) error

	// Stop gracefully disconnects from the platform.
	Stop() error

	// HTTPHandler returns an http.Handler for webhook-based adapters.
	// Socket and poll adapters return nil.
	HTTPHandler() http.Handler

	// Channels returns discovered channels/groups the bot has access to.
	Channels() []ChannelInfo

	// Status returns the adapter's connection state for the web UI.
	Status() AdapterStatus
}

// Notification is a normalized inbound event from an external platform.
// The Raw field contains the complete platform payload as JSON.
type Notification struct {
	Timestamp    time.Time       `json:"timestamp"`
	Raw          json.RawMessage `json:"raw"`
	Channel      string          `json:"channel"`
	ChannelID    string          `json:"channel_id,omitempty"` // platform-native channel id (e.g. WhatsApp JID), if known
	Platform     string          `json:"platform"`
	Sender       string          `json:"sender"`
	SenderID     string          `json:"sender_id,omitempty"`     // platform-native sender id (e.g. WhatsApp JID)
	SenderAvatar string          `json:"sender_avatar,omitempty"` // raw platform avatar URL for the sender, when the adapter cheaply has one
	Content      string          `json:"content"`                 // human-readable text for display/storage
	MessageID    string          `json:"message_id,omitempty"`    // platform-native message id (for reactions, threading)
	Mentions     []string        `json:"mentions"`
}

// ChannelInfo represents a discovered channel on a platform.
type ChannelInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Kind     string `json:"kind,omitempty"` // group | person | channel | feed | other
}

// Channel kind values stored in channel metadata.
const (
	ChannelKindGroup   = "group"
	ChannelKindPerson  = "person"
	ChannelKindChannel = "channel"
	ChannelKindFeed    = "feed"
	ChannelKindOther   = "other"
)

// ChannelMeta is human-readable display metadata for a platform channel.
// AvatarURL, when set, is the raw platform URL of the channel's picture
// (a person's profile photo or a group's icon). It is stored as-is; the
// server wraps it in a loopback image-proxy URL before exposing it to the
// web UI so tokens/expiring URLs never reach the browser directly.
type ChannelMeta struct {
	DisplayName      string
	Kind             string // group | person | channel | feed | other
	AvatarURL        string
	ParticipantCount int
}

// ChannelIdentity is optionally implemented by adapters that can resolve
// human-readable channel metadata (names, kinds) from their platform.
type ChannelIdentity interface {
	// ResolveChannel returns display metadata for a platform channel id.
	ResolveChannel(ctx context.Context, platformID string) (ChannelMeta, error)
}

// AdapterStatus reports connection state for the web UI.
type AdapterStatus struct {
	LastMessageAt time.Time `json:"last_message_at,omitempty"`
	Error         string    `json:"error,omitempty"`
	BotName       string    `json:"bot_name,omitempty"`
	Connected     bool      `json:"connected"`
	MessageCount  int64     `json:"message_count"`
}

// Truncate shortens a string to n characters, appending "..." if truncated.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
