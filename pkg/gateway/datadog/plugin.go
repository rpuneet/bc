package datadog

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Datadog webhooks.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "datadog",
		Label: "Datadog",
		Auth:  app.AuthWebhookSecret,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "secret", Label: "Shared Secret", Placeholder: "datadog-shared-secret", Secret: true},
		},
		Docs: []string{
			"Go to Datadog → Integrations → Webhooks.",
			"Datadog has NO built-in HMAC signing. To authenticate, put the same secret you set here into the webhook: either append it to the URL as /hooks/datadog?secret=<secret>, or add \"secret\": \"<secret>\" to the custom payload template.",
			"Leave the secret blank to accept the webhook unauthenticated.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return NewNamed(inst.Name, inst.OptionalSecret("secret")), nil
}
