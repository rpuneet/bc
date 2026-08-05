package telegram

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Telegram.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "telegram",
		Label: "Telegram",
		Auth:  app.AuthToken,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "bot_token", Label: "Bot Token", Placeholder: "1234567890:AAH...", Secret: true, Required: true},
			{Key: "mode", Label: "Mode", Placeholder: "polling"},
		},
		Docs: []string{
			"Message @BotFather on Telegram → https://t.me/BotFather — send /newbot.",
			"Copy the bot token. Message the bot in a DM or add it to a group.",
			"Channels appear after the first inbound message (telegram:<username|chat_id|group>).",
			"Do not subscribe to telegram:general or telegram:* as a stand-in — those are not real Telegram chats.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	token, err := inst.RequiredSecret("bot_token")
	if err != nil {
		return nil, err
	}
	return NewNamed(inst.Name, token, inst.Config["mode"]), nil
}
