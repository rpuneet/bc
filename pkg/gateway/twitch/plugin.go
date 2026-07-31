package twitch

import (
	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Twitch webhooks.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "twitch",
		Label: "Twitch",
		Auth:  app.AuthWebhookSecret,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "secret", Label: "EventSub Secret", Placeholder: "twitch-eventsub-secret", Secret: true},
		},
		Docs: []string{
			"Create a Twitch application → https://dev.twitch.tv/console",
			"Register EventSub subscriptions with callback /hooks/twitch.",
			"Set the EventSub secret here for signature verification.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return NewNamed(inst.Name, inst.OptionalSecret("secret")), nil
}
