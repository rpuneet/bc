package mqtt

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "mqtt" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want mqtt, non-empty", d.ID, d.Label)
	}
	if d.Auth != app.AuthNone {
		t.Errorf("Auth = %q, want %q", d.Auth, app.AuthNone)
	}
	if !d.Multi {
		t.Error("Multi = false, want true")
	}
	for i, f := range d.Fields {
		if f.Key == "" {
			t.Errorf("Fields[%d] has empty key", i)
		}
	}
	if len(d.Docs) == 0 {
		t.Error("Docs is empty")
	}
}

func TestPluginBuild(t *testing.T) {
	inst := app.Instance{
		App:    "mqtt",
		Name:   "mqtt:home",
		Config: map[string]string{"broker_url": "tcp://localhost:1883"},
	}
	adapter, err := plugin{}.Build(inst, app.Env{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if adapter.Name() != "mqtt:home" {
		t.Errorf("Name() = %q, want mqtt:home", adapter.Name())
	}
}

func TestPluginBuildMissingBroker(t *testing.T) {
	inst := app.Instance{App: "mqtt", Name: "mqtt", Config: map[string]string{}}
	if _, err := (plugin{}).Build(inst, app.Env{}); err == nil {
		t.Fatal("Build without broker_url succeeded, want error")
	}
}
