package mattermost

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "mattermost" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want mattermost, non-empty", d.ID, d.Label)
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
		{name: "with token", secrets: app.MapSecrets{"token": "abc123"}},
		{name: "without token", secrets: app.MapSecrets{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := app.Instance{
				App:     "mattermost",
				Name:    "mattermost",
				Config:  map[string]string{"url": "https://mm.example.com"},
				Secrets: tt.secrets,
			}
			adapter, err := plugin{}.Build(inst, app.Env{})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if adapter.Name() != "mattermost" {
				t.Errorf("Name() = %q, want mattermost", adapter.Name())
			}
		})
	}
}
