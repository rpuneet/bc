package nostr

import (
	"fmt"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Nostr relays.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "nostr",
		Label: "Nostr",
		Auth:  app.AuthNone,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "relay_url", Label: "Relay URL", Placeholder: "wss://relay.damus.io", Required: true},
		},
		Docs: []string{
			"Enter a Nostr relay WebSocket URL (e.g. wss://relay.damus.io).",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	relayURL := inst.Config["relay_url"]
	if relayURL == "" {
		return nil, fmt.Errorf("app %s: required field %q is missing", inst.Name, "relay_url")
	}
	return NewNamed(inst.Name, relayURL), nil
}
