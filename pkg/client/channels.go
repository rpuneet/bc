package client

import (
	"context"
	"fmt"
	"time"
)

// ChannelsClient provides channel operations via the daemon.
type ChannelsClient struct {
	client *Client
}

// ChannelInfo represents channel data returned by the daemon.
type ChannelInfo struct {
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Type         string    `json:"type,omitempty"`
	Members      []string  `json:"members"`
	MemberCount  int       `json:"member_count"`
	MessageCount int       `json:"message_count"`
}

// MessageInfo represents a channel message returned by the daemon.
type MessageInfo struct {
	Reactions map[string][]string `json:"reactions,omitempty"`
	CreatedAt time.Time           `json:"created_at"`
	Channel   string              `json:"channel"`
	Sender    string              `json:"sender"`
	Content   string              `json:"content"`
	Type      string              `json:"type,omitempty"`
	ID        int64               `json:"id,omitempty"`
}

// ChannelStatusInfo holds channel status summary.
type ChannelStatusInfo struct {
	ChannelCount int `json:"channel_count"`
	TotalMembers int `json:"total_members"`
}

// List returns all channels from the daemon.
func (ch *ChannelsClient) List(ctx context.Context) ([]ChannelInfo, error) {
	var channels []ChannelInfo
	if err := ch.client.get(ctx, "/api/channels", &channels); err != nil {
		return nil, err
	}
	return channels, nil
}

// Get returns a single channel.
func (ch *ChannelsClient) Get(ctx context.Context, name string) (*ChannelInfo, error) {
	var info ChannelInfo
	if err := ch.client.get(ctx, "/api/channels/"+name, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// Create creates a new channel.
func (ch *ChannelsClient) Create(ctx context.Context, name, description string) (*ChannelInfo, error) {
	body := map[string]string{"name": name, "description": description}
	var info ChannelInfo
	if err := ch.client.post(ctx, "/api/channels", body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// Update updates a channel's settings.
func (ch *ChannelsClient) Update(ctx context.Context, name, description string) (*ChannelInfo, error) {
	body := map[string]string{"description": description}
	var info ChannelInfo
	if err := ch.client.put(ctx, "/api/channels/"+name, body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// Delete removes a channel.
func (ch *ChannelsClient) Delete(ctx context.Context, name string) error {
	return ch.client.delete(ctx, "/api/channels/"+name)
}

// AddMember adds an agent to a channel.
func (ch *ChannelsClient) AddMember(ctx context.Context, chanName, agentName string) error {
	body := map[string]string{"agent_id": agentName}
	return ch.client.post(ctx, "/api/channels/"+chanName+"/members", body, nil)
}

// RemoveMember removes an agent from a channel.
func (ch *ChannelsClient) RemoveMember(ctx context.Context, chanName, agentName string) error {
	return ch.client.delete(ctx, "/api/channels/"+chanName+"/members/"+agentName)
}

// Send delivers a message to a channel as sender, reporting whether the
// daemon's gateway actually routed it. A channel with no configured route
// returns false with no error — that is the endpoint's contract, not a failure,
// so callers must check the flag before telling a user the message was sent.
//
// This posts to /api/channels/send, the same endpoint the web UI and the
// send_message MCP tool use. It previously posted to
// /api/channels/{name}/messages, which no route serves: /api/channels/ is
// handled by the history endpoint, which accepts GET only, so every CLI send
// failed with 405.
func (ch *ChannelsClient) Send(ctx context.Context, chanName, sender, message string) (bool, error) {
	body := map[string]string{"channel": chanName, "sender": sender, "message": message}
	var res struct {
		Sent bool `json:"sent"`
	}
	if err := ch.client.post(ctx, "/api/channels/send", body, &res); err != nil {
		return false, err
	}
	return res.Sent, nil
}

// History returns message history for a channel.
func (ch *ChannelsClient) History(ctx context.Context, chanName string, limit, offset int, agentFilter string) ([]MessageInfo, error) {
	path := fmt.Sprintf("/api/channels/%s/history?limit=%d&offset=%d", chanName, limit, offset)
	if agentFilter != "" {
		path += "&agent=" + agentFilter
	}
	var msgs []MessageInfo
	if err := ch.client.get(ctx, path, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// React adds or removes a reaction to a message.
func (ch *ChannelsClient) React(ctx context.Context, chanName string, msgID int, emoji, user string) (bool, error) {
	body := map[string]any{"msg_id": msgID, "emoji": emoji, "user": user}
	var result map[string]bool
	if err := ch.client.post(ctx, "/api/channels/"+chanName+"/react", body, &result); err != nil {
		return false, err
	}
	return result["added"], nil
}

// Status returns the channel status summary.
func (ch *ChannelsClient) Status(ctx context.Context) (*ChannelStatusInfo, error) {
	var status ChannelStatusInfo
	if err := ch.client.get(ctx, "/api/channels/status", &status); err != nil {
		return nil, err
	}
	return &status, nil
}
