package notify

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
)

// agentOfflineRe matches the two well-known "agent isn't running" error
// text patterns from pkg/agent — used to record delivery attempts to
// offline agents as StatusSkipped rather than StatusFailed so the
// failed-delivery count only reflects genuine send errors.
var agentOfflineRe = regexp.MustCompile(`(?i)agent not running|is stopped`)

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
	dispatches sync.WaitGroup // in-flight Dispatch goroutines
	pruneEvery int            // prune delivery log when entries exceed this per channel
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

// DrainDispatches blocks until every in-flight Dispatch goroutine has
// finished, or the timeout elapses. Returns false on timeout. Called at
// shutdown so deliveries don't race store teardown.
func (s *Service) DrainDispatches(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		s.dispatches.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
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

// DispatchOption adjusts how a single inbound message is handled.
type DispatchOption func(*dispatchOpts)

type dispatchOpts struct {
	automated bool
}

// Automated marks the message as machine-generated (notification mail,
// newsletters, bounces). Such messages are still recorded in the channel feed
// and published to the web UI, but no subscribed agent is woken for them:
// nobody is waiting on a reply, and prompting every subscriber costs real
// tokens. Adapters decide what counts as automated.
func Automated() DispatchOption {
	return func(o *dispatchOpts) { o.automated = true }
}

// Dispatch receives a normalized inbound message and delivers it to
// subscribed agents only. Runs in its own goroutine — never blocks the adapter.
// extraMentions carries pre-extracted platform mentions (e.g. WhatsApp JID user
// parts) that the text-based mention regex cannot capture; they are merged with
// the standard @name extraction from content.
func (s *Service) Dispatch(channel, platform, sender, senderID, senderAvatar, content, messageID string, extraMentions []string, attachments []Attachment, raw json.RawMessage, opts ...DispatchOption) {
	var o dispatchOpts
	for _, opt := range opts {
		opt(&o)
	}
	s.dispatches.Add(1)
	go func() {
		defer s.dispatches.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Error("notify: dispatch panic", "recover", r)
			}
		}()

		ctx := s.ctx

		// Store message for activity feed history
		if saveErr := s.store.SaveMessage(ctx, channel, sender, senderAvatar, content); saveErr != nil {
			log.Warn("notify: save message failed", "channel", channel, "error", saveErr)
		}

		// Build mention set: text-extracted names plus any pre-supplied platform
		// identifiers (e.g. WhatsApp JID user parts). Lowercase + deduplicate.
		textMentions := extractMentions(content)
		mentions := textMentions
		if len(extraMentions) > 0 {
			seen := make(map[string]bool, len(textMentions)+len(extraMentions))
			for _, m := range textMentions {
				seen[m] = true
			}
			merged := make([]string, len(textMentions), len(textMentions)+len(extraMentions))
			copy(merged, textMentions)
			for _, m := range extraMentions {
				lm := strings.ToLower(m)
				if !seen[lm] {
					seen[lm] = true
					merged = append(merged, lm)
				}
			}
			mentions = merged
		}
		// Machine-generated mail earns a place in the channel feed and the web
		// UI, but not an agent's attention: nobody is waiting on a reply to a
		// GitHub notification or a newsletter, and waking every subscriber for
		// one costs real tokens.
		if o.automated {
			log.Info("notify: automated message — feed only, agents not woken",
				"channel", channel, "sender", sender)
		} else {
			s.deliverToSubscribers(ctx, deliverable{
				Channel:     channel,
				Platform:    platform,
				Sender:      sender,
				Content:     content,
				MessageID:   messageID,
				Mentions:    mentions,
				Attachments: attachments,
				Raw:         raw,
			})
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

// deliverable is one inbound message resolved far enough to be delivered:
// mentions are merged and the content is final.
type deliverable struct {
	Raw         json.RawMessage
	Channel     string
	Platform    string
	Sender      string
	Content     string
	MessageID   string
	Mentions    []string
	Attachments []Attachment
}

// deliverToSubscribers sends the message to every subscribed agent that
// passes the self-skip and @mention filters, logging each attempt.
func (s *Service) deliverToSubscribers(ctx context.Context, d deliverable) {
	channel, platform, sender, content := d.Channel, d.Platform, d.Sender, d.Content

	payload, err := json.Marshal(Notification{
		Raw:         d.Raw,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Channel:     channel,
		Platform:    platform,
		Sender:      sender,
		Content:     content,
		MessageID:   d.MessageID,
		Mentions:    d.Mentions,
		Attachments: d.Attachments,
	})
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

	// Setup wizard historically subscribed agents to "{platform}:general"
	// even when no such channel exists (Telegram DMs arrive as
	// telegram:<username|chat_id>). Copy placeholder subscriptions onto
	// the first real channel so legacy installs self-heal.
	if len(subs) == 0 && platform != "" {
		if n, mErr := s.migratePlaceholderSubs(ctx, platform, channel); mErr != nil {
			log.Warn("notify: placeholder migration failed", "platform", platform, "channel", channel, "error", mErr)
		} else if n > 0 {
			subs, subErr = s.store.Subscribers(ctx, channel)
			if subErr != nil {
				log.Warn("notify: failed to get subscribers after migration", "channel", channel, "error", subErr)
				return
			}
		}
	}

	log.Info("notify: dispatch", "channel", channel, "sender", sender, "subscribers", len(subs))

	mentionSet := make(map[string]bool, len(d.Mentions))
	for _, m := range d.Mentions {
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
		switch {
		case sendErr == nil:
			log.Info("notify: delivered", "agent", sub.Agent, "channel", channel)
		case agentOfflineRe.MatchString(sendErr.Error()):
			// Subscribed agent was offline when the message arrived
			// — the routing decision was correct, delivery just
			// wasn't attempted. Skip the delivery-log write entirely
			// so the failed-delivery count only reflects genuine
			// send errors, not routine offline-agent skips.
			log.Debug("notify: delivery skipped (agent offline)", "agent", sub.Agent, "channel", channel)
			continue
		default:
			status = StatusFailed
			errStr = sendErr.Error()
			log.Warn("notify: delivery failed", "agent", sub.Agent, "channel", channel, "error", sendErr)
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
}

// migratePlaceholderSubs copies subscriptions from "{platform}:general" onto
// the real channel when the placeholder still has subscribers and the real
// channel has none. Returns the number of agents copied. The placeholder row
// is left in place (copy, not rewrite) so operators can clean it up later.
func (s *Service) migratePlaceholderSubs(ctx context.Context, platform, realChannel string) (int, error) {
	placeholder := platform + ":general"
	if realChannel == "" || realChannel == placeholder {
		return 0, nil
	}
	// Only migrate onto channels for this platform.
	prefix := platform + ":"
	if !strings.HasPrefix(realChannel, prefix) {
		return 0, nil
	}

	legacy, err := s.store.Subscribers(ctx, placeholder)
	if err != nil {
		return 0, err
	}
	if len(legacy) == 0 {
		return 0, nil
	}

	copied := 0
	for _, sub := range legacy {
		if err := s.store.Subscribe(ctx, realChannel, sub.Agent, sub.MentionOnly); err != nil {
			return copied, err
		}
		copied++
	}
	if copied > 0 {
		log.Info("notify: migrated placeholder subscriptions",
			"from", placeholder, "to", realChannel, "agents", copied)
	}
	return copied, nil
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

// ChannelActivity returns recent delivery log entries for a channel, newest
// first. When before > 0, only entries older than that id are returned
// (cursor pagination for older pages).
func (s *Service) ChannelActivity(ctx context.Context, channel string, limit int, before int64) ([]DeliveryEntry, error) {
	return s.store.RecentActivity(ctx, channel, limit, before)
}

// AllSubscriptions returns all subscriptions across all channels.
func (s *Service) AllSubscriptions(ctx context.Context) ([]Subscription, error) {
	return s.store.AllSubscriptions(ctx)
}

// ChannelMessages returns recent messages for a channel (newest first).
func (s *Service) ChannelMessages(ctx context.Context, channel string, limit int, before int64) ([]MessageRecord, error) {
	return s.store.GetMessages(ctx, channel, limit, before)
}

// ChannelStats returns aggregated per-channel activity stats (message
// counts, member counts, last activity, top senders).
func (s *Service) ChannelStats(ctx context.Context) ([]ChannelStat, error) {
	return s.store.ChannelStats(ctx)
}

// Channels returns all persisted gateway channel mappings with their
// resolved display metadata (name, kind, participant count).
func (s *Service) Channels(ctx context.Context) ([]PersistedChannel, error) {
	return s.store.LoadChannels(ctx)
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
