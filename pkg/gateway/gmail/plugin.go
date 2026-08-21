package gmailgw

import (
	"context"
	"fmt"
	"strconv"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/oauth"
)

// plugin implements app.Plugin for Gmail, plus app.OAuthFlow: "Sign in with
// Gmail" runs a local loopback OAuth flow that mints the refresh token.
// The connect UI always offers both paths — browser sign-in and Advanced
// manual paste of client id / secret / refresh token. One-click uses the
// server Google client (env or release ldflags) when present; otherwise
// BeginAuth uses client_id / client_secret the user entered in Advanced.
type plugin struct {
	oauth *oauth.LoopbackFlow
}

var (
	_ app.Plugin    = (*plugin)(nil)
	_ app.OAuthFlow = (*plugin)(nil)
)

func init() {
	app.Register(&plugin{oauth: newGoogleFlow()})
}

func (*plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "gmail",
		Label: "Gmail",
		Auth:  app.AuthToken,
		Fields: []app.FieldSpec{
			{Key: "client_id", Label: "OAuth Client ID", Placeholder: "xxxx.apps.googleusercontent.com", Secret: true, Required: true},
			{Key: "client_secret", Label: "OAuth Client Secret", Placeholder: "GOCSPX-...", Secret: true, Required: true},
			{Key: "refresh_token", Label: "OAuth Refresh Token", Placeholder: "1//0g...", Secret: true, Required: true},
			{Key: "label", Label: "Inbox Label", Placeholder: "INBOX"},
			{Key: "query", Label: "Search Query", Placeholder: "is:unread"},
			{Key: "interval", Label: "Poll Interval (seconds)", Placeholder: "60"},
		},
		Docs: []string{
			"Fastest path: click \"Sign in with Gmail\" — mycel opens Google in your browser and stores the token locally. Official releases (and servers with GOOGLE_OAUTH_CLIENT_ID/SECRET set) need no pasting.",
			"Bring-your-own client: paste OAuth Client ID + Client Secret under Advanced, then click \"Sign in with Gmail\" — same browser link flow, using your Google Cloud Desktop-app credentials.",
			"Fully manual: create OAuth credentials in Google Cloud Console → APIs & Services → Credentials, then paste Client ID, Client Secret, and a Refresh Token under Advanced (no browser step).",
			"Enable the Gmail API for the project, then create an OAuth 2.0 Client ID (type: Desktop app).",
			"Grant the scopes https://www.googleapis.com/auth/gmail.readonly and https://www.googleapis.com/auth/gmail.send on the consent screen.",
			"For the fully manual path, run the OAuth consent flow once (e.g. via the OAuth Playground with your own client) to obtain a refresh token for offline access.",
			"Optionally set a label (default INBOX), a Gmail search query (default is:unread), and a poll interval in seconds (default 60).",
		},
	}
}

// BeginAuth starts the loopback Google sign-in for this instance.
func (p *plugin) BeginAuth(ctx context.Context, inst app.Instance) (app.AuthSession, error) {
	return p.oauth.BeginAuth(ctx, inst)
}

// PollAuth reports sign-in progress; on success it returns the client
// credentials + refresh token for the server to persist to the vault.
func (p *plugin) PollAuth(ctx context.Context, session app.AuthSession) (app.AuthResult, error) {
	return p.oauth.PollAuth(ctx, session)
}

func (*plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	clientID, err := inst.RequiredSecret("client_id")
	if err != nil {
		return nil, err
	}
	clientSecret, err := inst.RequiredSecret("client_secret")
	if err != nil {
		return nil, err
	}
	refreshToken, err := inst.RequiredSecret("refresh_token")
	if err != nil {
		return nil, err
	}

	interval := 0
	if v := inst.Config["interval"]; v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil {
			return nil, fmt.Errorf("app %s: invalid interval %q", inst.Name, v)
		}
		interval = n
	}

	return New(Credentials{
		Name:         inst.Name,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RefreshToken: refreshToken,
		Label:        inst.Config["label"],
		Query:        inst.Config["query"],
		Interval:     interval,
	}), nil
}
