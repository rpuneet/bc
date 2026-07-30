package grafana

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Grafana webhooks.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "grafana",
		Label: "Grafana",
		Auth:  app.AuthWebhookSecret,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "token", Label: "API Token", Placeholder: "grafana-api-token", Secret: true},
		},
		Docs: []string{
			"Go to Grafana → Alerting → Contact Points.",
			"Add a webhook contact point with URL /hooks/grafana.",
			"Copy an API token from Configuration → API Keys.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return NewNamed(inst.Name, inst.OptionalSecret("token")), nil
}
