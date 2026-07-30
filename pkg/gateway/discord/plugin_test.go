package discord

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "discord" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want discord, non-empty", d.ID, d.Label)
	}
	if d.Auth != app.AuthToken {
		t.Errorf("Auth = %q, want %q", d.Auth, app.AuthToken)
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
		App:     "discord",
		Name:    "discord",
		Enabled: true,
		Secrets: app.MapSecrets{"bot_token": "MTIz-test"},
	}
	adapter, err := plugin{}.Build(inst, app.Env{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if adapter.Name() != "discord" {
		t.Errorf("Name() = %q, want discord", adapter.Name())
	}
}

func TestPluginBuildMissingToken(t *testing.T) {
	inst := app.Instance{App: "discord", Name: "discord", Secrets: app.MapSecrets{}}
	if _, err := (plugin{}).Build(inst, app.Env{}); err == nil {
		t.Fatal("Build without bot_token succeeded, want error")
	}
}
