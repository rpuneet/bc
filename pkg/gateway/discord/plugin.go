package discord

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Discord.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "discord",
		Label: "Discord",
		Auth:  app.AuthToken,
		Fields: []app.FieldSpec{
			{Key: "bot_token", Label: "Bot Token", Placeholder: "MTIz...", Secret: true, Required: true},
		},
		Docs: []string{
			"Create an app → https://discord.com/developers/applications",
			"Enable MESSAGE CONTENT INTENT under Bot settings, copy the bot token.",
			"Generate an invite URL with bot scope and add to your server.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	token, err := inst.RequiredSecret("bot_token")
	if err != nil {
		return nil, err
	}
	return New(token), nil
}
