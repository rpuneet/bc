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

func TestPluginRequiredFields(t *testing.T) {
	d := plugin{}.Describe()
	required := map[string]bool{}
	for _, f := range d.Fields {
		required[f.Key] = f.Required
	}
	if !required["url"] {
		t.Error("url field must be Required")
	}
	if !required["token"] {
		t.Error("token field must be Required")
	}
}

func TestPluginBuild(t *testing.T) {
	tests := []struct {
		secrets app.MapSecrets
		config  map[string]string
		name    string
		wantErr bool
	}{
		{name: "with url and token", config: map[string]string{"url": "https://mm.example.com"}, secrets: app.MapSecrets{"token": "abc123"}},
		{name: "missing token", config: map[string]string{"url": "https://mm.example.com"}, secrets: app.MapSecrets{}, wantErr: true},
		{name: "missing url", config: map[string]string{}, secrets: app.MapSecrets{"token": "abc123"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := app.Instance{
				App:     "mattermost",
				Name:    "mattermost",
				Config:  tt.config,
				Secrets: tt.secrets,
			}
			adapter, err := plugin{}.Build(inst, app.Env{})
			if tt.wantErr {
				if err == nil {
					t.Fatal("Build: expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if adapter.Name() != "mattermost" {
				t.Errorf("Name() = %q, want mattermost", adapter.Name())
			}
		})
	}
}
