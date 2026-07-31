package slackgw

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Slack.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "slack",
		Label: "Slack",
		Auth:  app.AuthToken,
		Fields: []app.FieldSpec{
			{Key: "bot_token", Label: "Bot Token", Placeholder: "xoxb-...", Secret: true, Required: true},
			{Key: "app_token", Label: "App Token", Placeholder: "xapp-...", Secret: true},
			{Key: "mode", Label: "Mode", Placeholder: "socket"},
		},
		Docs: []string{
			"Create a Slack app → https://api.slack.com/apps — enable Socket Mode.",
			"Add scopes: channels:read, chat:write, connections:write.",
			"Copy Bot Token from OAuth & Permissions, App Token from Basic Information.",
			"Install the app and invite the bot to your channels.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	botToken, err := inst.RequiredSecret("bot_token")
	if err != nil {
		return nil, err
	}
	return New(botToken, inst.OptionalSecret("app_token")), nil
}
