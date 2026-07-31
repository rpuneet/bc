package stripe

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Stripe webhooks.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "stripe",
		Label: "Stripe",
		Auth:  app.AuthWebhookSecret,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "secret", Label: "Signing Secret", Placeholder: "whsec_...", Secret: true},
		},
		Docs: []string{
			"Go to Stripe Dashboard → Developers → Webhooks.",
			"Add an endpoint pointing to /hooks/stripe on your mycel server.",
			"Copy the signing secret and paste it here.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return NewNamed(inst.Name, inst.OptionalSecret("secret")), nil
}
