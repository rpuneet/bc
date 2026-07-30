package googlechat

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Google Chat webhooks.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "googlechat",
		Label: "Google Chat",
		Auth:  app.AuthWebhookSecret,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "secret", Label: "Webhook Secret", Placeholder: "googlechat-secret", Secret: true},
		},
		Docs: []string{
			"Create a Google Chat app with an HTTP endpoint.",
			"Point the endpoint at /hooks/googlechat on your mycel server.",
			"Set a shared secret here for payload verification.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return NewNamed(inst.Name, inst.OptionalSecret("secret")), nil
}
