package homeassistant

import (
	"fmt"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Home Assistant.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "homeassistant",
		Label: "Home Assistant",
		Auth:  app.AuthToken,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "url", Label: "Server URL", Placeholder: "http://homeassistant.local:8123", Required: true},
			{Key: "token", Label: "Long-Lived Access Token", Placeholder: "eyJ...", Secret: true, Required: true},
		},
		Docs: []string{
			"Open your Home Assistant profile → Security → Long-lived access tokens.",
			"Create a token and paste it here together with the server URL.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	url := inst.Config["url"]
	if url == "" {
		return nil, fmt.Errorf("app %s: required field %q is missing", inst.Name, "url")
	}
	token, err := inst.RequiredSecret("token")
	if err != nil {
		return nil, err
	}
	return NewNamed(inst.Name, url, token), nil
}
