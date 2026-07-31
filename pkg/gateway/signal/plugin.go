package signal

import (
	"fmt"
	"strconv"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Signal (signal-cli REST) polling.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "signal",
		Label: "Signal",
		Auth:  app.AuthNone,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "api_url", Label: "signal-cli REST API URL", Placeholder: "http://localhost:8080", Required: true},
			{Key: "interval", Label: "Poll Interval (seconds)", Placeholder: "10"},
		},
		Docs: []string{
			"Install signal-cli-rest-api → https://github.com/bbernhard/signal-cli-rest-api",
			"Run: docker run -p 8080:8080 bbernhard/signal-cli-rest-api",
			"Register or link your phone number, then enter the API URL.",
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
	return NewNamed(inst.Name, apiURL, interval), nil
}
