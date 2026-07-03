package notify

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
)

// AgentSender is the interface for sending a message to an agent's tmux session.
// Implemented by *agent.AgentService (Send method).
type AgentSender interface {
	Send(ctx context.Context, name, message string) error
	// SendAll broadcasts a message to all running agents.
	SendAll(ctx context.Context, message string) (sent int, err error)
}

// Broadcaster pushes events to connected web clients via SSE/WebSocket.
// Implemented by *ws.Hub.
type Broadcaster interface {
	Publish(eventType string, data map[string]any)
}

// Service is the notification dispatch core. It receives inbound messages
// from gateway adapters and routes them to subscribed agents via tmux send-keys.
type Service struct {
	ctx        context.Context
	store      *Store
	agents     AgentSender
	hub        Broadcaster
	pruneEvery int // prune delivery log when entries exceed this per channel
}

// NewService creates a new notify service.
func NewService(store *Store, agents AgentSender, hub Broadcaster) *Service {
	return NewServiceWithContext(context.Background(), store, agents, hub)
}

// NewServiceWithContext creates a notify service with the given context.
// The context is used for background dispatch goroutines instead of
// context.Background(), allowing callers to cancel in-flight deliveries
// during shutdown.
func NewServiceWithContext(ctx context.Context, store *Store, agents AgentSender, hub Broadcaster) *Service {
	return &Service{
		ctx:        ctx,
		store:      store,
		agents:     agents,
		hub:        hub,
		pruneEvery: 1000,
	}
}

// Store returns the underlying store for direct access by handlers.
func (s *Service) Store() *Store { return s.store }

// platformPrefixRe strips "[platform] " prefix from sender names.
var platformPrefixRe = regexp.MustCompile(`^\[[\w-]+\]\s+`)

var mentionRe = regexp.MustCompile(`@([a-zA-Z][a-zA-Z0-9_-]*)`)

// extractMentions parses @agent-name mentions from message content.
func extractMentions(content string) []string {
	matches := mentionRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool, len(matches))
	var mentions []string
	for _, m := range matches {
		name := strings.ToLower(m[1])
		if !seen[name] {
			seen[name] = true
			mentions = append(mentions, name)
		}
	}
	return mentions
}

// extractReactions best-effort extracts emoji reactions from a raw
// gateway payload. Supports Slack + Discord shapes:
//
//	Slack:   { "reactions": [ { "name": "eyes", "count": 2 }, ... ] }
//	Discord: { "reactions": [ { "emoji": { "name": "eyes" }, "count": 2 }, ... ] }
//
// Returns nil for platforms that don't include reactions or when the raw
// payload can't be parsed — the caller stores nil as SQL NULL. #3075.
func extractReactions(raw json.RawMessage) []MessageReaction {
	if len(raw) == 0 {
		return nil
	}
	// Both shapes carry a `reactions` array — parse loosely.
	var envelope struct {
		Reactions []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
			Emoji struct {
				Name string `json:"name"`
			} `json:"emoji"`
		} `json:"reactions"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil
	}
	if len(envelope.Reactions) == 0 {
		return nil
	}
	out := make([]MessageReaction, 0, len(envelope.Reactions))
	for _, r := range envelope.Reactions {
		name := r.Name
		if name == "" {
			name = r.Emoji.Name
		}
		if name == "" || r.Count <= 0 {
			continue
		}
		out = append(out, MessageReaction{Name: name, Count: r.Count})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}


// Dispatch receives a normalized inbound message and delivers it to
// subscribed agents only. Runs in its own goroutine — never blocks the adapter.
func (s *Service) Dispatch(channel, platform, sender, senderID, content, messageID string, attachments []Attachment, raw json.RawMessage) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("notify: dispatch panic", "recover", r)
			}
		}()

		ctx := s.ctx

		// Store message for activity feed history. Reactions are extracted
		// from the raw platform payload — Slack + Discord include them,
		// other platforms return nil which SaveMessage stores as SQL NULL.
		reactions := extractReactions(raw)
		if saveErr := s.store.SaveMessage(ctx, channel, sender, content, reactions); saveErr != nil {
			log.Warn("notify: save message failed", "channel", channel, "error", saveErr)
		}

		// Build notification
		mentions := extractMentions(content)
		n := Notification{
			Raw:         raw,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			Channel:     channel,
			Platform:    platform,
			Sender:      sender,
			Content:     content,
			MessageID:   messageID,
			Mentions:    mentions,
			Attachments: attachments,
		}
		payload, err := json.Marshal(n)
		if err != nil {
			log.Error("notify: marshal notification", "error", err)
			return
		}

		// Get subscribers for this channel
		subs, subErr := s.store.Subscribers(ctx, channel)
		if subErr != nil {
			log.Warn("notify: failed to get subscribers", "channel", channel, "error", subErr)
			return
		}

		log.Info("notify: dispatch", "channel", channel, "sender", sender, "subscribers", len(subs))

		mentionSet := make(map[string]bool, len(mentions))
		for _, m := range mentions {
			mentionSet[m] = true
		}

		// Strip platform prefix from sender for self-skip comparison.
		// Gateway messages arrive as "[slack] agent-name" but subscriptions
		// store bare agent names.
		rawSender := platformPrefixRe.ReplaceAllString(sender, "")

		// Deliver to each subscribed agent
		for _, sub := range subs {
			// Self-skip: don't echo agent's own message back
			if strings.EqualFold(sub.Agent, rawSender) {
				continue
			}

			// @mention filter: if mention_only, skip unless agent is mentioned
			if sub.MentionOnly && !mentionSet[strings.ToLower(sub.Agent)] {
				continue
			}

			sendErr := s.agents.Send(ctx, sub.Agent, string(payload))
			status := StatusDelivered
			errStr := ""
			if sendErr != nil {
				status = StatusFailed
				errStr = sendErr.Error()
				log.Warn("notify: delivery failed", "agent", sub.Agent, "channel", channel, "error", sendErr)
			} else {
				log.Info("notify: delivered", "agent", sub.Agent, "channel", channel)
			}

			if logErr := s.store.LogDelivery(ctx, DeliveryEntry{
				Channel: channel,
				Agent:   sub.Agent,
				Status:  status,
				Error:   errStr,
				Preview: truncate(content, 120),
			}); logErr != nil {
				log.Warn("notify: log delivery failed", "error", logErr)
			}
		}

		// Publish to web UI
		if s.hub != nil {
			s.hub.Publish("gateway.message", map[string]any{
				"channel":  channel,
				"platform": platform,
				"sender":   sender,
				"content":  truncate(content, 200),
				"mentions": mentions,
			})
		}

		// Prune old entries
		if err := s.store.PruneActivity(ctx, channel, s.pruneEvery); err != nil {
			log.Warn("notify: prune failed", "channel", channel, "error", err)
		}
	}()
}

// Subscribe adds an agent to a channel.
func (s *Service) Subscribe(ctx context.Context, channel, agent string, mentionOnly bool) error {
	return s.store.Subscribe(ctx, channel, agent, mentionOnly)
}

// Unsubscribe removes an agent from a channel.
func (s *Service) Unsubscribe(ctx context.Context, channel, agent string) error {
	return s.store.Unsubscribe(ctx, channel, agent)
}

// SetMentionOnly updates the @mention-only toggle for a subscription.
func (s *Service) SetMentionOnly(ctx context.Context, channel, agent string, mentionOnly bool) error {
	return s.store.SetMentionOnly(ctx, channel, agent, mentionOnly)
}

// ChannelSubscriptions returns all subscriptions for a channel.
func (s *Service) ChannelSubscriptions(ctx context.Context, channel string) ([]Subscription, error) {
	return s.store.Subscribers(ctx, channel)
}

// ChannelActivity returns recent delivery log entries for a channel.
func (s *Service) ChannelActivity(ctx context.Context, channel string, limit int) ([]DeliveryEntry, error) {
	return s.store.RecentActivity(ctx, channel, limit)
}

// AllSubscriptions returns all subscriptions across all channels.
func (s *Service) AllSubscriptions(ctx context.Context) ([]Subscription, error) {
	return s.store.AllSubscriptions(ctx)
}

// ChannelMessages returns recent messages for a channel (newest first).
func (s *Service) ChannelMessages(ctx context.Context, channel string, limit int, before int64) ([]MessageRecord, error) {
	return s.store.GetMessages(ctx, channel, limit, before)
}

// PruneOldActivity removes old delivery log entries for every channel,
// keeping the most recent keepPerChannel entries in each.
func (s *Service) PruneOldActivity(ctx context.Context, keepPerChannel int) error {
	channels, err := s.store.DeliveryChannels(ctx)
	if err != nil {
		return err
	}
	for _, ch := range channels {
		if err := s.store.PruneActivity(ctx, ch, keepPerChannel); err != nil {
			log.Warn("notify: prune failed", "channel", ch, "error", err)
		}
	}
	return nil
}
