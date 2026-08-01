package gmailgw

import (
	"fmt"
	"strconv"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Gmail.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
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
			"Create OAuth credentials in Google Cloud Console → APIs & Services → Credentials.",
			"Enable the Gmail API for the project, then create an OAuth 2.0 Client ID (type: Desktop app).",
			"Grant the scopes https://www.googleapis.com/auth/gmail.readonly and https://www.googleapis.com/auth/gmail.send on the consent screen.",
			"Run the OAuth consent flow once (e.g. via the OAuth Playground with your own client) to obtain a refresh token for offline access.",
			"Paste the Client ID, Client Secret, and Refresh Token here. mycel builds an offline token source and refreshes access tokens automatically.",
			"Optionally set a label (default INBOX), a Gmail search query (default is:unread), and a poll interval in seconds (default 60).",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
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
