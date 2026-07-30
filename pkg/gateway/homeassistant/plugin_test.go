package homeassistant

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "homeassistant" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want homeassistant, non-empty", d.ID, d.Label)
	}
	if d.Auth != app.AuthToken {
		t.Errorf("Auth = %q, want %q", d.Auth, app.AuthToken)
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
		App:     "homeassistant",
		Name:    "homeassistant",
		Config:  map[string]string{"url": "http://homeassistant.local:8123"},
		Secrets: app.MapSecrets{"token": "eyJ-test"},
	}
	adapter, err := plugin{}.Build(inst, app.Env{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if adapter.Name() != "homeassistant" {
		t.Errorf("Name() = %q, want homeassistant", adapter.Name())
	}
}

func TestPluginBuildMissingToken(t *testing.T) {
	inst := app.Instance{
		App:     "homeassistant",
		Name:    "homeassistant",
		Config:  map[string]string{"url": "http://homeassistant.local:8123"},
		Secrets: app.MapSecrets{},
	}
	if _, err := (plugin{}).Build(inst, app.Env{}); err == nil {
		t.Fatal("Build without token succeeded, want error")
	}
}
