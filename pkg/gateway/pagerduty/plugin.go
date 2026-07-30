package pagerduty

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for PagerDuty webhooks.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "pagerduty",
		Label: "PagerDuty",
		Auth:  app.AuthWebhookSecret,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "secret", Label: "Signing Secret", Placeholder: "pagerduty-secret", Secret: true},
		},
		Docs: []string{
			"Go to PagerDuty → Integrations → Generic Webhooks V3.",
			"Add a webhook subscription pointing to /hooks/pagerduty.",
			"Copy the signing secret and paste it here.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return NewNamed(inst.Name, inst.OptionalSecret("secret")), nil
}
