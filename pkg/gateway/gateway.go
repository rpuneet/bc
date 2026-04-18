// Package gateway provides external messaging platform integrations.
// It bridges bc channels to platforms like Telegram, Discord, and Slack.
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
	// AdapterWebhook is an HTTP endpoint where the platform POSTs events to bc.
	AdapterWebhook AdapterType = "webhook"
	// AdapterPoll is timer-based polling where bc fetches new events.
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

// MessageSender is optionally implemented by NotificationAdapters that
// support outbound messaging (e.g., Slack, Telegram, Discord).
type MessageSender interface {
	// Send delivers a message to a platform channel.
	Send(ctx context.Context, channelID, sender, content string) error
}

// Notification is a normalized inbound event from an external platform.
// The Raw field contains the complete platform payload as JSON.
type Notification struct {
	Timestamp time.Time       `json:"timestamp"`
	Raw       json.RawMessage `json:"raw"`
	Channel   string          `json:"channel"`
	Platform  string          `json:"platform"`
	Sender    string          `json:"sender"`
	Mentions  []string        `json:"mentions"`
}

// ChannelInfo represents a discovered channel on a platform.
type ChannelInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

// AdapterStatus reports connection state for the web UI.
type AdapterStatus struct {
	LastMessageAt time.Time `json:"last_message_at,omitempty"`
	Error         string    `json:"error,omitempty"`
	BotName       string    `json:"bot_name,omitempty"`
	Connected     bool      `json:"connected"`
	MessageCount  int64     `json:"message_count"`
}

// --- Legacy interface (kept during migration) ---

// Adapter is the legacy interface for platform adapters.
// Deprecated: Use NotificationAdapter for new adapters.
type Adapter interface {
	// Name returns the platform identifier ("telegram", "discord", "slack").
	Name() string

	// Start connects to the platform and begins receiving messages.
	// Calls onMessage for each inbound message. Blocks until ctx is canceled.
	Start(ctx context.Context, onMessage func(InboundMessage)) error

	// Stop gracefully disconnects from the platform.
	Stop(ctx context.Context) error

	// Send delivers a message to a platform channel.
	Send(ctx context.Context, channelID, sender, content string) error

	// Channels returns all channels/groups the bot is a member of.
	Channels(ctx context.Context) ([]ExternalChannel, error)

	// Health returns nil if the adapter is connected and operational.
	Health(ctx context.Context) error
}

// FileSender is optionally implemented by adapters that support file uploads.
type FileSender interface {
	// SendFile uploads a file to a platform channel.
	SendFile(ctx context.Context, channelID, sender, filename string, data []byte, mimeType string) error
}

// StatusReporter is optionally implemented by legacy adapters that report connection state.
type StatusReporter interface {
	// Status returns the current connection state for UI display.
	Status() AdapterStatus
}

// Attachment represents a file attached to a message.
type Attachment struct {
	URL      string `json:"url"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Source   string `json:"source"` // "slack", "telegram", "discord", "local"
	FileID   string `json:"file_id,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

// InboundMessage is a normalized message from an external platform.
// Deprecated: NotificationAdapter uses Notification instead.
type InboundMessage struct {
	Timestamp   time.Time
	ChannelID   string
	ChannelName string
	Sender      string
	SenderID    string
	Content     string
	MessageID   string
	Attachments []Attachment
}

// ExternalChannel represents a channel/group on an external platform.
// Deprecated: NotificationAdapter uses ChannelInfo instead.
type ExternalChannel struct {
	ID   string
	Name string
	Type string // "group", "channel", "dm"
}

// Truncate shortens a string to n characters, appending "..." if truncated.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
