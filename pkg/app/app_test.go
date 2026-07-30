package app

import (
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/secret"
)

func TestMapSecrets(t *testing.T) {
	tests := []struct {
		secrets MapSecrets
		name    string
		key     string
		want    string
		wantErr bool
	}{
		{name: "present", key: "bot_token", want: "xoxb-1", secrets: MapSecrets{"bot_token": "xoxb-1"}},
		{name: "missing", key: "app_token", secrets: MapSecrets{"bot_token": "xoxb-1"}, wantErr: true},
		{name: "empty map", key: "bot_token", secrets: MapSecrets{}, wantErr: true},
		{name: "empty value present", key: "bot_token", want: "", secrets: MapSecrets{"bot_token": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.secrets.Get(tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Get(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestVaultSecrets(t *testing.T) {
	store, err := secret.NewStore(t.TempDir(), "test-passphrase")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	if err := store.Set(SecretName("slack", "bot_token"), "xoxb-vault", ""); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Set(SecretName("telegram:alerts", "bot_token"), "tg-vault", ""); err != nil {
		t.Fatalf("Set: %v", err)
	}

	tests := []struct {
		name     string
		instance string
		key      string
		want     string
		wantErr  bool
	}{
		{name: "resolves namespaced key", instance: "slack", key: "bot_token", want: "xoxb-vault"},
		{name: "labeled instance", instance: "telegram:alerts", key: "bot_token", want: "tg-vault"},
		{name: "missing key", instance: "slack", key: "app_token", wantErr: true},
		{name: "wrong instance", instance: "telegram", key: "bot_token", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs := VaultSecrets{Store: store, Instance: tt.instance}
			got, err := vs.Get(tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Get(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestSecretName(t *testing.T) {
	if got, want := SecretName("telegram:alerts", "bot_token"), "app:telegram:alerts:bot_token"; got != want {
		t.Errorf("SecretName = %q, want %q", got, want)
	}
}

func TestInstanceRequiredSecret(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		want    string
		errPart string
		inst    Instance
	}{
		{
			name: "present",
			key:  "bot_token",
			want: "tok",
			inst: Instance{Name: "slack", Secrets: MapSecrets{"bot_token": "tok"}},
		},
		{
			name:    "missing",
			key:     "bot_token",
			errPart: `required secret "bot_token"`,
			inst:    Instance{Name: "slack", Secrets: MapSecrets{}},
		},
		{
			name:    "empty value",
			key:     "bot_token",
			errPart: "is empty",
			inst:    Instance{Name: "slack", Secrets: MapSecrets{"bot_token": ""}},
		},
		{
			name:    "nil source",
			key:     "bot_token",
			errPart: "no secret source",
			inst:    Instance{Name: "slack"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.inst.RequiredSecret(tt.key)
			if tt.errPart != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errPart) {
					t.Fatalf("RequiredSecret error = %v, want containing %q", err, tt.errPart)
				}
				return
			}
			if err != nil {
				t.Fatalf("RequiredSecret: %v", err)
			}
			if got != tt.want {
				t.Errorf("RequiredSecret = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInstanceOptionalSecret(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
		inst Instance
	}{
		{name: "present", key: "secret", want: "shh", inst: Instance{Secrets: MapSecrets{"secret": "shh"}}},
		{name: "missing", key: "secret", want: "", inst: Instance{Secrets: MapSecrets{}}},
		{name: "nil source", key: "secret", want: "", inst: Instance{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.inst.OptionalSecret(tt.key); got != tt.want {
				t.Errorf("OptionalSecret = %q, want %q", got, tt.want)
			}
		})
	}
}
