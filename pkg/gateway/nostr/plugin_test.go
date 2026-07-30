package nostr

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "nostr" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want nostr, non-empty", d.ID, d.Label)
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
		App:    "nostr",
		Name:   "nostr",
		Config: map[string]string{"relay_url": "wss://relay.damus.io"},
	}
	adapter, err := plugin{}.Build(inst, app.Env{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if adapter.Name() != "nostr" {
		t.Errorf("Name() = %q, want nostr", adapter.Name())
	}
}

func TestPluginBuildMissingRelay(t *testing.T) {
	inst := app.Instance{App: "nostr", Name: "nostr", Config: map[string]string{}}
	if _, err := (plugin{}).Build(inst, app.Env{}); err == nil {
		t.Fatal("Build without relay_url succeeded, want error")
	}
}
