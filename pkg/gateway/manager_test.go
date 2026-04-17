package gateway

import (
	"context"
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

// mockAdapter is a minimal Adapter for testing registration.
type mockAdapter struct {
	name string
}

func (m *mockAdapter) Name() string                                          { return m.name }
func (m *mockAdapter) Start(_ context.Context, _ func(InboundMessage)) error { return nil }
func (m *mockAdapter) Stop(_ context.Context) error                          { return nil }
func (m *mockAdapter) Send(_ context.Context, _, _, _ string) error          { return nil }
func (m *mockAdapter) Channels(_ context.Context) ([]ExternalChannel, error) { return nil, nil }
func (m *mockAdapter) Health(_ context.Context) error                        { return nil }

func TestSeedChannelMultiColonPlatform(t *testing.T) {
	m := NewManager()
	// Register two adapters: "telegram" and "telegram:foo"
	m.Register(&mockAdapter{name: "telegram"})
	m.Register(&mockAdapter{name: "telegram:foo"})

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
	m.Register(&mockAdapter{name: "slack"})
	m.channelMap["slack:general"] = channelRoute{Platform: "slack", ChannelID: "C123"}

	// SeedChannel should not overwrite existing mapping
	m.SeedChannel("slack:general")
	if m.channelMap["slack:general"].ChannelID != "C123" {
		t.Error("SeedChannel overwrote existing mapping")
	}
}

func TestRegisterMultipleAdapters(t *testing.T) {
	m := NewManager()
	m.Register(&mockAdapter{name: "telegram:trade"})
	m.Register(&mockAdapter{name: "telegram:gateway"})
	m.Register(&mockAdapter{name: "telegram:kognivida"})

	if len(m.adapters) != 3 {
		t.Errorf("expected 3 adapters, got %d", len(m.adapters))
	}
	for _, name := range []string{"telegram:trade", "telegram:gateway", "telegram:kognivida"} {
		if _, ok := m.adapters[name]; !ok {
			t.Errorf("adapter %q not registered", name)
		}
	}
}
