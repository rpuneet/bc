package imessage

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "imessage" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want imessage, non-empty", d.ID, d.Label)
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
	tests := []struct {
		secrets app.MapSecrets
		name    string
	}{
		{name: "with password", secrets: app.MapSecrets{"password": "pw"}},
		{name: "without password", secrets: app.MapSecrets{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := app.Instance{
				App:     "imessage",
				Name:    "imessage",
				Config:  map[string]string{"api_url": "http://localhost:1234"},
				Secrets: tt.secrets,
			}
			adapter, err := plugin{}.Build(inst, app.Env{})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if adapter.Name() != "imessage" {
				t.Errorf("Name() = %q, want imessage", adapter.Name())
			}
		})
	}
}

func TestPluginBuildMissingAPIURL(t *testing.T) {
	inst := app.Instance{App: "imessage", Name: "imessage", Config: map[string]string{}}
	if _, err := (plugin{}).Build(inst, app.Env{}); err == nil {
		t.Fatal("Build without api_url succeeded, want error")
	}
}
