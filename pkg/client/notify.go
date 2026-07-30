package client

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// NotifyClient provides notification subscription operations via the daemon.
type NotifyClient struct {
	client *Client
}

// Subscription represents an agent's subscription to a channel.
type Subscription struct {
	CreatedAt   time.Time `json:"created_at"`
	Channel     string    `json:"channel"`
	Agent       string    `json:"agent"`
	ID          int64     `json:"id"`
	MentionOnly bool      `json:"mention_only"`
}

// DeliveryEntry represents a delivery log entry.
type DeliveryEntry struct {
	LoggedAt time.Time `json:"logged_at"`
	Channel  string    `json:"channel"`
	Agent    string    `json:"agent"`
	Status   string    `json:"status"`
	Error    string    `json:"error,omitempty"`
	Preview  string    `json:"preview,omitempty"`
	ID       int64     `json:"id"`
}

// splitGatewayChannel splits "slack:eng" into ("slack", "eng").
// Returns empty strings if not a gateway channel.
func splitGatewayChannel(channel string) (platform, name string) {
	for _, p := range []string{"slack", "telegram", "discord", "github"} {
		if strings.HasPrefix(channel, p+":") {
			return p, channel[len(p)+1:]
		}
	}
	return "", ""
}

// ListSubscriptions returns all subscriptions.
func (n *NotifyClient) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	var subs []Subscription
	err := n.client.get(ctx, "/api/notify/subscriptions", &subs)
	return subs, err
}

// ChannelSubscriptions returns subscriptions for a specific channel.
func (n *NotifyClient) ChannelSubscriptions(ctx context.Context, channel string) ([]Subscription, error) {
	var subs []Subscription
	if gw, ch := splitGatewayChannel(channel); gw != "" {
		err := n.client.get(ctx, fmt.Sprintf("/api/apps/%s/channels/%s/agents", gw, ch), &subs)
		return subs, err
	}
	err := n.client.get(ctx, fmt.Sprintf("/api/notify/subscriptions/%s", channel), &subs)
	return subs, err
}

// Subscribe adds an agent to a channel.
func (n *NotifyClient) Subscribe(ctx context.Context, channel, agent string, mentionOnly bool) error {
	body := map[string]any{"agent": agent, "mention_only": mentionOnly}
	if gw, ch := splitGatewayChannel(channel); gw != "" {
		return n.client.post(ctx, fmt.Sprintf("/api/apps/%s/channels/%s/agents", gw, ch), body, nil)
	}
	body["channel"] = channel
	return n.client.post(ctx, "/api/notify/subscriptions", body, nil)
}

// Unsubscribe removes an agent from a channel.
func (n *NotifyClient) Unsubscribe(ctx context.Context, channel, agent string) error {
	if gw, ch := splitGatewayChannel(channel); gw != "" {
		return n.client.delete(ctx, fmt.Sprintf("/api/apps/%s/channels/%s/agents/%s", gw, ch, agent))
	}
	return n.client.delete(ctx, fmt.Sprintf("/api/notify/subscriptions/%s?agent=%s", channel, agent))
}

// Activity returns recent delivery log entries for a channel.
func (n *NotifyClient) Activity(ctx context.Context, channel string, limit int) ([]DeliveryEntry, error) {
	var entries []DeliveryEntry
	if gw, ch := splitGatewayChannel(channel); gw != "" {
		err := n.client.get(ctx, fmt.Sprintf("/api/apps/%s/channels/%s/activity?limit=%d", gw, ch, limit), &entries)
		return entries, err
	}
	err := n.client.get(ctx, fmt.Sprintf("/api/notify/activity/%s?limit=%d", channel, limit), &entries)
	return entries, err
}
