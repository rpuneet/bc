package rss

import (
	"fmt"
	"strconv"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for RSS/Atom feeds.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "rss",
		Label: "RSS / Atom",
		Auth:  app.AuthNone,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "url", Label: "Feed URL", Placeholder: "https://example.com/feed.xml", Required: true},
			{Key: "interval", Label: "Poll Interval (seconds)", Placeholder: "300"},
		},
		Docs: []string{
			"Paste any RSS or Atom feed URL.",
			"Set a poll interval in seconds (default: 300 = 5 minutes).",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	url := inst.Config["url"]
	if url == "" {
		return nil, fmt.Errorf("app %s: required field %q is missing", inst.Name, "url")
	}
	interval := 0
	if v := inst.Config["interval"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("app %s: invalid interval %q", inst.Name, v)
		}
		interval = n
	}
	return NewNamed(inst.Name, url, interval), nil
}
