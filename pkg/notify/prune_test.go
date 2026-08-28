package notify

import "testing"

func TestFindPruneCandidates(t *testing.T) {
	subs := []Subscription{
		{Channel: "gmail:*", Agent: "fast-crane", MentionOnly: false},
		{Channel: "gmail:alertsbank", Agent: "fast-crane", MentionOnly: false},
		{Channel: "gmail:newslettereconomictimes", Agent: "fast-crane", MentionOnly: false},
		// deliberate: different mention_only than catch-all
		{Channel: "gmail:focused", Agent: "fast-crane", MentionOnly: true},
		// no catch-all for whatsapp — not a candidate
		{Channel: "whatsapp:alice", Agent: "fast-crane", MentionOnly: false},
		{Channel: "telegram:*", Agent: "broad", MentionOnly: false},
		{Channel: "telegram:dm-bob", Agent: "broad", MentionOnly: false},
		// mute marker must not be pruned
		{Channel: "telegram:noisy", Agent: "broad", MentionOnly: false, Muted: true},
		// other agent on same channel without matching catch-all
		{Channel: "telegram:dm-bob", Agent: "other", MentionOnly: false},
	}

	got := FindPruneCandidates(subs)
	want := map[string]bool{
		"gmail:alertsbank|fast-crane":              true,
		"gmail:newslettereconomictimes|fast-crane": true,
		"telegram:dm-bob|broad":                    true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d candidates, want %d: %+v", len(got), len(want), got)
	}
	for _, c := range got {
		key := c.Channel + "|" + c.Agent
		if !want[key] {
			t.Errorf("unexpected candidate %s", key)
		}
		if c.Muted {
			t.Errorf("muted row returned as candidate: %+v", c)
		}
	}
}

func TestFindPruneCandidates_LegacyCatchAll(t *testing.T) {
	subs := []Subscription{
		{Channel: "gmail:general", Agent: "fast-crane", MentionOnly: false},
		{Channel: "gmail:alertsbank", Agent: "fast-crane", MentionOnly: false},
	}
	got := FindPruneCandidates(subs)
	if len(got) != 1 || got[0].Channel != "gmail:alertsbank" {
		t.Fatalf("legacy catch-all should still seed prune heuristic, got %+v", got)
	}
}

func TestFindPruneCandidates_NoCatchAll(t *testing.T) {
	subs := []Subscription{
		{Channel: "gmail:only-explicit", Agent: "a", MentionOnly: false},
	}
	if got := FindPruneCandidates(subs); len(got) != 0 {
		t.Fatalf("expected no candidates without catch-all, got %+v", got)
	}
}

func TestFindPruneCandidates_SkipsCatchAllItself(t *testing.T) {
	subs := []Subscription{
		{Channel: "slack:*", Agent: "root", MentionOnly: false},
		{Channel: "slack:general", Agent: "root", MentionOnly: false}, // real #general room
	}
	// Real Slack #general must not be treated as a catch-all copy leftover.
	if got := FindPruneCandidates(subs); len(got) != 0 {
		t.Fatalf("slack:general is a real room, not a prune candidate, got %+v", got)
	}
}

func TestFilterPruneByPlatform(t *testing.T) {
	cands := []Subscription{
		{Channel: "gmail:a", Agent: "x"},
		{Channel: "whatsapp:b", Agent: "x"},
	}
	got := FilterPruneByPlatform(cands, "gmail")
	if len(got) != 1 || got[0].Channel != "gmail:a" {
		t.Fatalf("got %+v", got)
	}
	if all := FilterPruneByPlatform(cands, ""); len(all) != 2 {
		t.Fatalf("empty platform should keep all, got %d", len(all))
	}
}

func TestIsCatchAll(t *testing.T) {
	cases := map[string]bool{
		"gmail:*":         true,
		"slack:*":         true,
		"gmail:general":   false, // legacy — use IsLegacyCatchAll
		"slack:general":   false,
		"slack:eng":       false,
		"general":         false,
		"":                false,
		"gmail:generalx":  false,
		"foo:bar:general": false,
	}
	for ch, want := range cases {
		if got := IsCatchAll(ch); got != want {
			t.Errorf("IsCatchAll(%q)=%v want %v", ch, got, want)
		}
	}
	// Named-room #general is never legacy catch-all (#3730).
	for _, ch := range []string{
		"slack:general",
		"mattermost:general",
		"irc:general",
		"matrix:general",
		"discord:my-server:general",
	} {
		if IsLegacyCatchAll(ch) || IsAnyCatchAll(ch) {
			t.Fatalf("%s must not be legacy/any catch-all", ch)
		}
	}
	if !IsLegacyCatchAll("gmail:general") || !IsLegacyCatchAll("telegram:general") || IsLegacyCatchAll("slack:*") {
		t.Fatal("IsLegacyCatchAll mismatch for synthetic placeholders")
	}
	if !IsAnyCatchAll("slack:*") || !IsAnyCatchAll("gmail:general") {
		t.Fatal("IsAnyCatchAll mismatch")
	}
}

func TestFindPruneCandidates_NamedRoomGeneral(t *testing.T) {
	for _, pair := range []struct{ star, general string }{
		{"slack:*", "slack:general"},
		{"mattermost:*", "mattermost:general"},
		{"discord:my-server:*", "discord:my-server:general"},
	} {
		subs := []Subscription{
			{Channel: pair.star, Agent: "root", MentionOnly: false},
			{Channel: pair.general, Agent: "root", MentionOnly: false},
		}
		if got := FindPruneCandidates(subs); len(got) != 0 {
			t.Fatalf("%s must not be prune candidate under %s, got %+v", pair.general, pair.star, got)
		}
	}
}

func TestPlatformOfLabeledInstance(t *testing.T) {
	if got := PlatformOf("github:mycel:issue-1"); got != "github:mycel" {
		t.Fatalf("PlatformOf labeled = %q, want github:mycel", got)
	}
	if got := PlatformOf("slack:eng"); got != "slack" {
		t.Fatalf("PlatformOf slack = %q, want slack", got)
	}
	if !IsCatchAll("github:mycel:*") {
		t.Fatal("want IsCatchAll(github:mycel:*)")
	}
	if CatchAllChannel("github:mycel") != "github:mycel:*" {
		t.Fatal("CatchAllChannel labeled mismatch")
	}
}
