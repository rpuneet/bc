package slackgw

import "testing"

func TestIconEmojiForSender(t *testing.T) {
	cases := []struct {
		sender string
		want   string
	}{
		{"zen-zebra", ":zebra_face:"},
		{"lucid-meerkat", ":otter:"},
		{"noble-hyena", ":dog2:"},
		{"noble-kestrel", ":bird:"},
		{"fierce-capybara", ":hamster:"},
		{"easy-koala", ":koala:"},
		// Unknown species falls back to the neutral robot icon.
		{"weird-quokka", ":robot_face:"},
		// Empty / whitespace sender still resolves to fallback.
		{"", ":robot_face:"},
		{"   ", ":robot_face:"},
		// A bare token (no hyphen) works too.
		{"zebra", ":zebra_face:"},
		// Case-insensitive.
		{"Zen-ZEBRA", ":zebra_face:"},
	}
	for _, c := range cases {
		got := iconEmojiForSender(c.sender)
		if got != c.want {
			t.Errorf("iconEmojiForSender(%q) = %q, want %q", c.sender, got, c.want)
		}
	}
}
