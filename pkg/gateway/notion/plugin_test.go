package notion

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "notion" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want notion, non-empty", d.ID, d.Label)
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
		App:     "notion",
		Name:    "notion:docs",
		Enabled: true,
		Config:  map[string]string{"interval": "120"},
		Secrets: app.MapSecrets{"token": "secret_test"},
	}
	adapter, err := plugin{}.Build(inst, app.Env{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if adapter.Name() != "notion:docs" {
		t.Errorf("Name() = %q, want notion:docs", adapter.Name())
	}
}

func TestPluginBuildMissingToken(t *testing.T) {
	inst := app.Instance{App: "notion", Name: "notion", Secrets: app.MapSecrets{}}
	if _, err := (plugin{}).Build(inst, app.Env{}); err == nil {
		t.Fatal("Build without token succeeded, want error")
	}
}

func TestPluginBuildBadInterval(t *testing.T) {
	inst := app.Instance{
		App:     "notion",
		Name:    "notion",
		Config:  map[string]string{"interval": "abc"},
		Secrets: app.MapSecrets{"token": "secret_test"},
	}
	if _, err := (plugin{}).Build(inst, app.Env{}); err == nil {
		t.Fatal("Build with bad interval succeeded, want error")
	}
}
