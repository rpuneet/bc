package app

import (
	"strings"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	desc := Descriptor{
		ID:   "telegram",
		Auth: AuthToken,
		Fields: []FieldSpec{
			{Key: "bot_token", Label: "Bot Token", Secret: true, Required: true},
			{Key: "mode", Label: "Mode"},
			{Key: "url", Label: "URL", Required: true},
		},
	}

	tests := []struct {
		cfg     map[string]string
		name    string
		errPart string
	}{
		{
			name: "valid",
			cfg:  map[string]string{"mode": "poll", "url": "https://example.com"},
		},
		{
			name: "valid without optional",
			cfg:  map[string]string{"url": "https://example.com"},
		},
		{
			name:    "nil config misses required",
			cfg:     nil,
			errPart: `required field "url"`,
		},
		{
			name:    "unknown key rejected",
			cfg:     map[string]string{"url": "https://example.com", "bogus": "x"},
			errPart: `unknown config key "bogus"`,
		},
		{
			name:    "secret in config rejected",
			cfg:     map[string]string{"url": "https://example.com", "bot_token": "tok"},
			errPart: `field "bot_token" is a secret`,
		},
		{
			name:    "required non-secret empty",
			cfg:     map[string]string{"url": ""},
			errPart: `required field "url"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(desc, tt.cfg)
			if tt.errPart == "" {
				if err != nil {
					t.Fatalf("ValidateConfig: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.errPart) {
				t.Fatalf("ValidateConfig error = %v, want containing %q", err, tt.errPart)
			}
		})
	}
}

func TestValidateConfigRequiredSecretNotEnforced(t *testing.T) {
	// Required secret fields live in the vault; config validation must not
	// demand them.
	desc := Descriptor{
		ID:     "slack",
		Fields: []FieldSpec{{Key: "bot_token", Secret: true, Required: true}},
	}
	if err := ValidateConfig(desc, map[string]string{}); err != nil {
		t.Fatalf("ValidateConfig: %v", err)
	}
}

func TestResolveInstance(t *testing.T) {
	secrets := MapSecrets{"bot_token": "tok"}
	tests := []struct {
		name     string
		instName string
		cfg      InstanceConfig
	}{
		{
			name:     "plain instance",
			instName: "slack",
			cfg:      InstanceConfig{App: "slack", Enabled: true, Config: map[string]string{"mode": "socket"}},
		},
		{
			name:     "labeled disabled instance",
			instName: "telegram:alerts",
			cfg:      InstanceConfig{App: "telegram", Enabled: false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := ResolveInstance(tt.instName, tt.cfg, secrets)
			if inst.App != tt.cfg.App {
				t.Errorf("App = %q, want %q", inst.App, tt.cfg.App)
			}
			if inst.Name != tt.instName {
				t.Errorf("Name = %q, want %q", inst.Name, tt.instName)
			}
			if inst.Enabled != tt.cfg.Enabled {
				t.Errorf("Enabled = %v, want %v", inst.Enabled, tt.cfg.Enabled)
			}
			for k, v := range tt.cfg.Config {
				if inst.Config[k] != v {
					t.Errorf("Config[%q] = %q, want %q", k, inst.Config[k], v)
				}
			}
			got, err := inst.Secrets.Get("bot_token")
			if err != nil || got != "tok" {
				t.Errorf("Secrets.Get(bot_token) = %q, %v; want tok, nil", got, err)
			}
		})
	}
}

func TestValidInstanceName(t *testing.T) {
	valid := []string{"slack", "telegram:alerts", "a1", "irc", "x:y_z-2"}
	for _, n := range valid {
		if !ValidInstanceName(n) {
			t.Errorf("ValidInstanceName(%q) = false, want true", n)
		}
	}
	invalid := []string{"", "../etc", "a/b", "a:b:c", ":label", "app:",
		"UPPER", "a b", ".hidden", "app:..", "-lead", "app:/x"}
	for _, n := range invalid {
		if ValidInstanceName(n) {
			t.Errorf("ValidInstanceName(%q) = true, want false", n)
		}
	}
}
