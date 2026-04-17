package notify

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/rpuneet/bc/pkg/log"
)

// AgentSender is the interface for sending a message to an agent's tmux session.
// Implemented by *agent.AgentService (Send method).
type AgentSender interface {
	Send(ctx context.Context, name, message string) error
}

// Broadcaster pushes events to connected web clients via SSE/WebSocket.
// Implemented by *ws.Hub.
type Broadcaster interface {
	Publish(eventType string, data map[string]any)
}

// Service is the notification dispatch core. It receives inbound messages
// from gateway adapters and routes them to subscribed agents via tmux send-keys.
type Service struct {
	store      *Store
	agents     AgentSender
	hub        Broadcaster
	pruneEvery int // prune delivery log when entries exceed this per channel
}

// NewService creates a new notify service.
func NewService(store *Store, agents AgentSender, hub Broadcaster) *Service {
	return &Service{
		store:      store,
		agents:     agents,
		hub:        hub,
		pruneEvery: 1000,
	}
}

// Store returns the underlying store for direct access by handlers.
func (s *Service) Store() *Store { return s.store }

// platformPrefixRe strips "[platform] " prefix added by gateway inbound handlers,
// e.g. "[slack] jolly-vulture" → "jolly-vulture".
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

// Dispatch receives a normalized inbound message and delivers it to all
// subscribed agents. Runs in its own goroutine — never blocks the adapter.
func (s *Service) Dispatch(channel, platform, sender, senderID, content, messageID string, attachments []Attachment) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("notify: dispatch panic", "recover", r)
			}
		}()

		ctx := context.Background()

		// Store message for activity feed history
		if saveErr := s.store.SaveMessage(ctx, channel, sender, content); saveErr != nil {
			log.Warn("notify: save message failed", "channel", channel, "error", saveErr)
		}

		// Build notification
		mentions := extractMentions(content)
		n := Notification{
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

		// Get subscribers
		subs, err := s.store.Subscribers(ctx, channel)
		if err != nil {
			log.Warn("notify: failed to get subscribers", "channel", channel, "error", err)
			return
		}

		log.Info("notify: dispatch", "channel", channel, "sender", sender, "subscribers", len(subs))

		if len(subs) == 0 {
			return
		}

		mentionSet := make(map[string]bool, len(mentions))
		for _, m := range mentions {
			mentionSet[m] = true
		}

		// Strip platform prefix from sender for self-skip comparison.
		// Gateway inbound messages arrive as "[slack] agent-name" but
		// subscriptions store bare agent names like "agent-name".
		rawSender := platformPrefixRe.ReplaceAllString(sender, "")

		// Deliver to each subscriber
		for _, sub := range subs {
			// Self-skip: don't echo agent's own message back
			if strings.EqualFold(sub.Agent, rawSender) {
				continue
			}

			// @mention filter: if mention_only is ON, skip unless agent is mentioned
			if sub.MentionOnly && !mentionSet[strings.ToLower(sub.Agent)] {
				continue
			}

			// Deliver via tmux send-keys
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

			// Log delivery
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
