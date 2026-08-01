package gmailgw

import (
	"context"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

// newTestPlugin builds a plugin value for tests with the Google loopback flow
// wired, matching how init() registers it.
func newTestPlugin() *plugin {
	return &plugin{oauth: newGoogleFlow()}
}

// TestPluginImplementsOAuth locks the capability assertions the server relies
// on to dispatch the browser flow.
func TestPluginImplementsOAuth(t *testing.T) {
	var p any = newTestPlugin()
	if _, ok := p.(app.OAuthFlow); !ok {
		t.Error("gmail plugin does not implement app.OAuthFlow")
	}
	if _, ok := p.(app.OAuthConfigured); !ok {
		t.Error("gmail plugin does not implement app.OAuthConfigured")
	}
}

// TestOAuthConfiguredTracksEnv verifies one-click availability follows the
// server-side Google client env vars.
func TestOAuthConfiguredTracksEnv(t *testing.T) {
	p := newTestPlugin()

	t.Setenv(envGoogleClientID, "")
	t.Setenv(envGoogleClientSecret, "")
	if p.OAuthConfigured() {
		t.Error("OAuthConfigured() = true with no client creds; want false")
	}

	t.Setenv(envGoogleClientID, "client.apps.googleusercontent.com")
	t.Setenv(envGoogleClientSecret, "GOCSPX-secret")
	if !p.OAuthConfigured() {
		t.Error("OAuthConfigured() = false with client creds set; want true")
	}
}

// TestBeginAuthUnconfigured surfaces an actionable error (not a dead flow)
// when the server has no Google client.
func TestBeginAuthUnconfigured(t *testing.T) {
	t.Setenv(envGoogleClientID, "")
	t.Setenv(envGoogleClientSecret, "")
	p := newTestPlugin()
	_, err := p.BeginAuth(context.Background(), app.Instance{Name: "gmail"})
	if err == nil {
		t.Fatal("BeginAuth with no client creds returned nil error; want fallback message")
	}
}

// TestBeginAuthConfigured produces a Google consent URL carrying the loopback
// redirect and PKCE challenge.
func TestBeginAuthConfigured(t *testing.T) {
	t.Setenv(envGoogleClientID, "client.apps.googleusercontent.com")
	t.Setenv(envGoogleClientSecret, "GOCSPX-secret")
	p := newTestPlugin()
	sess, err := p.BeginAuth(context.Background(), app.Instance{Name: "gmail"})
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	defer p.oauth.PollAuth(context.Background(), app.AuthSession{ID: sess.ID}) //nolint:errcheck // drive teardown

	if sess.Kind != app.AuthKindCallback {
		t.Errorf("Kind = %q, want %q", sess.Kind, app.AuthKindCallback)
	}
	for _, want := range []string{
		"accounts.google.com",
		"code_challenge=",
		"code_challenge_method=S256",
		"redirect_uri=http%3A%2F%2F127.0.0.1%3A",
		"%2Foauth%2Fcallback",
		"access_type=offline",
		"prompt=consent",
		"gmail.readonly",
		"gmail.send",
	} {
		if !strings.Contains(sess.AuthURL, want) {
			t.Errorf("consent URL missing %q\nurl: %s", want, sess.AuthURL)
		}
	}
}
