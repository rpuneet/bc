package irc

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "irc" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want irc, non-empty", d.ID, d.Label)
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
		App:    "irc",
		Name:   "irc:libera",
		Config: map[string]string{"server": "irc.libera.chat:6697", "channels": "#general, #dev"},
	}
	adapter, err := plugin{}.Build(inst, app.Env{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if adapter.Name() != "irc:libera" {
		t.Errorf("Name() = %q, want irc:libera", adapter.Name())
	}
}

func TestPluginBuildMissingServer(t *testing.T) {
	inst := app.Instance{App: "irc", Name: "irc", Config: map[string]string{}}
	if _, err := (plugin{}).Build(inst, app.Env{}); err == nil {
		t.Fatal("Build without server succeeded, want error")
	}
}
