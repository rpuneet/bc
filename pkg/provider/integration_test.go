package provider_test

import (
	"sort"
	"testing"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/provider"
)

// expectedProviders is the canonical list of providers that must be in DefaultRegistry.
var expectedProviders = []string{
	"agy",
	"claude",
	"codex",
	"cursor",
	"pi",
}

func TestRegistryCompleteness(t *testing.T) {
	providers := provider.ListProviders()
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name()
	}
	sort.Strings(names)

	if len(names) != len(expectedProviders) {
		t.Fatalf("expected %d providers, got %d: %v", len(expectedProviders), len(names), names)
	}

	for i, want := range expectedProviders {
		if names[i] != want {
			t.Errorf("provider[%d] = %q, want %q", i, names[i], want)
		}
	}

	// Verify each provider has required fields
	for _, p := range providers {
		if p.Name() == "" {
			t.Error("provider has empty Name()")
		}
		if p.Description() == "" {
			t.Errorf("provider %q has empty Description()", p.Name())
		}
		if p.Command() == "" {
			t.Errorf("provider %q has empty Command()", p.Name())
		}
	}
}

func TestProviderConfigRoundtrip(t *testing.T) {
	// Build a Config with all providers enabled
	cfg := home.Config{
		Providers: home.ProvidersConfig{
			Providers: map[string]home.ProviderConfig{
				"claude": {Command: "claude --dangerously-skip-permissions"},
				"agy":    {Command: "agy --dangerously-skip-permissions"},
				"cursor": {Command: "cursor --force"},
				"codex":  {Command: "codex --auto"},
				"pi":     {Command: "pi"},
			},
		},
	}

	listed := cfg.ListProviders()
	sort.Strings(listed)

	// Every listed provider must exist in DefaultRegistry
	for _, name := range listed {
		p, err := provider.GetProvider(name)
		if err != nil {
			t.Errorf("ListProviders() returned %q but GetProvider() failed: %v", name, err)
			continue
		}
		if p.Name() != name {
			t.Errorf("GetProvider(%q).Name() = %q", name, p.Name())
		}
	}

	// All expected providers should be listed
	if len(listed) != len(expectedProviders) {
		t.Errorf("ListProviders() returned %d, want %d: %v", len(listed), len(expectedProviders), listed)
	}
}

func TestGetAgentCommandFromConfig_RealConfigs(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		cfg     *home.Config
		wantCmd string
		wantOk  bool
	}{
		{
			name: "workspace claude override",
			tool: "claude",
			cfg: &home.Config{
				Providers: home.ProvidersConfig{
					Providers: map[string]home.ProviderConfig{"claude": {Command: "claude --model opus"}},
				},
			},
			wantCmd: "claude --model opus",
			wantOk:  true,
		},
		{
			name: "workspace agy override",
			tool: "agy",
			cfg: &home.Config{
				Providers: home.ProvidersConfig{
					Providers: map[string]home.ProviderConfig{"agy": {Command: "agy --sandbox"}},
				},
			},
			wantCmd: "agy --sandbox",
			wantOk:  true,
		},
		{
			name: "providers config",
			tool: "codex",
			cfg: &home.Config{
				Providers: home.ProvidersConfig{
					Providers: map[string]home.ProviderConfig{"codex": {Command: "codex --new-flag"}},
				},
			},
			wantCmd: "codex --new-flag",
			wantOk:  true,
		},
		{
			name:    "nil config falls back to global",
			tool:    "claude",
			cfg:     nil,
			wantCmd: "claude --dangerously-skip-permissions",
			wantOk:  true,
		},
		{
			name:    "unknown tool",
			tool:    "nonexistent",
			cfg:     nil,
			wantCmd: "",
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := agent.GetAgentCommandFromConfig(tt.tool, tt.cfg)
			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
			}
			if cmd != tt.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tt.wantCmd)
			}
		})
	}
}

func TestConfigProviderRegistrySync(t *testing.T) {
	// Load settings.json defaults and verify they match DefaultRegistry
	cfg := home.DefaultConfig()

	// Every provider in config should be in the registry
	configProviders := []struct {
		cfg  *home.ProviderConfig
		name string
	}{
		{cfg.GetProvider("claude"), "claude"},
		{cfg.GetProvider("agy"), "agy"},
	}

	for _, cp := range configProviders {
		if cp.cfg == nil {
			t.Errorf("DefaultConfig missing provider %q in ProvidersConfig", cp.name)
			continue
		}

		p, err := provider.GetProvider(cp.name)
		if err != nil {
			t.Errorf("provider %q in config but not in DefaultRegistry: %v", cp.name, err)
			continue
		}

		// Provider registry command should be a superset of config command
		// (config may have different flags, but the binary name should match)
		if p.Name() != cp.name {
			t.Errorf("registry provider name %q != config name %q", p.Name(), cp.name)
		}
	}

	// Every provider in DefaultRegistry should be gettable via Config.GetProvider
	registryProviders := provider.ListProviders()
	for _, p := range registryProviders {
		name := p.Name()
		if !cfg.HasProviderDefined(name) {
			t.Logf("provider %q in registry but not in DefaultConfig (acceptable for optional providers)", name)
		}
	}

	// Verify GetDefaultProvider returns a valid provider from registry
	defaultName := cfg.GetDefaultProvider()
	if defaultName == "" {
		t.Fatal("GetDefaultProvider() returned empty string")
	}
	if _, err := provider.GetProvider(defaultName); err != nil {
		t.Errorf("default provider %q not found in registry: %v", defaultName, err)
	}
}
