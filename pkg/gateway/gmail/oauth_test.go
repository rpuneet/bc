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
// on to dispatch the browser flow. Gmail does not implement OAuthConfigured —
// Sign in is always offered alongside Advanced manual paste.
func TestPluginImplementsOAuth(t *testing.T) {
	var p any = newTestPlugin()
	if _, ok := p.(app.OAuthFlow); !ok {
		t.Error("gmail plugin does not implement app.OAuthFlow")
	}
	if _, ok := p.(app.OAuthConfigured); ok {
		t.Error("gmail plugin must not implement OAuthConfigured (always offer Sign in + Advanced)")
	}
}

// TestGoogleConfiguredTracksEnv verifies server-side zero-paste availability
// follows the Google client env vars (used by resolveGoogleClientCreds).
func TestGoogleConfiguredTracksEnv(t *testing.T) {
	t.Setenv(envGoogleClientID, "")
	t.Setenv(envGoogleClientSecret, "")
	if googleConfigured() {
		t.Error("googleConfigured() = true with no client creds; want false")
	}

	t.Setenv(envGoogleClientID, "client.apps.googleusercontent.com")
	t.Setenv(envGoogleClientSecret, "GOCSPX-secret")
	if !googleConfigured() {
		t.Error("googleConfigured() = false with client creds set; want true")
	}
}

// TestGoogleConfiguredTracksBuildDefault verifies resolveGoogleClientCreds
// also follows the ldflags-injected default* vars (simulated here by setting
// them directly, since the real injection happens via `go build -ldflags -X`
// and can't be exercised from within a test binary).
func TestGoogleConfiguredTracksBuildDefault(t *testing.T) {
	t.Setenv(envGoogleClientID, "")
	t.Setenv(envGoogleClientSecret, "")

	origID, origSecret := defaultGoogleClientID, defaultGoogleClientSecret
	t.Cleanup(func() { defaultGoogleClientID, defaultGoogleClientSecret = origID, origSecret })

	defaultGoogleClientID, defaultGoogleClientSecret = "", ""
	if googleConfigured() {
		t.Error("googleConfigured() = true with no creds at all; want false")
	}

	defaultGoogleClientID = "default.apps.googleusercontent.com"
	defaultGoogleClientSecret = "GOCSPX-default" //nolint:gosec // test fixture, not a real credential

	if !googleConfigured() {
		t.Error("googleConfigured() = false with build default set; want true")
	}
	gotID, gotSecret := resolveGoogleClientCreds()
	if gotID != defaultGoogleClientID || gotSecret != defaultGoogleClientSecret {
		t.Errorf("resolveGoogleClientCreds() = (%q, %q), want build defaults (%q, %q)",
			gotID, gotSecret, defaultGoogleClientID, defaultGoogleClientSecret)
	}

	// Env var set alongside a build default: env wins.
	t.Setenv(envGoogleClientID, "env.apps.googleusercontent.com")
	t.Setenv(envGoogleClientSecret, "GOCSPX-env")
	gotID, gotSecret = resolveGoogleClientCreds()
	if gotID != "env.apps.googleusercontent.com" || gotSecret != "GOCSPX-env" { //nolint:gosec // test fixture, not a real credential
		t.Errorf("resolveGoogleClientCreds() = (%q, %q), want env override", gotID, gotSecret)
	}
}

// TestBeginAuthUnconfigured surfaces an actionable error (not a dead flow)
// when neither the server nor the instance has a Google client.
func TestBeginAuthUnconfigured(t *testing.T) {
	t.Setenv(envGoogleClientID, "")
	t.Setenv(envGoogleClientSecret, "")
	origID, origSecret := defaultGoogleClientID, defaultGoogleClientSecret
	t.Cleanup(func() { defaultGoogleClientID, defaultGoogleClientSecret = origID, origSecret })
	defaultGoogleClientID, defaultGoogleClientSecret = "", ""

	p := newTestPlugin()
	_, err := p.BeginAuth(context.Background(), app.Instance{Name: "gmail"})
	if err == nil {
		t.Fatal("BeginAuth with no client creds returned nil error; want fallback message")
	}
}

// TestBeginAuthWithInstanceCreds uses Advanced-pasted client_id/secret when
// the server has no Google client — the bring-your-own-client Sign in path.
func TestBeginAuthWithInstanceCreds(t *testing.T) {
	t.Setenv(envGoogleClientID, "")
	t.Setenv(envGoogleClientSecret, "")
	origID, origSecret := defaultGoogleClientID, defaultGoogleClientSecret
	t.Cleanup(func() { defaultGoogleClientID, defaultGoogleClientSecret = origID, origSecret })
	defaultGoogleClientID, defaultGoogleClientSecret = "", ""

	p := newTestPlugin()
	inst := app.Instance{
		Name: "gmail",
		Secrets: app.MapSecrets{
			"client_id":     "byo.apps.googleusercontent.com",
			"client_secret": "GOCSPX-byo",
		},
	}
	sess, err := p.BeginAuth(context.Background(), inst)
	if err != nil {
		t.Fatalf("BeginAuth with instance creds: %v", err)
	}
	defer p.oauth.PollAuth(context.Background(), app.AuthSession{ID: sess.ID}) //nolint:errcheck // drive teardown
	if sess.Kind != app.AuthKindCallback {
		t.Errorf("Kind = %q, want %q", sess.Kind, app.AuthKindCallback)
	}
	if !strings.Contains(sess.AuthURL, "accounts.google.com") {
		t.Errorf("consent URL missing Google host: %s", sess.AuthURL)
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
