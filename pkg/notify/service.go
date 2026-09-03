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
// newsletters, bounces). Such messages are always recorded in the channel
// feed and published to the web UI. Agents are woken only when their
// subscription has deliver_automated (#3459); the default is off so one PR
// notification thread cannot wake every subscriber (#3457).
func Automated() DispatchOption {
	return func(o *dispatchOpts) { o.automated = true }
}

// RecordOutbound stores a message an agent sent out to a platform channel,
// so channel history reads as a conversation rather than only the half that came
// in. It also delivers the message to other subscribed agents (excluding the
// sender) because platform echoes (like Slack bot_message events) may not be
// reliable for waking other agents.
//
// Called after the gateway reports a successful send. Errors are logged rather
// than returned: the message has already left, so failing the caller's send
// would misreport what happened.
func (s *Service) RecordOutbound(channel, sender, content string) {
	if channel == "" || content == "" {
		return
	}
	ctx := s.ctx

	if err := s.store.SaveMessage(ctx, channel, sender, "", content); err != nil {
		log.Warn("notify: save outbound message failed", "channel", channel, "sender", sender, "error", err)
		return
	}

	// Deliver to other subscribed agents (local fan-out)
	s.deliverOutboundToSubscribers(ctx, channel, sender, content)

	// Same event shape the inbound path publishes, so an open channel view
	// appends the message live instead of waiting for a refetch.
	if s.hub != nil {
		s.hub.Publish("channel.message", map[string]any{
			"channel": channel,
			"message": map[string]any{
				"sender":  sender,
				"content": content,
				"type":    "text",
			},
		})
	}
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
		// Machine-generated mail always lands in the channel feed. Agents are
		// woken only when their subscription opts into deliver_automated
		// (#3459); default remains off (#3457).
		if o.automated {
			log.Info("notify: automated message — feed always; agents opt-in only",
				"channel", channel, "sender", sender)
		}
		s.deliverToSubscribers(ctx, deliverable{
			Channel:     channel,
			Platform:    platform,
			Sender:      sender,
			Content:     content,
			MessageID:   messageID,
			Mentions:    mentions,
			Attachments: attachments,
			Raw:         raw,
			Automated:   o.automated,
		})

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
	Automated   bool
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
	active, mutedAgents := partitionSubs(subs)

	// The connect-app flow subscribes agents to "{platform}:*" as a
	// catch-all, because the real channel is not known until a message
	// arrives (Telegram DMs land on telegram:<username|chat_id>, mail on
	// gmail:<sender>). When a channel has no active subscribers of its own,
	// fall back to the catch-all's subscribers for this delivery.
	//
	// This is deliberately a read, not a write. Copying the catch-all onto
	// each new channel used to leave a permanent subscription behind, and
	// platforms that mint a channel per correspondent (Gmail per sender,
	// WhatsApp per chat) turned one catch-all row into dozens nobody asked
	// for — see #3463. An explicit subscription on the real channel still
	// wins, so per-channel settings keep working.
	//
	// Muted rows on the real channel suppress catch-all for that agent only
	// (#3466) and do not count as active subscribers.
	//
	// Explicit subscribers on the channel keep their own settings, but they
	// must not suppress catch-all delivery for *other* agents that only have
	// "{platform}:*" (#3688). Merge catch-all agents that are not already
	// covered and not muted.
	if platform != "" {
		fallback, fErr := s.catchAllSubscribers(ctx, platform, channel)
		if fErr != nil {
			log.Warn("notify: catch-all lookup failed", "platform", platform, "channel", channel, "error", fErr)
		} else if len(fallback) > 0 {
			covered := make(map[string]bool, len(active))
			for _, sub := range active {
				covered[sub.Agent] = true
			}
			merged := 0
			for _, sub := range filterCatchAll(fallback, mutedAgents) {
				if covered[sub.Agent] {
					continue
				}
				active = append(active, sub)
				covered[sub.Agent] = true
				merged++
			}
			if merged > 0 {
				log.Info("notify: delivering via catch-all subscription",
					"channel", channel, "catch_all", CatchAllChannel(platform), "added", merged, "agents", len(active))
			}
		}
	}
	subs = active

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
		if sub.Muted {
			continue
		}
		// Self-skip: don't echo agent's own message back
		if strings.EqualFold(sub.Agent, rawSender) {
			continue
		}

		// Automated mail: skip unless this subscription opted in (#3459).
		if d.Automated && !sub.DeliverAutomated {
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
			// Persist as skipped so the activity UI counts the miss
			// without inflating failed-delivery totals (#3694).
			status = StatusSkipped
			errStr = sendErr.Error()
			log.Debug("notify: delivery skipped (agent offline)", "agent", sub.Agent, "channel", channel)
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

// catchAllSubscribers returns subscribers of the platform catch-all so a
// channel with no subscribers of its own can still be delivered. It only
// reads: writing a subscription per channel is what produced the runaway
// subscription lists in #3463.
//
// Canonical key is "{platform}:*" (#3467). Synthetic legacy "{platform}:general"
// rows (gmail/telegram/…) are read until migrateLegacyCatchAll rewrites them.
// Named-room ":general" (Slack #general, Discord guild:general, …) is never
// a catch-all (#3730).
//
// Returns nothing when realChannel is the canonical catch-all itself (already
// handled by the normal lookup) or belongs to another platform.
func (s *Service) catchAllSubscribers(ctx context.Context, platform, realChannel string) ([]Subscription, error) {
	catchAll := CatchAllChannel(platform)
	if realChannel == "" || realChannel == catchAll {
		return nil, nil
	}
	if !strings.HasPrefix(realChannel, platform+":") {
		return nil, nil
	}

	canonical, err := s.store.Subscribers(ctx, catchAll)
	if err != nil {
		return nil, err
	}
	if len(canonical) > 0 {
		return canonical, nil
	}
	legacy := LegacyCatchAllChannel(platform)
	if !IsLegacyCatchAll(legacy) {
		return nil, nil
	}
	return s.store.Subscribers(ctx, legacy)
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

// SetDeliverAutomated updates whether this subscription receives automated mail (#3459).
func (s *Service) SetDeliverAutomated(ctx context.Context, channel, agent string, deliver bool) error {
	return s.store.SetDeliverAutomated(ctx, channel, agent, deliver)
}

// SetMuted upserts or clears a mute that suppresses catch-all for this
// agent on the channel (#3466).
func (s *Service) SetMuted(ctx context.Context, channel, agent string, muted bool) error {
	return s.store.SetMuted(ctx, channel, agent, muted)
}

// partitionSubs splits channel rows into active subscribers and a mute set.
// Muted rows must not count as active (that would block catch-all for everyone).
func partitionSubs(subs []Subscription) (active []Subscription, muted map[string]bool) {
	muted = make(map[string]bool)
	for _, sub := range subs {
		if sub.Muted {
			muted[strings.ToLower(sub.Agent)] = true
			continue
		}
		active = append(active, sub)
	}
	return active, muted
}

func filterCatchAll(fallback []Subscription, muted map[string]bool) []Subscription {
	if len(muted) == 0 {
		return fallback
	}
	out := make([]Subscription, 0, len(fallback))
	for _, sub := range fallback {
		if muted[strings.ToLower(sub.Agent)] {
			continue
		}
		out = append(out, sub)
	}
	return out
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

// deliverOutboundToSubscribers delivers an outbound message to other subscribed
// agents (excluding the sender). Used for local fan-out when platform echoes
// (like Slack bot_message events) may not be reliable.
func (s *Service) deliverOutboundToSubscribers(ctx context.Context, channel, sender, content string) {
	// Get subscribers for this channel
	subs, subErr := s.store.Subscribers(ctx, channel)
	if subErr != nil {
		log.Warn("notify: failed to get subscribers for outbound", "channel", channel, "error", subErr)
		return
	}
	active, mutedAgents := partitionSubs(subs)

	// Extract platform from channel name (e.g., "slack:general" -> "slack")
	platform := PlatformOf(channel)

	// Merge catch-all agents not already covered (#3688).
	if platform != "" {
		fallback, fErr := s.catchAllSubscribers(ctx, platform, channel)
		if fErr != nil {
			log.Warn("notify: catch-all lookup failed for outbound", "platform", platform, "channel", channel, "error", fErr)
		} else if len(fallback) > 0 {
			covered := make(map[string]bool, len(active))
			for _, sub := range active {
				covered[sub.Agent] = true
			}
			for _, sub := range filterCatchAll(fallback, mutedAgents) {
				if covered[sub.Agent] {
					continue
				}
				active = append(active, sub)
				covered[sub.Agent] = true
			}
		}
	}
	subs = active

	log.Info("notify: outbound fan-out", "channel", channel, "sender", sender, "subscribers", len(subs))

	// Extract mentions from content for mention_only filtering
	mentions := extractMentions(content)
	mentionSet := make(map[string]bool, len(mentions))
	for _, m := range mentions {
		mentionSet[strings.ToLower(m)] = true
	}

	// Strip platform prefix from sender for self-skip comparison
	// Gateway messages arrive as "[slack] agent-name" but subscriptions
	// store bare agent names.
	rawSender := platformPrefixRe.ReplaceAllString(sender, "")

	// Create notification payload
	payload, err := json.Marshal(Notification{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Channel:   channel,
		Platform:  platform,
		Sender:    sender,
		Content:   content,
		Mentions:  mentions,
	})
	if err != nil {
		log.Error("notify: marshal outbound notification", "error", err)
		return
	}

	// Deliver to each subscribed agent
	for _, sub := range subs {
		if sub.Muted {
			continue
		}
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
			log.Info("notify: outbound delivered", "agent", sub.Agent, "channel", channel)
		case agentOfflineRe.MatchString(sendErr.Error()):
			status = StatusSkipped
			errStr = sendErr.Error()
			log.Debug("notify: outbound delivery skipped (agent offline)", "agent", sub.Agent, "channel", channel)
		default:
			status = StatusFailed
			errStr = sendErr.Error()
			log.Warn("notify: outbound delivery failed", "agent", sub.Agent, "channel", channel, "error", sendErr)
		}

		if logErr := s.store.LogDelivery(ctx, DeliveryEntry{
			Channel: channel,
			Agent:   sub.Agent,
			Status:  status,
			Error:   errStr,
			Preview: truncate(content, 120),
		}); logErr != nil {
			log.Warn("notify: log outbound delivery failed", "error", logErr)
		}
	}
}
