package github

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for GitHub webhooks.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "github",
		Label: "GitHub",
		Auth:  app.AuthWebhookSecret,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "secret", Label: "Webhook Secret", Placeholder: "your-webhook-secret", Secret: true},
		},
		Docs: []string{
			"Create a webhook → your repo → Settings → Webhooks.",
			"Set the payload URL to your mycel server's /hooks/github endpoint.",
			"Set the secret here to match the webhook secret.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return NewNamed(inst.Name, inst.OptionalSecret("secret")), nil
}
