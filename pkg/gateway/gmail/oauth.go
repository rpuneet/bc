package gmailgw

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmail "google.golang.org/api/gmail/v1"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/oauth"
)

// Server-side Google "Desktop app" OAuth client. These are the
// mycel-registered client credentials that make one-click "Connect with
// Google" work; when neither source below is set, the connect UI falls back
// to the manual client-id/secret/refresh-token paste.
//
// Resolution order:
//  1. GOOGLE_OAUTH_CLIENT_ID / GOOGLE_OAUTH_CLIENT_SECRET environment
//     variables — always wins, lets anyone override at runtime.
//  2. defaultGoogleClientID / defaultGoogleClientSecret — empty in source
//     (nothing secret is committed) and injected at build time via
//     `-ldflags -X` for official mycel release builds. For a Google
//     "installed application" (Desktop app) OAuth client, Google's own docs
//     say the client secret is "not treated as confidential" — see
//     https://developers.google.com/identity/protocols/oauth2#installed —
//     so baking it into the binary is the standard, supported pattern.
const (
	envGoogleClientID     = "GOOGLE_OAUTH_CLIENT_ID"
	envGoogleClientSecret = "GOOGLE_OAUTH_CLIENT_SECRET" //nolint:gosec // env var name, not a credential
)

// defaultGoogleClientID and defaultGoogleClientSecret are empty by default so
// nothing secret is ever committed to source. The official release build
// injects mycel's registered Google Desktop-app OAuth client via:
//
//	-ldflags "-X 'github.com/rpuneet/mycel/pkg/gateway/gmail.defaultGoogleClientID=...' \
//	          -X 'github.com/rpuneet/mycel/pkg/gateway/gmail.defaultGoogleClientSecret=...'"
//
// A local build with neither the env vars nor these ldflags set behaves
// exactly as before: one-click sign-in reports unconfigured and the UI falls
// back to the manual paste path.
var (
	defaultGoogleClientID     string //nolint:gochecknoglobals // ldflags injection target, empty by default
	defaultGoogleClientSecret string //nolint:gochecknoglobals // ldflags injection target, empty by default
)

// newGoogleFlow builds the loopback OAuth flow for Gmail: Google's endpoints,
// the Gmail read + send scopes, and offline+consent so Google always returns
// a refresh token. On completion it hands the client credentials and refresh
// token back as the vault secrets the Gmail adapter's Build expects.
func newGoogleFlow() *oauth.LoopbackFlow {
	return oauth.NewLoopbackFlow(
		oauth.Provider{
			Endpoint: google.Endpoint,
			Scopes:   []string{gmail.GmailReadonlyScope, gmail.GmailSendScope},
			// prompt=consent forces the consent screen so a refresh_token is
			// returned even on re-authorization (AccessTypeOffline alone can
			// omit it once the user has previously consented).
			AuthCodeOptions: []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("prompt", "consent")},
			Secrets: func(clientID, clientSecret string, tok *oauth2.Token) map[string]string {
				return map[string]string{
					"client_id":     clientID,
					"client_secret": clientSecret,
					"refresh_token": tok.RefreshToken,
				}
			},
		},
		googleClientCreds,
	)
}

// googleClientCreds resolves the server-side Google client — environment
// variables first, then the build-time embedded default — returning an
// actionable error (surfaced to the user) when neither is configured.
func googleClientCreds(_ app.Instance) (clientID, clientSecret string, err error) {
	clientID, clientSecret = resolveGoogleClientCreds()
	if clientID == "" || clientSecret == "" {
		return "", "", fmt.Errorf(
			"one-click Google sign-in is not configured on this server — set %s and %s to a Google \"Desktop app\" OAuth client, or paste an OAuth refresh token below instead",
			envGoogleClientID, envGoogleClientSecret)
	}
	return clientID, clientSecret, nil
}

// resolveGoogleClientCreds applies the resolution order documented above the
// default* vars: env override, then the build-injected default.
func resolveGoogleClientCreds() (clientID, clientSecret string) {
	clientID = strings.TrimSpace(os.Getenv(envGoogleClientID))
	clientSecret = strings.TrimSpace(os.Getenv(envGoogleClientSecret))
	if clientID == "" {
		clientID = strings.TrimSpace(defaultGoogleClientID)
	}
	if clientSecret == "" {
		clientSecret = strings.TrimSpace(defaultGoogleClientSecret)
	}
	return clientID, clientSecret
}

// googleConfigured reports whether one-click Google sign-in is available,
// whether via env vars or the build-time embedded default client.
func googleConfigured() bool {
	clientID, clientSecret := resolveGoogleClientCreds()
	return clientID != "" && clientSecret != ""
}
