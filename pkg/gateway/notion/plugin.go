package notion

import (
	"fmt"
	"strconv"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Notion polling.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "notion",
		Label: "Notion",
		Auth:  app.AuthToken,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "token", Label: "API Token", Placeholder: "secret_...", Secret: true, Required: true},
			{Key: "interval", Label: "Poll Interval (seconds)", Placeholder: "300"},
		},
		Docs: []string{
			"Create an integration → https://www.notion.so/my-integrations",
			"Copy the API token and share target pages/databases with the integration.",
			"Set a poll interval in seconds (default: 300).",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	token, err := inst.RequiredSecret("token")
	if err != nil {
		return nil, err
	}
	interval := 0
	if v := inst.Config["interval"]; v != "" {
		n, atoiErr := strconv.Atoi(v)
		if atoiErr != nil {
			return nil, fmt.Errorf("app %s: invalid interval %q", inst.Name, v)
		}
		interval = n
	}
	return NewNamed(inst.Name, token, interval), nil
}
