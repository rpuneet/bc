package notify

import (
	"context"
	"reflect"
	"testing"
)

func TestBuildDiscordChannelMapping(t *testing.T) {
	const canon = "discord:blancode-coder-community:python"
	tests := []struct {
		want map[string]string
		name string
		keys []string
	}{
		{
			name: "canonical keys untouched",
			keys: []string{canon, "discord:my-server:general"},
			want: map[string]string{},
		},
		{
			name: "colon-mangled guild normalizes deterministically",
			keys: []string{"discord:blancode:-coder-community:python"},
			want: map[string]string{"discord:blancode:-coder-community:python": canon},
		},
		{
			name: "bare channel merges into existing canonical key",
			keys: []string{"discord:python", canon},
			want: map[string]string{"discord:python": canon},
		},
		{
			name: "concatenated guild+channel merges into canonical key",
			keys: []string{"discord:blancode-coder-communitypython", canon},
			want: map[string]string{"discord:blancode-coder-communitypython": canon},
		},
		{
			name: "dashed concatenation merges into canonical key",
			keys: []string{"discord:blancode-coder-community-python", canon},
			want: map[string]string{"discord:blancode-coder-community-python": canon},
		},
		{
			name: "all three legacy generations merge via mangled-derived canonical",
			keys: []string{
				"discord:python",
				"discord:blancode-coder-communitypython",
				"discord:blancode:-coder-community:python",
			},
			want: map[string]string{
				"discord:python":                           canon,
				"discord:blancode-coder-communitypython":   canon,
				"discord:blancode:-coder-community:python": canon,
			},
		},
		{
			name: "ambiguous bare key is left untouched",
			keys: []string{"discord:python", "discord:guild-one:python", "discord:guild-two:python"},
			want: map[string]string{},
		},
		{
			name: "bare key without canonical match is left untouched",
			keys: []string{"discord:python"},
			want: map[string]string{},
		},
		{
			name: "two-segment key with unslugged parts is normalized",
			keys: []string{"discord:My Server:general"},
			want: map[string]string{"discord:My Server:general": "discord:my-server:general"},
		},
		{
			name: "mangled key with empty channel segment is left untouched",
			keys: []string{"discord:guild:"},
			want: map[string]string{},
		},
		{
			name: "no discord keys",
			keys: nil,
			want: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDiscordChannelMapping(tt.keys)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildDiscordChannelMapping(%v) = %v, want %v", tt.keys, got, tt.want)
			}
		})
	}
}

func TestIsCanonicalDiscordKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"discord:blancode-coder-community:python", true},
		{"discord:guild:chan", true},
		{"discord:python", false},                           // bare, no guild segment
		{"discord:blancode:-coder-community:python", false}, // extra segments
		{"discord:My Server:general", false},                // not slugified
		{"discord:guild:", false},                           // empty channel
		{"discord::chan", false},                            // empty guild
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := isCanonicalDiscordKey(tt.key); got != tt.want {
				t.Errorf("isCanonicalDiscordKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

// TestNormalizeDiscordChannels_MergesDuplicates seeds all three legacy key
// generations across the four notify tables and asserts that reopening the
// store folds them into the canonical key, keeping the newest subscription
// per (channel, agent).
func TestNormalizeDiscordChannels_MergesDuplicates(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	const (
		bare    = "discord:python"
		concat  = "discord:blancode-coder-communitypython"
		mangled = "discord:blancode:-coder-community:python"
		canon   = "discord:blancode-coder-community:python"
	)

	// Subscriptions: agent-a subscribed under two legacy keys — the mangled
	// row is newer (higher id) and carries mention_only=true, so it must win.
	if err := store.Subscribe(ctx, bare, "agent-a", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Subscribe(ctx, mangled, "agent-a", true); err != nil {
		t.Fatal(err)
	}
	if err := store.Subscribe(ctx, concat, "agent-b", false); err != nil {
		t.Fatal(err)
	}
	// Unrelated subscription must survive untouched.
	if err := store.Subscribe(ctx, "slack:general", "agent-a", false); err != nil {
		t.Fatal(err)
	}

	// Messages under each legacy key.
	for _, ch := range []string{bare, concat, mangled} {
		if err := store.SaveMessage(ctx, ch, "user", "hello from "+ch); err != nil {
			t.Fatal(err)
		}
	}

	// Delivery log entries under each legacy key.
	for _, ch := range []string{bare, concat, mangled} {
		if err := store.LogDelivery(ctx, DeliveryEntry{Channel: ch, Agent: "agent-a", Status: "delivered"}); err != nil {
			t.Fatal(err)
		}
	}

	// Channel mappings: canonical row exists with the real platform_id; the
	// legacy rows must be dropped in its favor.
	if err := store.SaveChannel(ctx, canon, "discord", "111222333"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveChannel(ctx, bare, "discord", "999"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveChannel(ctx, mangled, "discord", "888"); err != nil {
		t.Fatal(err)
	}

	// Reopen the store: normalization runs against the shared DB.
	migrated, err := OpenStore("/tmp/test-workspace")
	if err != nil {
		t.Fatalf("OpenStore (migration): %v", err)
	}

	// Subscriptions: agent-a keeps the newest row (mention_only=true),
	// agent-b's concatenated row folds in, slack row untouched.
	subs, err := migrated.AllSubscriptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 3 {
		t.Fatalf("expected 3 subscriptions after merge, got %d: %+v", len(subs), subs)
	}
	byAgent := make(map[string]Subscription)
	for _, sub := range subs {
		if sub.Channel == "slack:general" {
			continue
		}
		if sub.Channel != canon {
			t.Errorf("subscription channel = %q, want %q", sub.Channel, canon)
		}
		byAgent[sub.Agent] = sub
	}
	if !byAgent["agent-a"].MentionOnly {
		t.Error("agent-a mention_only = false, want true (newest row must win)")
	}
	if _, ok := byAgent["agent-b"]; !ok {
		t.Error("agent-b subscription missing after merge")
	}

	// Messages: all three legacy messages now live under the canonical key.
	msgs, err := migrated.GetMessages(ctx, canon, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages under %q, got %d", canon, len(msgs))
	}

	// Delivery log: all entries under the canonical key.
	entries, err := migrated.RecentActivity(ctx, canon, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 delivery entries under %q, got %d", canon, len(entries))
	}

	// Channel mappings: a single canonical row, real platform_id preserved.
	channels, err := migrated.LoadChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel mapping, got %d: %+v", len(channels), channels)
	}
	if channels[0].BCChannel != canon || channels[0].PlatformID != "111222333" {
		t.Errorf("channel mapping = %+v, want {%s discord 111222333}", channels[0], canon)
	}

	// Idempotency: reopening again must not change anything.
	again, err := OpenStore("/tmp/test-workspace")
	if err != nil {
		t.Fatalf("OpenStore (idempotency): %v", err)
	}
	subs2, err := again.AllSubscriptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(subs, subs2) {
		t.Errorf("normalization not idempotent:\nfirst:  %+v\nsecond: %+v", subs, subs2)
	}
}

// TestNormalizeDiscordChannels_RenamesLegacyOnlyMapping asserts that a legacy
// notify_channels row is renamed (not dropped) when no canonical row exists.
func TestNormalizeDiscordChannels_RenamesLegacyOnlyMapping(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	const (
		mangled = "discord:my:-server:general"
		canon   = "discord:my-server:general"
	)
	if err := store.SaveChannel(ctx, mangled, "discord", "42"); err != nil {
		t.Fatal(err)
	}

	migrated, err := OpenStore("/tmp/test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	channels, err := migrated.LoadChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel mapping, got %d", len(channels))
	}
	if channels[0].BCChannel != canon || channels[0].PlatformID != "42" {
		t.Errorf("channel mapping = %+v, want {%s discord 42}", channels[0], canon)
	}
}
