package feishu

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Feishu / Lark webhooks.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "feishu",
		Label: "Feishu / Lark",
		Auth:  app.AuthWebhookSecret,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "secret", Label: "Verification Secret", Placeholder: "feishu-verification-secret", Secret: true},
		},
		Docs: []string{
			"Create a Feishu/Lark app and enable event subscriptions.",
			"Set the request URL to /hooks/feishu on your mycel server.",
			"Copy the verification secret and paste it here.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return NewNamed(inst.Name, inst.OptionalSecret("secret")), nil
}
