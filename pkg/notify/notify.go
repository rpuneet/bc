// Package notify implements the notification gateway for delivering
// external platform events (Slack, Telegram, Discord, etc.) to subscribed
// mycel agents via tmux send-keys.
package notify

import (
	"encoding/json"
	"time"
)

// ChannelKey is the canonical identifier for an external channel.
// Format: "<platform>:<channel_name>", e.g., "slack:engineering".
type ChannelKey = string

// DeliveryStatus is the outcome of a tmux send-keys attempt.
type DeliveryStatus string

const (
	StatusDelivered DeliveryStatus = "delivered"
	StatusFailed    DeliveryStatus = "failed"
	StatusPending   DeliveryStatus = "pending"
)

// Notification is the JSON payload sent to subscribed agents via tmux send-keys.
type Notification struct {
	Raw         json.RawMessage `json:"raw,omitempty"`
	Timestamp   string          `json:"timestamp"`
	Channel     string          `json:"channel"`
	Platform    string          `json:"platform"`
	Sender      string          `json:"sender"`
	Content     string          `json:"content"`
	MessageID   string          `json:"message_id,omitempty"`
	Mentions    []string        `json:"mentions,omitempty"`
	Attachments []Attachment    `json:"attachments,omitempty"`
}

// Attachment describes a file shared on a channel.
type Attachment struct {
	Filename  string `json:"filename"`
	MimeType  string `json:"mime_type"`
	URL       string `json:"url,omitempty"`
	LocalPath string `json:"local_path,omitempty"`
	Size      int64  `json:"size"`
}

// Subscription ties an agent to a channel with delivery settings.
type Subscription struct {
	CreatedAt   time.Time `json:"created_at"`
	Channel     string    `json:"channel"`
	Agent       string    `json:"agent"`
	ID          int64     `json:"id"`
	MentionOnly bool      `json:"mention_only"`
	// Muted suppresses catch-all delivery for this agent on this channel
	// without counting as an active subscription (#3466).
	Muted bool `json:"muted"`
}

// DeliveryEntry records one delivery attempt in the activity log.
type DeliveryEntry struct {
	LoggedAt time.Time      `json:"logged_at"`
	Channel  string         `json:"channel"`
	Agent    string         `json:"agent"`
	Status   DeliveryStatus `json:"status"`
	Error    string         `json:"error,omitempty"`
	Preview  string         `json:"preview"`
	ID       int64          `json:"id"`
}

// truncate returns the first n characters of s.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
