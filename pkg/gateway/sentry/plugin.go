package sentry

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Sentry webhooks.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "sentry",
		Label: "Sentry",
		Auth:  app.AuthWebhookSecret,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "secret", Label: "Client Secret", Placeholder: "sentry-client-secret", Secret: true},
		},
		Docs: []string{
			"Go to Sentry → Settings → Integrations → Internal Integration.",
			"Create an integration with webhook URL pointing to /hooks/sentry.",
			"Copy the client secret and paste it here.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return NewNamed(inst.Name, inst.OptionalSecret("secret")), nil
}
