package webhook

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for generic inbound webhooks.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "webhook",
		Label: "Generic Webhook",
		Auth:  app.AuthWebhookSecret,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "secret", Label: "Shared Secret (optional)", Placeholder: "optional-secret", Secret: true},
		},
		Docs: []string{
			"POST JSON to /hooks/webhook on your mycel server.",
			"Optionally set a shared secret for HMAC signature verification.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return NewWithSecret(inst.Name, inst.OptionalSecret("secret")), nil
}
