package server

import "testing"

func TestIsSystemContainer(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"mycel-db", true},
		{"mycel-playwright", true},
		{"bc-db", true},
		{"bc-playwright", true},
		{"mycel-daemon", true},
		{"bc-daemon", true},
		{"mycel-13c6e9-zen-zebra", false},
		{"bc-13c6e9-zen-zebra", false},
		{"unrelated", false},
	}
	for _, c := range cases {
		if got := isSystemContainer(c.name); got != c.want {
			t.Errorf("isSystemContainer(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsAgentContainer(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"mycel-13c6e9-zen-zebra", true},
		{"mycel-abc123-bold-falcon", true},
		{"bc-13c6e9-zen-zebra", true}, // legacy prefix
		{"mycel-db", false},
		{"bc-db", false},
		{"mycel-daemon", false},
		{"random-container", false},
		{"mycel-", false}, // no agent name after prefix — treated as not an agent
	}
	for _, c := range cases {
		if got := isAgentContainer(c.name); got != c.want {
			t.Errorf("isAgentContainer(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestExtractAgentName(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"mycel-13c6e9-zen-zebra", "zen-zebra"},
		{"mycel-abc123-bold-falcon", "bold-falcon"},
		{"bc-13c6e9-zen-zebra", "zen-zebra"},           // legacy
		{"bc-abc123-noble-hyena", "noble-hyena"},       // legacy
		{"mycel-13c6e9-a", "a"},                        // single-char agent name
		{"unrelated-container", "unrelated-container"}, // no known prefix
	}
	for _, c := range cases {
		if got := extractAgentName(c.name); got != c.want {
			t.Errorf("extractAgentName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
