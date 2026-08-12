package notify

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// mockSender records SendToAgent calls.
type mockSender struct {
	errFn func(name string) error
	calls []sendCall
	mu    sync.Mutex
}

type sendCall struct {
	Name    string
	Message string
}

func (m *mockSender) Send(_ context.Context, name, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, sendCall{Name: name, Message: message})
	if m.errFn != nil {
		return m.errFn(name)
	}
	return nil
}

func (m *mockSender) SendAll(_ context.Context, message string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, sendCall{Name: "*", Message: message})
	return 1, nil
}

func (m *mockSender) getCalls() []sendCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]sendCall, len(m.calls))
	copy(out, m.calls)
	return out
}

// mockHub records Publish calls.
type mockHub struct {
	events   []string
	payloads []map[string]any
	mu       sync.Mutex
}

func (m *mockHub) Publish(eventType string, payload map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, eventType)
	m.payloads = append(m.payloads, payload)
}

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	store, err := OpenStore(d, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSubscribeUnsubscribe(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Subscribe two agents
	if err := store.Subscribe(ctx, "slack:eng", "eng-01", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Subscribe(ctx, "slack:eng", "eng-02", true); err != nil {
		t.Fatal(err)
	}

	// Verify
	subs, err := store.Subscribers(ctx, "slack:eng")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscribers, got %d", len(subs))
	}
	if subs[0].Agent != "eng-01" || subs[0].MentionOnly {
		t.Errorf("eng-01: expected mention_only=false, got %v", subs[0].MentionOnly)
	}
	if subs[1].Agent != "eng-02" || !subs[1].MentionOnly {
		t.Errorf("eng-02: expected mention_only=true, got %v", subs[1].MentionOnly)
	}

	// Unsubscribe
	if err = store.Unsubscribe(ctx, "slack:eng", "eng-01"); err != nil {
		t.Fatal(err)
	}
	subs, err = store.Subscribers(ctx, "slack:eng")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscriber after unsubscribe, got %d", len(subs))
	}
}

func TestSubscribeIdempotent(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	if err := store.Subscribe(ctx, "slack:eng", "eng-01", false); err != nil {
		t.Fatal(err)
	}
	// Subscribe again with different mention_only — should update
	if err := store.Subscribe(ctx, "slack:eng", "eng-01", true); err != nil {
		t.Fatal(err)
	}

	subs, err := store.Subscribers(ctx, "slack:eng")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscriber (idempotent), got %d", len(subs))
	}
	if !subs[0].MentionOnly {
		t.Error("expected mention_only to be updated to true")
	}
}

func TestSetMentionOnly(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	if err := store.Subscribe(ctx, "slack:eng", "eng-01", false); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMentionOnly(ctx, "slack:eng", "eng-01", true); err != nil {
		t.Fatal(err)
	}

	subs, err := store.Subscribers(ctx, "slack:eng")
	if err != nil {
		t.Fatal(err)
	}
	if !subs[0].MentionOnly {
		t.Error("expected mention_only=true after SetMentionOnly")
	}
}

func TestExtractMentions(t *testing.T) {
	tests := []struct {
		content  string
		expected []string
	}{
		{"@eng-01 review this PR", []string{"eng-01"}},
		{"@eng-01 @eng-02 both look", []string{"eng-01", "eng-02"}},
		{"no mentions here", nil},
		{"@eng-01 @eng-01 duplicate", []string{"eng-01"}},
		{"@ALL broadcast", []string{"all"}},
		{"hey @root can you check?", []string{"root"}},
	}

	for _, tt := range tests {
		got := extractMentions(tt.content)
		if len(got) != len(tt.expected) {
			t.Errorf("extractMentions(%q) = %v, want %v", tt.content, got, tt.expected)
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("extractMentions(%q)[%d] = %q, want %q", tt.content, i, got[i], tt.expected[i])
			}
		}
	}
}

func TestDispatchMentionFilter(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	sender := &mockSender{}
	hub := &mockHub{}
	svc := NewService(store, sender, hub)

	// eng-01 gets all messages, eng-02 is mention-only
	if err := store.Subscribe(ctx, "slack:eng", "eng-01", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Subscribe(ctx, "slack:eng", "eng-02", true); err != nil {
		t.Fatal(err)
	}

	// Message mentions eng-01 only — eng-02 (mention_only) should be skipped
	svc.Dispatch("slack:eng", "slack", "alice", "U123", "", "hey @eng-01 review this", "msg1", nil, nil, nil)

	time.Sleep(100 * time.Millisecond)

	calls := sender.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 delivery (eng-01 only), got %d: %v", len(calls), calls)
	}
	if calls[0].Name != "eng-01" {
		t.Errorf("expected delivery to eng-01, got %s", calls[0].Name)
	}
}

func TestDispatchSelfSkip(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	sender := &mockSender{}
	svc := NewService(store, sender, nil)

	if err := store.Subscribe(ctx, "slack:eng", "eng-01", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Subscribe(ctx, "slack:eng", "eng-02", false); err != nil {
		t.Fatal(err)
	}

	// eng-01 sends — should NOT be delivered back to eng-01
	svc.Dispatch("slack:eng", "slack", "[slack] eng-01", "U456", "", "I just pushed a fix", "msg2", nil, nil, nil)

	time.Sleep(100 * time.Millisecond)

	calls := sender.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 delivery (eng-02 only), got %d", len(calls))
	}
	if calls[0].Name != "eng-02" {
		t.Errorf("expected delivery to eng-02, got %s", calls[0].Name)
	}
}

func TestDeliveryLog(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Log some entries
	for range 5 {
		if err := store.LogDelivery(ctx, DeliveryEntry{
			Channel: "slack:eng",
			Agent:   "eng-01",
			Status:  StatusDelivered,
			Preview: "test message",
		}); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := store.RecentActivity(ctx, "slack:eng", 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (limit), got %d", len(entries))
	}
}

// TestMentionOnlyModeSwitch proves that toggling mention_only false→true→false
// takes effect immediately on the next Dispatch — no restart required.
func TestMentionOnlyModeSwitch(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	sender := &mockSender{}
	svc := NewService(store, sender, nil)

	// Start as mention-only.
	if err := store.Subscribe(ctx, "whatsapp:family", "zen-zebra", true); err != nil {
		t.Fatal(err)
	}

	// A message with no mention — zen-zebra must NOT receive it.
	svc.Dispatch("whatsapp:family", "whatsapp", "alice", "", "", "good morning everyone", "m1", nil, nil, nil)
	time.Sleep(80 * time.Millisecond)
	if calls := sender.getCalls(); len(calls) != 0 {
		t.Fatalf("mention-only mode: expected 0 deliveries, got %d", len(calls))
	}

	// Switch to all-messages — change persists immediately.
	if err := store.SetMentionOnly(ctx, "whatsapp:family", "zen-zebra", false); err != nil {
		t.Fatal(err)
	}

	// Same message without @mention must now be delivered.
	svc.Dispatch("whatsapp:family", "whatsapp", "alice", "", "", "good morning everyone", "m2", nil, nil, nil)
	time.Sleep(80 * time.Millisecond)
	calls := sender.getCalls()
	if len(calls) != 1 {
		t.Fatalf("all-messages mode: expected 1 delivery, got %d: %v", len(calls), calls)
	}
	if calls[0].Name != "zen-zebra" {
		t.Errorf("expected delivery to zen-zebra, got %s", calls[0].Name)
	}
}

// TestDispatchExtraMentions proves that pre-supplied platform mentions
// (e.g. WhatsApp JID user parts) are added to the mention set and cause
// a mention-only subscriber to receive the message.
func TestDispatchExtraMentions(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	sender := &mockSender{}
	svc := NewService(store, sender, nil)

	// zen-zebra is mention-only; its WhatsApp JID user part is "918051005416".
	if err := store.Subscribe(ctx, "whatsapp:family", "918051005416", true); err != nil {
		t.Fatal(err)
	}

	// Message content has no @name mention — but the platform supplies the JID.
	svc.Dispatch("whatsapp:family", "whatsapp", "alice", "", "",
		"hey look at this", // no text @mention
		"m1",
		[]string{"918051005416"}, // pre-extracted from ContextInfo.MentionedJID
		nil,
		nil,
	)
	time.Sleep(80 * time.Millisecond)

	calls := sender.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 delivery via extraMentions, got %d: %v", len(calls), calls)
	}
	if calls[0].Name != "918051005416" {
		t.Errorf("expected delivery to 918051005416, got %s", calls[0].Name)
	}
}

// TestMentionOnlyTextName proves that a mention_only subscriber IS delivered a message
// when the agent name appears as a typed "@agentname" token in the content.
// This is the primary agent-mention path — WhatsApp JID user parts (phone numbers)
// are phone numbers that never match agent names like "zen-zebra".
func TestMentionOnlyTextName(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	sender := &mockSender{}
	svc := NewService(store, sender, nil)

	// zen-zebra is mention-only; helper-bot receives all messages.
	if err := store.Subscribe(ctx, "whatsapp:family", "zen-zebra", true); err != nil {
		t.Fatal(err)
	}
	if err := store.Subscribe(ctx, "whatsapp:family", "helper-bot", false); err != nil {
		t.Fatal(err)
	}

	// Message with no @mention — zen-zebra must be skipped, helper-bot delivered.
	svc.Dispatch("whatsapp:family", "whatsapp", "alice", "", "", "good morning", "m1", nil, nil, nil)
	time.Sleep(80 * time.Millisecond)
	calls := sender.getCalls()
	if len(calls) != 1 || calls[0].Name != "helper-bot" {
		t.Fatalf("no-mention: expected only helper-bot, got %v", calls)
	}

	// Message with typed "@zen-zebra" — both must be delivered.
	svc.Dispatch("whatsapp:family", "whatsapp", "alice", "", "", "hey @zen-zebra check this", "m2", nil, nil, nil)
	time.Sleep(80 * time.Millisecond)
	calls = sender.getCalls()
	// Expect 2 more calls (total 3): helper-bot and zen-zebra for m2.
	if len(calls) != 3 {
		t.Fatalf("typed @mention: expected 3 total deliveries, got %d: %v", len(calls), calls)
	}
	hasZenZebra := false
	for _, c := range calls[1:] {
		if c.Name == "zen-zebra" {
			hasZenZebra = true
		}
	}
	if !hasZenZebra {
		t.Errorf("expected zen-zebra to receive m2 via typed @mention, got calls: %v", calls)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("truncate('hello world', 5) = %q, want 'hello...'", got)
	}
	if got := truncate("hi", 5); got != "hi" {
		t.Errorf("truncate('hi', 5) = %q, want 'hi'", got)
	}
}

// slowSender blocks each Send until released, so tests can hold a
// dispatch in flight deterministically.
type slowSender struct {
	release chan struct{}
}

func (s *slowSender) Send(_ context.Context, _, _ string) error {
	<-s.release
	return nil
}

func (s *slowSender) SendAll(_ context.Context, _ string) (int, error) { return 0, nil }

// TestDrainDispatches covers shutdown draining: DrainDispatches must wait
// for in-flight dispatch goroutines (they used to be fire-and-forget and
// could hit the store mid-teardown) and report a timeout when one is stuck.
func TestDrainDispatches(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	sender := &slowSender{release: make(chan struct{})}
	svc := NewService(store, sender, nil)

	if err := store.Subscribe(ctx, "slack:eng", "eng-01", false); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("slack:eng", "slack", "user", "U1", "", "hello", "m1", nil, nil, nil)

	// The dispatch is blocked inside Send — draining must time out.
	if svc.DrainDispatches(50 * time.Millisecond) {
		t.Fatal("DrainDispatches returned true while a dispatch was in flight")
	}

	// Release the sender; draining must now complete.
	close(sender.release)
	if !svc.DrainDispatches(5 * time.Second) {
		t.Fatal("DrainDispatches timed out after the dispatch was released")
	}
}

// TestCatchAllDeliversWithoutCreatingSubscriptions covers the connect-app
// flow: agents are subscribed to "{platform}:*" because the real
// per-conversation channel isn't known until a message arrives. Delivery must
// work — and must not leave a subscription behind on the real channel.
func TestCatchAllDeliversWithoutCreatingSubscriptions(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "telegram:*", "mdrndr-manager", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Subscribe(ctx, "telegram:*", "mdrndr-tui", false); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("telegram:ab010300", "telegram", "[telegram] Agni", "", "", "ping", "", nil, nil, nil)

	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch did not finish")
	}

	if calls := sender.getCalls(); len(calls) != 2 {
		t.Fatalf("expected 2 deliveries via the catch-all, got %d: %+v", len(calls), calls)
	}

	// The real channel must stay clean: the catch-all is read, never copied.
	subs, err := store.Subscribers(ctx, "telegram:ab010300")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 0 {
		t.Errorf("catch-all delivery created %d subscription(s) on the real channel: %+v", len(subs), subs)
	}

	// And the catch-all itself is untouched.
	catchAll, err := store.Subscribers(ctx, "telegram:*")
	if err != nil {
		t.Fatal(err)
	}
	if len(catchAll) != 2 {
		t.Errorf("expected the catch-all to remain with 2 agents, got %d", len(catchAll))
	}
}

// TestCatchAllDoesNotFanOutAcrossChannels is the #3463 regression test.
// Platforms that mint a channel per correspondent (Gmail per sender address,
// WhatsApp per chat) used to turn one catch-all row into one permanent
// subscription per correspondent — a live workspace had accumulated 7 Gmail and
// 53 WhatsApp subscriptions nobody created, each of them prompting an agent.
func TestCatchAllDoesNotFanOutAcrossChannels(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "gmail:*", "fast-crane", false); err != nil {
		t.Fatal(err)
	}

	// Mail from five different senders — five channels, as Gmail names a
	// channel after the sender address.
	senders := []string{
		"gmail:alertshdfcbankbankin",
		"gmail:hellonewsexpressvpncom",
		"gmail:newslettereconomictimesnewscom",
		"gmail:supportnpmjscom",
		"gmail:puneetexamplecom",
	}
	for i, ch := range senders {
		svc.Dispatch(ch, "gmail", "someone", "", "", "message", string(rune('a'+i)), nil, nil, nil)
	}
	if !svc.DrainDispatches(3 * time.Second) {
		t.Fatal("dispatches did not finish")
	}

	all, err := store.AllSubscriptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected the single gmail:* subscription to remain the only one, got %d: %+v",
			len(all), all)
	}
	if all[0].Channel != "gmail:*" {
		t.Errorf("surviving subscription is %q, want gmail:*", all[0].Channel)
	}

	// Delivery still reached the agent for every sender.
	if calls := sender.getCalls(); len(calls) != len(senders) {
		t.Errorf("expected %d deliveries via the catch-all, got %d", len(senders), len(calls))
	}
}

// TestExplicitSettingsWinButCatchAllPeersStillDeliver (#3688): an explicit
// mention-only sub keeps its own filter, but must not suppress catch-all peers.
func TestExplicitSettingsWinButCatchAllPeersStillDeliver(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "gmail:*", "broad-agent", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Subscribe(ctx, "gmail:noisysendercom", "focused-agent", true); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("gmail:noisysendercom", "gmail", "someone", "", "", "no mention here", "m1", nil, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch did not finish")
	}

	// focused-agent is mention-only and wasn't mentioned; broad-agent still
	// receives via catch-all merge.
	calls := sender.getCalls()
	if len(calls) != 1 || calls[0].Name != "broad-agent" {
		t.Fatalf("expected broad-agent only, got %+v", calls)
	}

	svc.Dispatch("gmail:noisysendercom", "gmail", "someone", "", "", "hey @focused-agent look", "m2", []string{"focused-agent"}, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch did not finish")
	}

	calls = sender.getCalls()
	got := map[string]int{}
	for _, c := range calls {
		got[c.Name]++
	}
	// m1→broad, m2→focused + broad
	if got["broad-agent"] != 2 || got["focused-agent"] != 1 {
		t.Fatalf("want broad=2 focused=1, got %+v (calls=%+v)", got, calls)
	}
}

// TestCatchAllRespectsMutedChannel: mute on the real channel suppresses
// catch-all for that agent only (#3466).
func TestCatchAllRespectsMutedChannel(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "telegram:*", "broad-agent", false); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMuted(ctx, "telegram:alice", "broad-agent", true); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("telegram:alice", "telegram", "alice", "", "", "hi muted", "m1", nil, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch did not finish")
	}
	if calls := sender.getCalls(); len(calls) != 0 {
		t.Fatalf("muted agent must not receive catch-all delivery, got %+v", calls)
	}

	svc.Dispatch("telegram:bob", "telegram", "bob", "", "", "hi bob", "m2", nil, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch did not finish")
	}
	calls := sender.getCalls()
	if len(calls) != 1 || calls[0].Name != "broad-agent" {
		t.Fatalf("expected catch-all delivery on unmuted channel, got %+v", calls)
	}
}

// TestMutedRowDoesNotBlockCatchAllForOtherAgents: a mute for agent A must
// not prevent agent B from receiving via catch-all on the same channel.
func TestMutedRowDoesNotBlockCatchAllForOtherAgents(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "telegram:*", "agent-a", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Subscribe(ctx, "telegram:*", "agent-b", false); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMuted(ctx, "telegram:alice", "agent-a", true); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("telegram:alice", "telegram", "alice", "", "", "hi", "m1", nil, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch did not finish")
	}
	calls := sender.getCalls()
	if len(calls) != 1 || calls[0].Name != "agent-b" {
		t.Fatalf("expected only agent-b, got %+v", calls)
	}
}

// TestDispatchAutomatedFeedsWithoutWakingAgents pins the notification-mail
// policy: machine-generated mail still lands in the channel feed and reaches
// the web UI, but no agent is prompted. Without this, every GitHub
// notification and newsletter wakes each subscriber and costs tokens on a
// message nobody can reply to.
func TestDispatchAutomatedFeedsWithoutWakingAgents(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	hub := &mockHub{}
	svc := NewService(store, sender, hub)
	ctx := context.Background()

	if err := store.Subscribe(ctx, "gmail:notificationsgithubcom", "fast-crane", false); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("gmail:notificationsgithubcom", "gmail",
		`[gmail] "coderabbitai[bot]" <notifications@github.com>`, "notifications@github.com", "",
		"Re: [rpuneet/mycel] approved this pull request", "m1", nil, nil, nil, Automated())

	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch did not finish")
	}

	if calls := sender.getCalls(); len(calls) != 0 {
		t.Fatalf("automated mail must not be delivered to agents, got %d: %+v", len(calls), calls)
	}

	// Ingested: the message is still readable in the channel feed.
	msgs, err := store.GetMessages(ctx, "gmail:notificationsgithubcom", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected automated mail to be stored in the feed, got %d messages", len(msgs))
	}

	// And the web UI is still told about it.
	if len(hub.events) != 1 || hub.events[0] != "gateway.message" {
		t.Errorf("expected one gateway.message publish, got %v", hub.events)
	}

	// No delivery attempt was made, so nothing should be logged as failed.
	entries, err := store.RecentActivity(ctx, "gmail:notificationsgithubcom", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no delivery-log entries for automated mail, got %+v", entries)
	}
}

// TestDispatchWithoutAutomatedStillDelivers is the control for the test
// above: the same channel and subscriber, minus the Automated option, must
// still wake the agent. Guards against the filter swallowing human mail.
func TestDispatchWithoutAutomatedStillDelivers(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "gmail:notificationsgithubcom", "fast-crane", false); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("gmail:notificationsgithubcom", "gmail", "[gmail] Puneet <puneet@example.com>",
		"puneet@example.com", "", "can you ship the release?", "m1", nil, nil, nil)

	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch did not finish")
	}

	calls := sender.getCalls()
	if len(calls) != 1 || calls[0].Name != "fast-crane" {
		t.Fatalf("expected one delivery to fast-crane, got %+v", calls)
	}
}

func TestDispatchOfflineAgentLogsSkipped(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{errFn: func(name string) error {
		return fmt.Errorf("agent %s is stopped", name)
	}}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "slack:eng", "offline-bot", false); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("slack:eng", "slack", "alice", "U1", "", "hello fleet", "m1", nil, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch did not finish")
	}

	entries, err := store.RecentActivity(ctx, "slack:eng", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 skipped delivery row, got %+v", entries)
	}
	if entries[0].Status != StatusSkipped {
		t.Errorf("status = %q, want %q", entries[0].Status, StatusSkipped)
	}
	if entries[0].Agent != "offline-bot" {
		t.Errorf("agent = %q, want offline-bot", entries[0].Agent)
	}
	if entries[0].Error == "" {
		t.Error("expected offline error text on skipped row")
	}
}

func TestDispatchSendErrorLogsFailedNotSkipped(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{errFn: func(string) error {
		return fmt.Errorf("tmux session not found")
	}}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "slack:eng", "eng-01", false); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("slack:eng", "slack", "alice", "U1", "", "hello", "m1", nil, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch did not finish")
	}

	entries, err := store.RecentActivity(ctx, "slack:eng", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Status != StatusFailed {
		t.Fatalf("expected 1 failed row, got %+v", entries)
	}
}

func TestMigratePlaceholderSubsNoOpWhenRealHasSubs(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "telegram:*", "legacy-agent", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Subscribe(ctx, "telegram:ab010300", "real-agent", false); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("telegram:ab010300", "telegram", "user", "", "", "hi", "", nil, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch timeout")
	}

	subs, err := store.Subscribers(ctx, "telegram:ab010300")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].Agent != "real-agent" {
		t.Fatalf("expected only real-agent, got %+v", subs)
	}
	// Subscriptions stay uncopied; delivery merges catch-all peers (#3688).
	calls := sender.getCalls()
	got := map[string]bool{}
	for _, c := range calls {
		got[c.Name] = true
	}
	if !got["real-agent"] || !got["legacy-agent"] || len(got) != 2 {
		t.Fatalf("expected real-agent + legacy-agent via catch-all merge, got %+v", calls)
	}
}

// TestRecordOutboundStoresAndPublishes covers the other half of the
// conversation. Channel history is built from notify_messages, and only inbound
// messages were ever written there, so a transcript showed the question and
// never the answer.
func TestRecordOutboundStoresAndPublishes(t *testing.T) {
	store := setupTestStore(t)
	hub := &mockHub{}
	sender := &mockSender{}
	svc := NewService(store, sender, hub)
	ctx := context.Background()

	svc.RecordOutbound("slack:general", "fast-crane", "merged both PRs, tracker is #3468")

	msgs, err := store.GetMessages(ctx, "slack:general", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected the outbound message to be stored, got %d", len(msgs))
	}
	if msgs[0].Sender != "fast-crane" {
		t.Errorf("stored sender = %q, want fast-crane", msgs[0].Sender)
	}
	if msgs[0].Content != "merged both PRs, tracker is #3468" {
		t.Errorf("stored content = %q", msgs[0].Content)
	}

	// An open channel view appends from channel.message, so the payload has to
	// carry a nested message object or the UI silently ignores the event.
	if len(hub.events) != 1 || hub.events[0] != "channel.message" {
		t.Fatalf("published events = %v, want [channel.message]", hub.events)
	}
	p := hub.payloads[0]
	if p["channel"] != "slack:general" {
		t.Errorf("payload channel = %v, want slack:general", p["channel"])
	}
	msg, ok := p["message"].(map[string]any)
	if !ok {
		t.Fatalf("payload has no nested message object: %+v", p)
	}
	if msg["sender"] != "fast-crane" || msg["content"] != "merged both PRs, tracker is #3468" {
		t.Errorf("published message = %+v", msg)
	}
	if msg["type"] != "text" {
		t.Errorf("published message type = %v, want text", msg["type"])
	}

	// Recording an outbound message must never prompt an agent: the message
	// came *from* one.
	if calls := sender.getCalls(); len(calls) != 0 {
		t.Errorf("recording an outbound message woke %d agent(s): %+v", len(calls), calls)
	}
}

// TestRecordOutboundIgnoresEmpty keeps blank rows out of the transcript.
func TestRecordOutboundIgnoresEmpty(t *testing.T) {
	store := setupTestStore(t)
	hub := &mockHub{}
	svc := NewService(store, &mockSender{}, hub)
	ctx := context.Background()

	svc.RecordOutbound("", "fast-crane", "no channel")
	svc.RecordOutbound("slack:general", "fast-crane", "")

	for _, ch := range []string{"", "slack:general"} {
		msgs, err := store.GetMessages(ctx, ch, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(msgs) != 0 {
			t.Errorf("channel %q stored %d message(s), want 0", ch, len(msgs))
		}
	}
	if len(hub.events) != 0 {
		t.Errorf("published %v for an empty message, want nothing", hub.events)
	}
}

// TestRecordOutboundThenInboundReadsAsConversation is the end-to-end shape the
// bug report asked for: history in order, both sides present.
func TestRecordOutboundThenInboundReadsAsConversation(t *testing.T) {
	store := setupTestStore(t)
	svc := NewService(store, &mockSender{}, &mockHub{})
	ctx := context.Background()

	svc.Dispatch("slack:general", "slack", "[slack] Puneet Rai", "U1", "", "??", "m1", nil, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch did not finish")
	}
	svc.RecordOutbound("slack:general", "fast-crane", "answered in Slack a minute ago")

	msgs, err := store.GetMessages(ctx, "slack:general", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected both sides of the exchange, got %d message(s)", len(msgs))
	}
	// GetMessages returns newest first.
	if msgs[0].Sender != "fast-crane" {
		t.Errorf("newest message sender = %q, want the agent's reply", msgs[0].Sender)
	}
	if msgs[1].Sender != "[slack] Puneet Rai" {
		t.Errorf("older message sender = %q, want the human's question", msgs[1].Sender)
	}
}

// TestSlackGeneralIsNotCatchAll (#3467): subscribing to real #general must not
// make that agent the fallback for every other Slack channel.
func TestSlackGeneralIsNotCatchAll(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "slack:*", "catch-agent", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Subscribe(ctx, "slack:general", "general-only", false); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("slack:eng", "slack", "alice", "", "", "hello eng", "m1", nil, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch timeout")
	}
	calls := sender.getCalls()
	if len(calls) != 1 || calls[0].Name != "catch-agent" {
		t.Fatalf("eng should only hit catch-all agent, got %+v", calls)
	}

	sender.mu.Lock()
	sender.calls = nil
	sender.mu.Unlock()

	svc.Dispatch("slack:general", "slack", "bob", "", "", "hello general", "m2", nil, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch timeout")
	}
	calls = sender.getCalls()
	got := map[string]bool{}
	for _, c := range calls {
		got[c.Name] = true
	}
	// Explicit #general sub keeps general-only; catch-all peer still receives (#3688).
	if !got["general-only"] || !got["catch-agent"] || len(got) != 2 {
		t.Fatalf("#general should hit general-only + catch-agent, got %+v", calls)
	}
}

// TestLegacyCatchAllStillDelivers: pre-migration "{platform}:general" rows
// still provide fallback until migrateLegacyCatchAll rewrites them.
func TestLegacyCatchAllStillDelivers(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "slack:general", "legacy-root", false); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("slack:eng", "slack", "alice", "", "", "via legacy", "m1", nil, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch timeout")
	}
	calls := sender.getCalls()
	if len(calls) != 1 || calls[0].Name != "legacy-root" {
		t.Fatalf("expected legacy catch-all delivery, got %+v", calls)
	}
}

// TestCatchAllStarDeliversToUnmatchedGeneral: with only slack:*, messages on
// real #general fall back to the catch-all when nobody subscribed to #general.
func TestCatchAllStarDeliversToUnmatchedGeneral(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "slack:*", "root", false); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("slack:general", "slack", "alice", "", "", "in general", "m1", nil, nil, nil)
	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch timeout")
	}
	calls := sender.getCalls()
	if len(calls) != 1 || calls[0].Name != "root" {
		t.Fatalf("expected catch-all delivery into #general, got %+v", calls)
	}
}
