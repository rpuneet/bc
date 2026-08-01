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

// Server-side Google "Desktop app" OAuth client, read from the environment.
// These are the mycel-registered client credentials that make one-click
// "Connect with Google" work; when unset, the connect UI falls back to the
// manual client-id/secret/refresh-token paste.
const (
	envGoogleClientID     = "GOOGLE_OAUTH_CLIENT_ID"
	envGoogleClientSecret = "GOOGLE_OAUTH_CLIENT_SECRET" //nolint:gosec // env var name, not a credential
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

// googleClientCreds resolves the server-side Google client from the
// environment, returning an actionable error (surfaced to the user) when the
// one-click client is not configured.
func googleClientCreds(_ app.Instance) (clientID, clientSecret string, err error) {
	clientID = strings.TrimSpace(os.Getenv(envGoogleClientID))
	clientSecret = strings.TrimSpace(os.Getenv(envGoogleClientSecret))
	if clientID == "" || clientSecret == "" {
		return "", "", fmt.Errorf(
			"one-click Google sign-in is not configured on this server — set %s and %s to a Google \"Desktop app\" OAuth client, or paste an OAuth refresh token below instead",
			envGoogleClientID, envGoogleClientSecret)
	}
	return clientID, clientSecret, nil
}

// googleConfigured reports whether one-click Google sign-in is available.
func googleConfigured() bool {
	return strings.TrimSpace(os.Getenv(envGoogleClientID)) != "" &&
		strings.TrimSpace(os.Getenv(envGoogleClientSecret)) != ""
}
