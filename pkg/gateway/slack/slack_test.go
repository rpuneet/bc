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
// sending agent's name as the Slack message username (agent attribution). With
// no public avatar base configured, no icon is set — and never a hardcoded
// emoji.
func TestSendAttributesAgentUsername(t *testing.T) {
	const agent = "zen-zebra"
	t.Setenv("MYCEL_AVATAR_PUBLIC_BASE", "")

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
	// No hardcoded emoji icon — attribution is username-only until a public
	// avatar base is configured.
	if e := got.Get("icon_emoji"); e != "" {
		t.Errorf("icon_emoji = %q, want empty (no hardcoded emoji)", e)
	}
	if u := got.Get("icon_url"); u != "" {
		t.Errorf("icon_url = %q, want empty when no public avatar base", u)
	}
}

// TestSendUsesAvatarIconURL verifies that when a public avatar base is
// configured, Send attaches the agent's real AgentCharacter avatar URL as
// icon_url — never a hardcoded emoji.
func TestSendUsesAvatarIconURL(t *testing.T) {
	const agent = "zen-zebra"
	t.Setenv("MYCEL_AVATAR_PUBLIC_BASE", "https://bc-infra.com/avatars")

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
	if err := a.Send(context.Background(), "C1", agent, "hi"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if want := "https://bc-infra.com/avatars/zen-zebra.png"; got.Get("icon_url") != want {
		t.Errorf("icon_url = %q, want %q", got.Get("icon_url"), want)
	}
	if e := got.Get("icon_emoji"); e != "" {
		t.Errorf("icon_emoji = %q, want empty", e)
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
