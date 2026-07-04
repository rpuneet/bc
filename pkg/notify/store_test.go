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

	const bcChannel = "telegram:general"
	const platform = "telegram"
	const goodID = "123456789" // numeric chat_id
	const fallbackID = "general"

	// 1. First write: real platform_id stored.
	if err := store.SaveChannel(ctx, bcChannel, platform, goodID); err != nil {
		t.Fatalf("SaveChannel (initial): %v", err)
	}

	// 2. Second write with a fallback platform_id. Must NOT overwrite.
	if err := store.SaveChannel(ctx, bcChannel, platform, fallbackID); err != nil {
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

	const bcChannel = "slack:general"
	const platform = "slack"

	// 1. First write: empty platform_id.
	if err := store.SaveChannel(ctx, bcChannel, platform, ""); err != nil {
		t.Fatalf("SaveChannel (empty): %v", err)
	}

	// 2. Second write with a real ID. Must populate the previously-empty value.
	if err := store.SaveChannel(ctx, bcChannel, platform, "C0123ABC"); err != nil {
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
				if err := store.SaveMessage(ctx, m.channel, m.sender, "hi"); err != nil {
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
				for j, ws := range w.TopSenders {
					if g.TopSenders[j] != ws {
						t.Errorf("channel[%d] top_senders[%d] = %+v, want %+v", i, j, g.TopSenders[j], ws)
					}
				}
				if g.MessageCount > 0 && g.LastActivity.IsZero() {
					t.Errorf("channel[%d] %s: last_activity is zero despite %d messages", i, g.Name, g.MessageCount)
				}
			}
		})
	}
}
