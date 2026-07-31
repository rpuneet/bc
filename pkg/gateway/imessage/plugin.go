package imessage

import (
	"fmt"
	"strconv"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for iMessage (BlueBubbles) polling.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "imessage",
		Label: "iMessage",
		Auth:  app.AuthToken,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "api_url", Label: "BlueBubbles API URL", Placeholder: "http://localhost:1234", Required: true},
			{Key: "password", Label: "API Password", Placeholder: "BlueBubbles password", Secret: true},
			{Key: "interval", Label: "Poll Interval (seconds)", Placeholder: "10"},
		},
		Docs: []string{
			"Install BlueBubbles on a Mac → https://bluebubbles.app",
			"Enable the API server in BlueBubbles settings.",
			"Enter the API URL and password from BlueBubbles.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	apiURL := inst.Config["api_url"]
	if apiURL == "" {
		return nil, fmt.Errorf("app %s: required field %q is missing", inst.Name, "api_url")
	}
	interval := 0
	if v := inst.Config["interval"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("app %s: invalid interval %q", inst.Name, v)
		}
		interval = n
	}
	return NewNamed(inst.Name, apiURL, inst.OptionalSecret("password"), interval), nil
}
