package grafana

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "grafana" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want grafana, non-empty", d.ID, d.Label)
	}
	if d.Auth != app.AuthWebhookSecret {
		t.Errorf("Auth = %q, want %q", d.Auth, app.AuthWebhookSecret)
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
		{name: "with secret", secrets: app.MapSecrets{"token": "shh"}},
		{name: "without secret", secrets: app.MapSecrets{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := app.Instance{App: "grafana", Name: "grafana:ci", Secrets: tt.secrets}
			adapter, err := plugin{}.Build(inst, app.Env{})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if adapter.Name() != "grafana:ci" {
				t.Errorf("Name() = %q, want grafana:ci", adapter.Name())
			}
		})
	}
}
