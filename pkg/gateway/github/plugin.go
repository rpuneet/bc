package github

import (
	"context"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for GitHub webhooks, plus app.OAuthFlow:
// "Sign in with GitHub" runs the device flow entirely locally and stores
// the resulting token as the optional api_token secret.
type plugin struct {
	oauth *deviceFlow
}

var (
	_ app.Plugin    = (*plugin)(nil)
	_ app.OAuthFlow = (*plugin)(nil)
)

func init() {
	app.Register(&plugin{oauth: newDeviceFlow()})
}

func (*plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "github",
		Label: "GitHub",
		Auth:  app.AuthWebhookSecret,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "secret", Label: "Webhook Secret", Placeholder: "your-webhook-secret", Secret: true},
			{
				Key:         "oauth_client_id",
				Label:       "OAuth Client ID (advanced)",
				Placeholder: "Ov23li... (leave blank to use mycel's built-in sign-in)",
			},
			{Key: "api_token", Label: "API Token", Placeholder: "ghp_... (or use Sign in with GitHub)", Secret: true},
		},
		Docs: []string{
			"Create a webhook → your repo → Settings → Webhooks.",
			"Set the payload URL to your mycel server's /hooks/github endpoint.",
			"Set the secret here to match the webhook secret.",
			"Sign in with GitHub works out of the box — no setup needed — using mycel's built-in OAuth app to mint an api_token for outbound calls (comments, statuses).",
			"Optional: paste your own GitHub OAuth app's client ID (advanced — for your own org, or higher rate limits). Create one at https://github.com/settings/applications/new; the device flow needs no client secret and no redirect URL. Leave blank to use mycel's built-in sign-in.",
		},
	}
}

func (*plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return NewNamed(inst.Name, inst.OptionalSecret("secret")), nil
}

// BeginAuth starts the GitHub device flow for this instance.
func (p *plugin) BeginAuth(ctx context.Context, inst app.Instance) (app.AuthSession, error) {
	return p.oauth.BeginAuth(ctx, inst)
}

// PollAuth reports device-flow progress; on success the api_token secret
// is returned for the server to persist.
func (p *plugin) PollAuth(ctx context.Context, session app.AuthSession) (app.AuthResult, error) {
	return p.oauth.PollAuth(ctx, session)
}
