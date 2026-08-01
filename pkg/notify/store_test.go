package notify

import (
	"context"
	"testing"
)

// TestSaveChannel_PreservesPlatformID asserts that an existing non-empty
// platform_id is not clobbered by a later SaveChannel call carrying a
// fallback (e.g., the channel name when the real platform-specific ID
// could not be extracted from the inbound payload).
func TestSaveChannel_PreservesPlatformID(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	const channel = "telegram:general"
	const platform = "telegram"
	const goodID = "123456789" // numeric chat_id
	const fallbackID = "general"

	// 1. First write: real platform_id stored.
	if err := store.SaveChannel(ctx, channel, platform, goodID); err != nil {
		t.Fatalf("SaveChannel (initial): %v", err)
	}

	// 2. Second write with a fallback platform_id. Must NOT overwrite.
	if err := store.SaveChannel(ctx, channel, platform, fallbackID); err != nil {
		t.Fatalf("SaveChannel (fallback): %v", err)
	}

	channels, err := store.LoadChannels(ctx)
	if err != nil {
		t.Fatalf("LoadChannels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	if got := channels[0].PlatformID; got != goodID {
		t.Fatalf("platform_id was clobbered: got %q, want %q", got, goodID)
	}
}

// TestSaveChannel_FillsEmptyPlatformID asserts that if the existing row has
// an empty platform_id, the next write does populate it.
func TestSaveChannel_FillsEmptyPlatformID(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	const channel = "slack:general"
	const platform = "slack"

	// 1. First write: empty platform_id.
	if err := store.SaveChannel(ctx, channel, platform, ""); err != nil {
		t.Fatalf("SaveChannel (empty): %v", err)
	}

	// 2. Second write with a real ID. Must populate the previously-empty value.
	if err := store.SaveChannel(ctx, channel, platform, "C0123ABC"); err != nil {
		t.Fatalf("SaveChannel (real): %v", err)
	}

	channels, err := store.LoadChannels(ctx)
	if err != nil {
		t.Fatalf("LoadChannels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	if got := channels[0].PlatformID; got != "C0123ABC" {
		t.Fatalf("platform_id not filled: got %q, want %q", got, "C0123ABC")
	}
}

// TestChannelStats exercises the /api/stats/channels aggregation: message
// counts, member counts, top senders, ordering, and the subscription-only
// channel case.
func TestChannelStats(t *testing.T) {
	type msg struct{ channel, sender string }
	type sub struct{ channel, agent string }

	tests := []struct {
		name string
		msgs []msg
		subs []sub
		want []ChannelStat
	}{
		{
			name: "empty store",
			want: []ChannelStat{},
		},
		{
			name: "messages only, sorted by count desc then name",
			msgs: []msg{
				{"slack:eng", "alice"},
				{"slack:eng", "alice"},
				{"slack:eng", "bob"},
				{"telegram:ops", "carol"},
			},
			want: []ChannelStat{
				{
					Name:         "slack:eng",
					MessageCount: 3,
					TopSenders:   []TopSender{{Sender: "alice", Count: 2}, {Sender: "bob", Count: 1}},
				},
				{
					Name:         "telegram:ops",
					MessageCount: 1,
					TopSenders:   []TopSender{{Sender: "carol", Count: 1}},
				},
			},
		},
		{
			name: "subscription-only channel appears with zero messages",
			msgs: []msg{{"slack:eng", "alice"}},
			subs: []sub{
				{"slack:eng", "eng-01"},
				{"slack:eng", "eng-02"},
				{"discord:quiet", "ops-01"},
			},
			want: []ChannelStat{
				{
					Name:         "slack:eng",
					MessageCount: 1,
					MemberCount:  2,
					TopSenders:   []TopSender{{Sender: "alice", Count: 1}},
				},
				{
					Name:        "discord:quiet",
					MemberCount: 1,
				},
			},
		},
		{
			name: "top senders capped at five",
			msgs: []msg{
				{"slack:eng", "s1"}, {"slack:eng", "s1"}, {"slack:eng", "s1"},
				{"slack:eng", "s2"}, {"slack:eng", "s2"},
				{"slack:eng", "s3"}, {"slack:eng", "s4"},
				{"slack:eng", "s5"}, {"slack:eng", "s6"},
			},
			want: []ChannelStat{
				{
					Name:         "slack:eng",
					MessageCount: 9,
					TopSenders: []TopSender{
						{Sender: "s1", Count: 3}, {Sender: "s2", Count: 2},
						{Sender: "s3", Count: 1}, {Sender: "s4", Count: 1},
						{Sender: "s5", Count: 1},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupTestStore(t)
			ctx := context.Background()
			for _, m := range tt.msgs {
				if err := store.SaveMessage(ctx, m.channel, m.sender, "", "hi"); err != nil {
					t.Fatalf("SaveMessage: %v", err)
				}
			}
			for _, s := range tt.subs {
				if err := store.Subscribe(ctx, s.channel, s.agent, false); err != nil {
					t.Fatalf("Subscribe: %v", err)
				}
			}

			got, err := store.ChannelStats(ctx)
			if err != nil {
				t.Fatalf("ChannelStats: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d channels, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				g := got[i]
				if g.Name != w.Name || g.MessageCount != w.MessageCount || g.MemberCount != w.MemberCount {
					t.Errorf("channel[%d] = {%s %d msgs %d members}, want {%s %d msgs %d members}",
						i, g.Name, g.MessageCount, g.MemberCount, w.Name, w.MessageCount, w.MemberCount)
				}
				if len(g.TopSenders) != len(w.TopSenders) {
					t.Errorf("channel[%d] top_senders = %+v, want %+v", i, g.TopSenders, w.TopSenders)
					continue
				}
				for j, h := range w.TopSenders {
					if g.TopSenders[j] != h {
						t.Errorf("channel[%d] top_senders[%d] = %+v, want %+v", i, j, g.TopSenders[j], h)
					}
				}
				if g.MessageCount > 0 && g.LastActivity.IsZero() {
					t.Errorf("channel[%d] %s: last_activity is zero despite %d messages", i, g.Name, g.MessageCount)
				}
			}
		})
	}
}

// TestUpsertChannelMeta_RoundTrip asserts that resolved display metadata is
// stored, survives empty re-writes, and can be updated with new values.
func TestUpsertChannelMeta_RoundTrip(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	const channel = "whatsapp:family-group"
	if err := store.SaveChannel(ctx, channel, "whatsapp", "1234@g.us"); err != nil {
		t.Fatalf("SaveChannel: %v", err)
	}
	if err := store.UpsertChannelMeta(ctx, channel, "Family Group", "group", "", 12); err != nil {
		t.Fatalf("UpsertChannelMeta: %v", err)
	}

	channels, err := store.LoadChannels(ctx)
	if err != nil {
		t.Fatalf("LoadChannels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	got := channels[0]
	if got.DisplayName != "Family Group" || got.Kind != "group" || got.ParticipantCount != 12 {
		t.Fatalf("meta round-trip failed: %+v", got)
	}
	if got.PlatformID != "1234@g.us" {
		t.Fatalf("platform_id clobbered by meta upsert: %q", got.PlatformID)
	}

	// Empty values must not clobber previously-resolved metadata.
	if upErr := store.UpsertChannelMeta(ctx, channel, "", "", "", 0); upErr != nil {
		t.Fatalf("UpsertChannelMeta (empty): %v", upErr)
	}
	channels, err = store.LoadChannels(ctx)
	if err != nil {
		t.Fatalf("LoadChannels: %v", err)
	}
	if got := channels[0]; got.DisplayName != "Family Group" || got.Kind != "group" || got.ParticipantCount != 12 {
		t.Fatalf("empty upsert clobbered meta: %+v", got)
	}

	// New values replace old ones.
	if upErr := store.UpsertChannelMeta(ctx, channel, "Renamed Group", "group", "", 13); upErr != nil {
		t.Fatalf("UpsertChannelMeta (update): %v", upErr)
	}
	channels, err = store.LoadChannels(ctx)
	if err != nil {
		t.Fatalf("LoadChannels: %v", err)
	}
	if got := channels[0]; got.DisplayName != "Renamed Group" || got.ParticipantCount != 13 {
		t.Fatalf("meta update failed: %+v", got)
	}
}

// TestUpsertChannelMeta_InsertsMissingRow asserts that meta for a channel
// with no prior mapping creates the row, deriving platform from the prefix.
func TestUpsertChannelMeta_InsertsMissingRow(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	if err := store.UpsertChannelMeta(ctx, "slack:general", "general", "channel", "", 0); err != nil {
		t.Fatalf("UpsertChannelMeta: %v", err)
	}
	channels, err := store.LoadChannels(ctx)
	if err != nil {
		t.Fatalf("LoadChannels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	got := channels[0]
	if got.Platform != "slack" || got.DisplayName != "general" || got.Kind != "channel" {
		t.Fatalf("inserted row wrong: %+v", got)
	}

	// A later SaveChannel fills the platform_id without touching meta.
	if saveErr := store.SaveChannel(ctx, "slack:general", "slack", "C0123ABC"); saveErr != nil {
		t.Fatalf("SaveChannel: %v", saveErr)
	}
	channels, err = store.LoadChannels(ctx)
	if err != nil {
		t.Fatalf("LoadChannels: %v", err)
	}
	got = channels[0]
	if got.PlatformID != "C0123ABC" || got.DisplayName != "general" || got.Kind != "channel" {
		t.Fatalf("SaveChannel disturbed meta: %+v", got)
	}
}

// TestUpsertChannelMeta_PreservesMessages asserts that storing metadata
// never disturbs the channel's stored messages or subscriptions.
func TestUpsertChannelMeta_PreservesMessages(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	const channel = "whatsapp:1234"
	for _, msg := range []string{"hello", "world"} {
		if err := store.SaveMessage(ctx, channel, "alice", "", msg); err != nil {
			t.Fatalf("SaveMessage: %v", err)
		}
	}
	if err := store.Subscribe(ctx, channel, "eng-01", false); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := store.UpsertChannelMeta(ctx, channel, "Alice", "person", "", 0); err != nil {
		t.Fatalf("UpsertChannelMeta: %v", err)
	}

	msgs, err := store.GetMessages(ctx, channel, 10, 0)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after meta upsert, got %d", len(msgs))
	}
	subs, err := store.Subscribers(ctx, channel)
	if err != nil {
		t.Fatalf("Subscribers: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription after meta upsert, got %d", len(subs))
	}
}
