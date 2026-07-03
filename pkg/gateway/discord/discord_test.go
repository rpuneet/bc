package discord

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "already a slug", in: "python", want: "python"},
		{name: "spaces to dashes", in: "Blancode Coder Community", want: "blancode-coder-community"},
		{name: "colon-mangled guild", in: "blancode:-coder-community", want: "blancode-coder-community"},
		{name: "mixed colons and spaces", in: "Guild: Name Here", want: "guild-name-here"},
		{name: "collapses repeated separators", in: "a -- b::c  d", want: "a-b-c-d"},
		{name: "trims leading and trailing separators", in: " --general-- ", want: "general"},
		{name: "uppercase lowered", in: "GENERAL", want: "general"},
		{name: "underscore preserved", in: "dev_ops", want: "dev_ops"},
		{name: "punctuation dropped", in: "rock & roll!", want: "rock-roll"},
		{name: "digits preserved", in: "team 42", want: "team-42"},
		{name: "numeric snowflake id", in: "123456789012345678", want: "123456789012345678"},
		{name: "empty", in: "", want: ""},
		{name: "only separators", in: ":-: ", want: ""},
		{name: "non-ascii dropped", in: "日本語", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := slugify(tt.in); got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestChannelKey(t *testing.T) {
	tests := []struct {
		name    string
		guild   string
		channel string
		want    string
	}{
		{name: "guild and channel", guild: "Blancode Coder Community", channel: "python", want: "blancode-coder-community:python"},
		{name: "guild with colon", guild: "Blancode: Coder Community", channel: "python", want: "blancode-coder-community:python"},
		{name: "snowflake guild fallback", guild: "123456789012345678", channel: "general", want: "123456789012345678:general"},
		{name: "channel name with spaces", guild: "My Server", channel: "General Chat", want: "my-server:general-chat"},
		{name: "no guild yields bare channel", guild: "", channel: "python", want: "python"},
		{name: "empty channel yields empty key", guild: "My Server", channel: "", want: ""},
		{name: "unresolvable channel yields empty key", guild: "My Server", channel: "::", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := channelKey(tt.guild, tt.channel); got != tt.want {
				t.Errorf("channelKey(%q, %q) = %q, want %q", tt.guild, tt.channel, got, tt.want)
			}
		})
	}
}
