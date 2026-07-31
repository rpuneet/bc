package mqtt

import (
	"fmt"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for MQTT.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "mqtt",
		Label: "MQTT",
		Auth:  app.AuthNone,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "broker_url", Label: "Broker URL", Placeholder: "tcp://localhost:1883", Required: true},
			{Key: "topic", Label: "Topic", Placeholder: "home/sensors/#"},
		},
		Docs: []string{
			"MQTT broker docs → https://mosquitto.org/ or https://www.hivemq.com/",
			"Enter your broker URL and the topic pattern to subscribe to.",
		},
	}
}

func (plugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	broker := inst.Config["broker_url"]
	if broker == "" {
		return nil, fmt.Errorf("app %s: required field %q is missing", inst.Name, "broker_url")
	}
	topics := []string{inst.Config["topic"]}
	if inst.Config["topic"] == "" {
		topics = []string{"#"}
	}
	return New(inst.Name, Config{
		Broker:   broker,
		ClientID: "mycel-" + inst.Name,
		Topics:   topics,
	}), nil
}
