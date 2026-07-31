package telegram

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "telegram" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want telegram, non-empty", d.ID, d.Label)
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
		App:     "telegram",
		Name:    "telegram:alerts",
		Config:  map[string]string{"mode": "polling"},
		Secrets: app.MapSecrets{"bot_token": "123:AAH"},
	}
	adapter, err := plugin{}.Build(inst, app.Env{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if adapter.Name() != "telegram:alerts" {
		t.Errorf("Name() = %q, want telegram:alerts", adapter.Name())
	}
}

func TestPluginBuildMissingToken(t *testing.T) {
	inst := app.Instance{App: "telegram", Name: "telegram", Secrets: app.MapSecrets{}}
	if _, err := (plugin{}).Build(inst, app.Env{}); err == nil {
		t.Fatal("Build without bot_token succeeded, want error")
	}
}
