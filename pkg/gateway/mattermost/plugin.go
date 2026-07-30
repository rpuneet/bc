package mattermost

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Mattermost.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "mattermost",
		Label: "Mattermost",
		Auth:  app.AuthToken,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "url", Label: "Server URL", Placeholder: "https://mattermost.example.com"},
			{Key: "token", Label: "Personal Access Token", Placeholder: "abc123...", Secret: true},
		},
		Docs: []string{
			"Mattermost docs → https://docs.mattermost.com/developer/personal-access-tokens.html",
			"Go to Account Settings → Security → Personal Access Tokens, create and paste above.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return New(inst.Name, Config{
		URL:   inst.Config["url"],
		Token: inst.OptionalSecret("token"),
	}), nil
}
