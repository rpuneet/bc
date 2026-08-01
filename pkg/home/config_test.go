package home

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

// TestConfigAppsRoundTrip verifies the generic apps section survives a
// marshal/unmarshal cycle with plain and labeled instance keys.
func TestConfigAppsRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Apps = map[string]app.InstanceConfig{
		"slack":           {App: "slack", Enabled: true, Config: map[string]string{"mode": "socket"}},
		"telegram:alerts": {App: "telegram", Enabled: true, Config: map[string]string{"mode": "poll"}},
		"rss:blog":        {App: "rss", Enabled: false, Config: map[string]string{"url": "https://example.com/feed.xml"}},
	}

	data, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(parsed.Apps) != 3 {
		t.Fatalf("expected 3 apps, got %d", len(parsed.Apps))
	}
	if ic := parsed.Apps["telegram:alerts"]; ic.App != "telegram" || !ic.Enabled || ic.Config["mode"] != "poll" {
		t.Errorf("telegram:alerts round-trip mismatch: %+v", ic)
	}
	if ic := parsed.Apps["rss:blog"]; ic.App != "rss" || ic.Enabled {
		t.Errorf("rss:blog round-trip mismatch: %+v", ic)
	}
}

// TestDefaultConfigRuntimeIsTmux verifies fresh installs default to the tmux
// runtime (the RuntimePicker in the web UI labels tmux "Recommended", and the
// owner decided the fresh-install default should agree with that label).
// Docker remains fully supported — this only changes what a brand-new
// ~/.mycel/prefs.json starts with.
func TestDefaultConfigRuntimeIsTmux(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Runtime.Default != "tmux" {
		t.Fatalf("DefaultConfig().Runtime.Default = %q, want %q", cfg.Runtime.Default, "tmux")
	}
}

// TestConfigOnboardingRoundTrip verifies the onboarding section survives a
// marshal/unmarshal cycle and that OnboardingComplete tracks the "done"
// sentinel.
func TestConfigOnboardingRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Onboarding.OnboardingComplete() {
		t.Fatal("fresh config should not be marked onboarding-complete")
	}

	cfg.Onboarding = OnboardingConfig{Step: "runtime", Completed: []string{"welcome", "system"}}
	data, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if parsed.Onboarding.Step != "runtime" || len(parsed.Onboarding.Completed) != 2 {
		t.Errorf("onboarding round-trip mismatch: %+v", parsed.Onboarding)
	}
	if parsed.Onboarding.OnboardingComplete() {
		t.Error("OnboardingComplete = true without a done sentinel")
	}

	parsed.Onboarding.Completed = append(parsed.Onboarding.Completed, "done")
	if !parsed.Onboarding.OnboardingComplete() {
		t.Error("OnboardingComplete = false after appending done")
	}
}

// TestConfigRealignmentNonDestructive proves the v2→v3 realignment is
// non-destructive: an older prefs blob — carrying the retired
// runtime.k8s placeholder and none of the new fields (default_model,
// notifications) — loads cleanly, upgrades its version via FillDefaults,
// passes Validate, and re-serializes without the dropped k8s field.
func TestConfigRealignmentNonDestructive(t *testing.T) {
	// A realistic pre-realignment prefs.json: version 2, a stray
	// runtime.k8s "future" placeholder, and no new fields.
	old := `{
		"version": 2,
		"user": {"name": "dana"},
		"providers": {"default": "claude", "providers": {"claude": {"command": "claude"}}},
		"runtime": {"default": "docker", "k8s": {"cluster": "prod"}},
		"storage": {"default": "sqlite", "sqlite": {"path": ".mycel"}},
		"ui": {"theme": "dark", "mode": "auto", "default_view": "dashboard"}
	}`

	cfg, err := ParseConfig([]byte(old))
	if err != nil {
		t.Fatalf("ParseConfig on old prefs: %v", err)
	}
	// Unknown fields (k8s) are dropped by the decoder, not an error.
	if cfg.User.Name != "dana" {
		t.Errorf("user.name = %q, want dana", cfg.User.Name)
	}
	if cfg.Providers.DefaultModel != "" {
		t.Errorf("default_model should be empty on an old blob, got %q", cfg.Providers.DefaultModel)
	}

	// FillDefaults upgrades the version and fills the new fields' zero values.
	cfg.FillDefaults()
	if cfg.Version != ConfigVersion {
		t.Errorf("version = %d after FillDefaults, want %d", cfg.Version, ConfigVersion)
	}
	if valErr := cfg.Validate(); valErr != nil {
		t.Fatalf("upgraded config failed Validate: %v", valErr)
	}

	// Re-serialize: the dropped k8s field must not reappear, and the new
	// sections round-trip.
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal upgraded config: %v", err)
	}
	if strings.Contains(string(data), "k8s") {
		t.Errorf("re-serialized config still carries k8s: %s", data)
	}
	reparsed, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("re-parse upgraded config: %v", err)
	}
	if reparsed.Version != ConfigVersion {
		t.Errorf("reparsed version = %d, want %d", reparsed.Version, ConfigVersion)
	}
}

// TestFillDefaultsBackfillsDockerAndProviders verifies FillDefaults repairs
// a config whose runtime.default is "docker" but which carries no docker
// block and no provider entries — the shape an older or hand-edited
// prefs.json can take. Without the backfill, agents would spawn with an
// empty image / zero limits and Validate would reject the empty provider map.
func TestFillDefaultsBackfillsDockerAndProviders(t *testing.T) {
	partial := `{
		"version": 3,
		"providers": {"default": "claude"},
		"runtime": {"default": "docker"},
		"storage": {"default": "sqlite", "sqlite": {"path": ".mycel"}},
		"ui": {"theme": "dark", "mode": "auto", "default_view": "dashboard"}
	}`

	cfg, err := ParseConfig([]byte(partial))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Runtime.Docker.Image != "" || cfg.Runtime.Docker.CPUs != 0 {
		t.Fatalf("precondition: expected empty docker block, got %+v", cfg.Runtime.Docker)
	}

	cfg.FillDefaults()
	d := DefaultConfig()

	if cfg.Runtime.Docker.Image != d.Runtime.Docker.Image {
		t.Errorf("docker.image = %q, want %q", cfg.Runtime.Docker.Image, d.Runtime.Docker.Image)
	}
	if cfg.Runtime.Docker.Network != d.Runtime.Docker.Network {
		t.Errorf("docker.network = %q, want %q", cfg.Runtime.Docker.Network, d.Runtime.Docker.Network)
	}
	if cfg.Runtime.Docker.CPUs != d.Runtime.Docker.CPUs {
		t.Errorf("docker.cpus = %v, want %v", cfg.Runtime.Docker.CPUs, d.Runtime.Docker.CPUs)
	}
	if cfg.Runtime.Docker.MemoryMB != d.Runtime.Docker.MemoryMB {
		t.Errorf("docker.memory_mb = %v, want %v", cfg.Runtime.Docker.MemoryMB, d.Runtime.Docker.MemoryMB)
	}
	if len(cfg.Providers.Providers) == 0 {
		t.Error("providers map should be seeded when empty")
	}
	// The repaired config must pass Validate (previously ErrDefaultProviderNotFound).
	if err := cfg.Validate(); err != nil {
		t.Fatalf("repaired config failed Validate: %v", err)
	}
}

// TestConfigNotificationsRoundTrip verifies the new notifications section
// survives a marshal/unmarshal cycle.
func TestConfigNotificationsRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Notifications = NotificationsConfig{DefaultChannel: "slack:general", Enabled: true}
	cfg.Providers.DefaultModel = "claude-sonnet-4"

	data, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if parsed.Notifications.DefaultChannel != "slack:general" || !parsed.Notifications.Enabled {
		t.Errorf("notifications round-trip mismatch: %+v", parsed.Notifications)
	}
	if parsed.Providers.DefaultModel != "claude-sonnet-4" {
		t.Errorf("default_model round-trip mismatch: %q", parsed.Providers.DefaultModel)
	}
}

// TestConfigAppsAbsent verifies configs without an apps section parse to
// an empty map-safe zero value.
func TestConfigAppsAbsent(t *testing.T) {
	parsed, err := ParseConfig([]byte(`{"version": 2}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(parsed.Apps) != 0 {
		t.Errorf("expected no apps, got %d", len(parsed.Apps))
	}
}
