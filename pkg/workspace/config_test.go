package workspace

import (
	"encoding/json"
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
