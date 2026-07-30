package gitlab

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for GitLab webhooks.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "gitlab",
		Label: "GitLab",
		Auth:  app.AuthWebhookSecret,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "token", Label: "Secret Token", Placeholder: "webhook-secret-token", Secret: true},
		},
		Docs: []string{
			"Go to your GitLab project → Settings → Webhooks.",
			"Set the URL to your mycel server's /hooks/gitlab endpoint.",
			"Copy the secret token and paste it here.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return NewNamed(inst.Name, inst.OptionalSecret("token")), nil
}
