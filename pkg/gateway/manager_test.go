package gateway

import (
	"context"
	"net/http"
	"testing"
)

func TestSanitizeChannelName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Marketing", "marketing"},
		{"All BC Infra", "all-bc-infra"},
		{"dev-chat", "dev-chat"},
		{"hello_world", "hello_world"},
		{"café ☕", "caf-"},
		{"UPPER CASE", "upper-case"},
		{"a/b\\c", "abc"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeChannelName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeChannelName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		want  string
		n     int
	}{
		{"hello", "hello", 10},
		{"hello world", "hello...", 5},
		{"", "", 5},
		{"abc", "abc", 3},
		{"abcd", "abc...", 3},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Truncate(tt.input, tt.n)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
			}
		})
	}
}

func TestManagerIsGatewayChannel(t *testing.T) {
	m := NewManager()
	if m.IsGatewayChannel("telegram:marketing") {
		t.Error("expected false for unknown channel")
	}

	m.channelMap["telegram:marketing"] = channelRoute{Platform: "telegram", ChannelID: "123"}
	if !m.IsGatewayChannel("telegram:marketing") {
		t.Error("expected true for known channel")
	}
}

func TestManagerExternalChannels(t *testing.T) {
	m := NewManager()
	if len(m.ExternalChannels()) != 0 {
		t.Error("expected empty list")
	}

	m.channelMap["telegram:marketing"] = channelRoute{Platform: "telegram"}
	m.channelMap["slack:general"] = channelRoute{Platform: "slack"}

	channels := m.ExternalChannels()
	if len(channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(channels))
	}
}

// mockLegacyAdapter is a minimal legacy Adapter for testing registration.
type mockLegacyAdapter struct {
	name string
}

func (m *mockLegacyAdapter) Name() string                                          { return m.name }
func (m *mockLegacyAdapter) Start(_ context.Context, _ func(InboundMessage)) error { return nil }
func (m *mockLegacyAdapter) Stop(_ context.Context) error                          { return nil }
func (m *mockLegacyAdapter) Send(_ context.Context, _, _, _ string) error          { return nil }
func (m *mockLegacyAdapter) Channels(_ context.Context) ([]ExternalChannel, error) { return nil, nil }
func (m *mockLegacyAdapter) Health(_ context.Context) error                        { return nil }

// mockNotifAdapter is a minimal NotificationAdapter for testing registration.
type mockNotifAdapter struct {
	name string
}

func (m *mockNotifAdapter) Name() string                                              { return m.name }
func (m *mockNotifAdapter) Type() AdapterType                                         { return AdapterSocket }
func (m *mockNotifAdapter) Start(_ context.Context, _ func(Notification)) error       { return nil }
func (m *mockNotifAdapter) Stop() error                                               { return nil }
func (m *mockNotifAdapter) HTTPHandler() http.Handler                                 { return nil }
func (m *mockNotifAdapter) Channels() []ChannelInfo                                   { return nil }
func (m *mockNotifAdapter) Status() AdapterStatus                                     { return AdapterStatus{} }

func TestSeedChannelMultiColonPlatform(t *testing.T) {
	m := NewManager()
	// Register two adapters: "telegram" and "telegram:foo"
	m.Register(&mockNotifAdapter{name: "telegram"})
	m.Register(&mockNotifAdapter{name: "telegram:foo"})

	// Seed a channel for the labeled adapter
	m.SeedChannel("telegram:foo:general")
	route, ok := m.channelMap["telegram:foo:general"]
	if !ok {
		t.Fatal("expected channel to be seeded")
	}
	if route.Platform != "telegram:foo" {
		t.Errorf("expected platform telegram:foo, got %s", route.Platform)
	}
	if route.ChannelID != "general" {
		t.Errorf("expected channelID general, got %s", route.ChannelID)
	}

	// Seed a channel for the plain adapter
	m.SeedChannel("telegram:marketing")
	route, ok = m.channelMap["telegram:marketing"]
	if !ok {
		t.Fatal("expected channel to be seeded")
	}
	if route.Platform != "telegram" {
		t.Errorf("expected platform telegram, got %s", route.Platform)
	}
	if route.ChannelID != "marketing" {
		t.Errorf("expected channelID marketing, got %s", route.ChannelID)
	}
}

func TestSeedChannelNoOverwrite(t *testing.T) {
	m := NewManager()
	m.Register(&mockNotifAdapter{name: "slack"})
	m.channelMap["slack:general"] = channelRoute{Platform: "slack", ChannelID: "C123"}

	// SeedChannel should not overwrite existing mapping
	m.SeedChannel("slack:general")
	if m.channelMap["slack:general"].ChannelID != "C123" {
		t.Error("SeedChannel overwrote existing mapping")
	}
}

func TestRegisterMultipleNotificationAdapters(t *testing.T) {
	m := NewManager()
	m.Register(&mockNotifAdapter{name: "telegram:trade"})
	m.Register(&mockNotifAdapter{name: "telegram:gateway"})
	m.Register(&mockNotifAdapter{name: "telegram:kognivida"})

	if len(m.notificationAdapters) != 3 {
		t.Errorf("expected 3 notification adapters, got %d", len(m.notificationAdapters))
	}
	for _, name := range []string{"telegram:trade", "telegram:gateway", "telegram:kognivida"} {
		if _, ok := m.notificationAdapters[name]; !ok {
			t.Errorf("notification adapter %q not registered", name)
		}
	}
}

func TestRegisterLegacyAdapter(t *testing.T) {
	m := NewManager()
	m.Register(&mockLegacyAdapter{name: "slack"})

	if len(m.legacyAdapters) != 1 {
		t.Errorf("expected 1 legacy adapter, got %d", len(m.legacyAdapters))
	}
	if _, ok := m.legacyAdapters["slack"]; !ok {
		t.Error("legacy adapter 'slack' not registered")
	}
}

func TestRegisterMixedAdapters(t *testing.T) {
	m := NewManager()
	m.Register(&mockLegacyAdapter{name: "legacy-platform"})
	m.Register(&mockNotifAdapter{name: "new-platform"})

	if len(m.legacyAdapters) != 1 {
		t.Errorf("expected 1 legacy adapter, got %d", len(m.legacyAdapters))
	}
	if len(m.notificationAdapters) != 1 {
		t.Errorf("expected 1 notification adapter, got %d", len(m.notificationAdapters))
	}
}

func TestAdapterStatusNotificationAdapter(t *testing.T) {
	m := NewManager()
	m.Register(&mockNotifAdapter{name: "discord"})

	status := m.AdapterStatus("discord")
	// mockNotifAdapter returns empty status (Connected: false)
	if status.Connected {
		t.Error("expected not connected for mock adapter")
	}
	if status.Error != "" {
		t.Errorf("expected no error, got %q", status.Error)
	}
}

func TestAdapterStatusUnknown(t *testing.T) {
	m := NewManager()
	status := m.AdapterStatus("unknown")
	if status.Error != "adapter not registered" {
		t.Errorf("expected 'adapter not registered' error, got %q", status.Error)
	}
}

func TestSeedChannelMixedAdapters(t *testing.T) {
	m := NewManager()
	m.Register(&mockLegacyAdapter{name: "legacy"})
	m.Register(&mockNotifAdapter{name: "notif"})

	m.SeedChannel("legacy:test")
	route, ok := m.channelMap["legacy:test"]
	if !ok {
		t.Fatal("expected legacy channel to be seeded")
	}
	if route.LegacyAdapter == nil {
		t.Error("expected legacy adapter to be set")
	}
	if route.NotificationAdapter != nil {
		t.Error("expected notification adapter to be nil for legacy channel")
	}

	m.SeedChannel("notif:test")
	route, ok = m.channelMap["notif:test"]
	if !ok {
		t.Fatal("expected notif channel to be seeded")
	}
	if route.NotificationAdapter == nil {
		t.Error("expected notification adapter to be set")
	}
	if route.LegacyAdapter != nil {
		t.Error("expected legacy adapter to be nil for notif channel")
	}
}
