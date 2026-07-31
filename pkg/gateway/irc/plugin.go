package irc

import (
	"fmt"
	"strings"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for IRC.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "irc",
		Label: "IRC",
		Auth:  app.AuthNone,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "server", Label: "Server", Placeholder: "irc.libera.chat:6697", Required: true},
			{Key: "channels", Label: "Channels (comma-separated)", Placeholder: "#general,#dev"},
		},
		Docs: []string{
			"Popular servers: irc.libera.chat:6697, irc.oftc.net:6697 (TLS).",
			"List channels to join, separated by commas (e.g. #general,#dev).",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	server := inst.Config["server"]
	if server == "" {
		return nil, fmt.Errorf("app %s: required field %q is missing", inst.Name, "server")
	}
	var channels []string
	for _, ch := range strings.Split(inst.Config["channels"], ",") {
		if ch = strings.TrimSpace(ch); ch != "" {
			channels = append(channels, ch)
		}
	}
	return New(inst.Name, Config{
		Server:   server,
		Nick:     "mycel-bot",
		Channels: channels,
		UseTLS:   true,
	}), nil
}
