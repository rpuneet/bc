package reddit

import (
	"fmt"
	"strconv"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for Reddit polling.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "reddit",
		Label: "Reddit",
		Auth:  app.AuthToken,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "subreddit", Label: "Subreddit", Placeholder: "golang", Required: true},
			{Key: "bearer_token", Label: "Bearer Token", Placeholder: "OAuth bearer token", Secret: true, Required: true},
			{Key: "interval", Label: "Poll Interval (seconds)", Placeholder: "60"},
		},
		Docs: []string{
			"Create a Reddit app → https://www.reddit.com/prefs/apps (script type).",
			"Generate an OAuth bearer token using client credentials.",
			"Enter the subreddit name (without r/) and poll interval.",
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
	return NewNamed(inst.Name, inst.Config["subreddit"], bearer, interval), nil
}
