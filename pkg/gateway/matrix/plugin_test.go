package matrix

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "matrix" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want matrix, non-empty", d.ID, d.Label)
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
		App:     "matrix",
		Name:    "matrix",
		Enabled: true,
		Config:  map[string]string{"homeserver": "https://matrix.org"},
		Secrets: app.MapSecrets{"token": "syt_test"},
	}
	adapter, err := plugin{}.Build(inst, app.Env{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if adapter.Name() != "matrix" {
		t.Errorf("Name() = %q, want matrix", adapter.Name())
	}
}

func TestPluginBuildMissingToken(t *testing.T) {
	inst := app.Instance{
		App:     "matrix",
		Name:    "matrix",
		Config:  map[string]string{"homeserver": "https://matrix.org"},
		Secrets: app.MapSecrets{},
	}
	if _, err := (plugin{}).Build(inst, app.Env{}); err == nil {
		t.Fatal("Build without token succeeded, want error")
	}
}
