package slackgw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/slack-go/slack"
)

// newTestAdapter wires an Adapter to a fake Slack API served by srv so we can
// inspect the outbound chat.postMessage form without talking to Slack.
func newTestAdapter(t *testing.T, srvURL string) *Adapter {
	t.Helper()
	a := New("xoxb-test", "xapp-test")
	a.api = slack.New("xoxb-test",
		slack.OptionAPIURL(srvURL+"/"),
		slack.OptionOnResponseHeaders(a.observeScopes),
	)
	return a
}

// TestSendAttributesAgentUsername verifies the outbound payload carries the
// sending agent's name as the Slack message username (agent attribution) and a
// deterministic per-agent icon.
func TestSendAttributesAgentUsername(t *testing.T) {
	const agent = "zen-zebra"

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		got = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"channel":"C1","ts":"1.2"}`))
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv.URL)
	if err := a.Send(context.Background(), "C1", agent, "hello world"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if u := got.Get("username"); u != agent {
		t.Errorf("username = %q, want %q", u, agent)
	}
	if got.Get("text") != "hello world" {
		t.Errorf("text = %q, want %q", got.Get("text"), "hello world")
	}
	if want := agentIconEmoji(agent); got.Get("icon_emoji") != want {
		t.Errorf("icon_emoji = %q, want %q", got.Get("icon_emoji"), want)
	}
}

func TestAgentIconEmojiMushroom(t *testing.T) {
	// Every agent posts under the mycel mushroom, regardless of name.
	for _, name := range []string{"zen-zebra", "lucid-meerkat", ""} {
		if got := agentIconEmoji(name); got != ":mushroom:" {
			t.Errorf("agentIconEmoji(%q) = %q, want :mushroom:", name, got)
		}
	}
}

func TestObserveScopes(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		wantSeen   bool
		wantCustom bool
	}{
		{"missing header", "", false, false},
		{"has customize", "chat:write, chat:write.customize, channels:read", true, true},
		{"lacks customize", "chat:write, channels:read", true, false},
		{"spacing tolerated", "chat:write.customize", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := New("xoxb-test", "xapp-test")
			h := http.Header{}
			if tc.header != "" {
				h.Set("X-OAuth-Scopes", tc.header)
			}
			a.observeScopes("auth.test", h)
			if a.scopesSeen.Load() != tc.wantSeen {
				t.Errorf("scopesSeen = %v, want %v", a.scopesSeen.Load(), tc.wantSeen)
			}
			if a.customizeScope.Load() != tc.wantCustom {
				t.Errorf("customizeScope = %v, want %v", a.customizeScope.Load(), tc.wantCustom)
			}
		})
	}
}
