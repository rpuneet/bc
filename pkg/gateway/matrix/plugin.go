package matrix

import (
	"fmt"
	"strconv"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Matrix polling.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "matrix",
		Label: "Matrix",
		Auth:  app.AuthToken,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "homeserver", Label: "Homeserver URL", Placeholder: "https://matrix.org", Required: true},
			{Key: "token", Label: "Access Token", Placeholder: "syt_...", Secret: true, Required: true},
			{Key: "interval", Label: "Poll Interval (seconds)", Placeholder: "10"},
		},
		Docs: []string{
			"Download Element → https://element.io/download",
			"Get an access token: Element → Settings → Help & About → Access Token.",
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
	return NewNamed(inst.Name, inst.Config["homeserver"], token, interval), nil
}
