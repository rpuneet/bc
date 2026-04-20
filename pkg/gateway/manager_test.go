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

func TestManagerDiscoveredSources(t *testing.T) {
	m := NewManager()
	if len(m.DiscoveredSources()) != 0 {
		t.Error("expected empty list")
	}

	m.channelMap["telegram:marketing"] = channelRoute{Platform: "telegram"}
	m.channelMap["slack:general"] = channelRoute{Platform: "slack"}

	channels := m.DiscoveredSources()
	if len(channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(channels))
	}
}

// mockNotifAdapter is a minimal NotificationAdapter for testing registration.
type mockNotifAdapter struct {
	name string
}

func (m *mockNotifAdapter) Name() string                                        { return m.name }
func (m *mockNotifAdapter) Type() AdapterType                                   { return AdapterSocket }
func (m *mockNotifAdapter) Start(_ context.Context, _ func(Notification)) error { return nil }
func (m *mockNotifAdapter) Stop() error                                         { return nil }
func (m *mockNotifAdapter) HTTPHandler() http.Handler                           { return nil }
func (m *mockNotifAdapter) Channels() []ChannelInfo                             { return nil }
func (m *mockNotifAdapter) Status() AdapterStatus                               { return AdapterStatus{} }

func TestRegisterMultipleAdapters(t *testing.T) {
	m := NewManager()
	m.Register(&mockNotifAdapter{name: "telegram:trade"})
	m.Register(&mockNotifAdapter{name: "telegram:gateway"})
	m.Register(&mockNotifAdapter{name: "telegram:kognivida"})

	if len(m.adapters) != 3 {
		t.Errorf("expected 3 adapters, got %d", len(m.adapters))
	}
	for _, name := range []string{"telegram:trade", "telegram:gateway", "telegram:kognivida"} {
		if _, ok := m.adapters[name]; !ok {
			t.Errorf("adapter %q not registered", name)
		}
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
