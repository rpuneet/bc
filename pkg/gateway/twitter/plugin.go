package twitter

import (
	"fmt"
	"strconv"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Twitter/X polling.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "twitter",
		Label: "Twitter / X",
		Auth:  app.AuthToken,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "bearer_token", Label: "Bearer Token", Placeholder: "Twitter API v2 bearer token", Secret: true, Required: true},
			{Key: "user_id", Label: "User ID", Placeholder: "Numeric user ID to monitor", Required: true},
			{Key: "interval", Label: "Poll Interval (seconds)", Placeholder: "60"},
		},
		Docs: []string{
			"Create a developer app → https://developer.twitter.com/en/portal/dashboard",
			"Copy the Bearer Token from the app settings.",
			"Find your numeric user ID (not @handle).",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	bearer, err := inst.RequiredSecret("bearer_token")
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
	return NewNamed(inst.Name, bearer, inst.Config["user_id"], interval), nil
}
