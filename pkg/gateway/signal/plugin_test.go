package signal

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "signal" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want signal, non-empty", d.ID, d.Label)
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
		App:     "signal",
		Name:    "signal",
		Enabled: true,
		Config:  map[string]string{"api_url": "http://localhost:8080", "interval": "15"},
	}
	adapter, err := plugin{}.Build(inst, app.Env{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if adapter.Name() != "signal" {
		t.Errorf("Name() = %q, want signal", adapter.Name())
	}
}

func TestPluginBuildMissingAPIURL(t *testing.T) {
	inst := app.Instance{App: "signal", Name: "signal", Config: map[string]string{}}
	if _, err := (plugin{}).Build(inst, app.Env{}); err == nil {
		t.Fatal("Build without api_url succeeded, want error")
	}
}
