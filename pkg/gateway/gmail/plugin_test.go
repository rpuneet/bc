package gmailgw

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

func TestPluginDescribe(t *testing.T) {
	d := newTestPlugin().Describe()
	if d.ID != "gmail" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want gmail, non-empty", d.ID, d.Label)
	}
	if d.Auth != app.AuthToken {
		t.Errorf("Auth = %q, want %q", d.Auth, app.AuthToken)
	}
	// Required credential fields must be secret so they land in the vault.
	wantSecret := map[string]bool{"client_id": true, "client_secret": true, "refresh_token": true}
	seen := map[string]app.FieldSpec{}
	for i, f := range d.Fields {
		if f.Key == "" {
			t.Errorf("Fields[%d] has empty key", i)
		}
		seen[f.Key] = f
	}
	for key := range wantSecret {
		f, ok := seen[key]
		if !ok {
			t.Errorf("missing field %q", key)
			continue
		}
		if !f.Secret {
			t.Errorf("field %q Secret = false, want true", key)
		}
		if !f.Required {
			t.Errorf("field %q Required = false, want true", key)
		}
	}
	if len(d.Docs) == 0 {
		t.Error("Docs is empty")
	}
	// The descriptor must satisfy the config validator (no unknown/secret
	// keys in a plain config map).
	if err := app.ValidateConfig(d, map[string]string{"label": "INBOX", "interval": "60"}); err != nil {
		t.Errorf("ValidateConfig() = %v, want nil", err)
	}
}

func TestPluginBuild(t *testing.T) {
	fullSecrets := app.MapSecrets{
		"client_id":     "cid",
		"client_secret": "csecret",
		"refresh_token": "rtok",
	}
	tests := []struct {
		secrets app.MapSecrets
		cfg     map[string]string
		name    string
		wantErr bool
	}{
		{name: "full creds", secrets: fullSecrets, cfg: map[string]string{"label": "INBOX", "interval": "30"}},
		{name: "creds only", secrets: fullSecrets, cfg: map[string]string{}},
		{name: "missing refresh_token", secrets: app.MapSecrets{"client_id": "cid", "client_secret": "csecret"}, wantErr: true},
		{name: "missing client_id", secrets: app.MapSecrets{"client_secret": "csecret", "refresh_token": "rtok"}, wantErr: true},
		{name: "bad interval", secrets: fullSecrets, cfg: map[string]string{"interval": "soon"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := app.Instance{App: "gmail", Name: "gmail", Config: tt.cfg, Secrets: tt.secrets}
			adapter, err := newTestPlugin().Build(inst, app.Env{})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Build error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if adapter.Name() != "gmail" {
				t.Errorf("Name() = %q, want gmail", adapter.Name())
			}
			if adapter.Type() != gateway.AdapterPoll {
				t.Errorf("Type() = %q, want %q", adapter.Type(), gateway.AdapterPoll)
			}
		})
	}
}

func TestPluginRegistered(t *testing.T) {
	if _, ok := app.Get("gmail"); !ok {
		t.Fatal("gmail plugin not registered in default registry")
	}
}
