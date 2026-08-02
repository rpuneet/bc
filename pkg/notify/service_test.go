package notify

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// mockSender records SendToAgent calls.
type mockSender struct {
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
	events []string
	mu     sync.Mutex
}

func (m *mockHub) Publish(eventType string, _ map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, eventType)
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

func TestMigratePlaceholderSubsOnDispatch(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	// Legacy wizard subscription on telegram:general only.
	if err := store.Subscribe(ctx, "telegram:general", "mdrndr-manager", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Subscribe(ctx, "telegram:general", "mdrndr-tui", false); err != nil {
		t.Fatal(err)
	}

	svc.Dispatch("telegram:ab010300", "telegram", "[telegram] Agni", "", "", "ping", "", nil, nil, nil)

	if !svc.DrainDispatches(2 * time.Second) {
		t.Fatal("dispatch did not finish")
	}

	subs, err := store.Subscribers(ctx, "telegram:ab010300")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 migrated subscribers, got %d", len(subs))
	}
	// Placeholder remains (copy, not rewrite).
	legacy, err := store.Subscribers(ctx, "telegram:general")
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy) != 2 {
		t.Fatalf("expected placeholder to remain, got %d", len(legacy))
	}

	calls := sender.getCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 deliveries, got %d: %+v", len(calls), calls)
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

func TestMigratePlaceholderSubsNoOpWhenRealHasSubs(t *testing.T) {
	store := setupTestStore(t)
	sender := &mockSender{}
	svc := NewService(store, sender, &mockHub{})
	ctx := context.Background()

	if err := store.Subscribe(ctx, "telegram:general", "legacy-agent", false); err != nil {
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
	calls := sender.getCalls()
	if len(calls) != 1 || calls[0].Name != "real-agent" {
		t.Fatalf("expected delivery only to real-agent, got %+v", calls)
	}
}
