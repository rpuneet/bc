package gateway

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
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
		{"My Server:general", "my-server:general"},
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

// fakeChannelStore records SaveChannel/UpsertChannelMeta calls in memory.
type fakeChannelStore struct {
	saved map[string]PersistedChannel
	mu    sync.Mutex
}

func newFakeChannelStore() *fakeChannelStore {
	return &fakeChannelStore{saved: make(map[string]PersistedChannel)}
}

func (f *fakeChannelStore) SaveChannel(_ context.Context, bcChannel, platform, platformID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := f.saved[bcChannel]
	ch.BCChannel, ch.Platform, ch.PlatformID = bcChannel, platform, platformID
	f.saved[bcChannel] = ch
	return nil
}

func (f *fakeChannelStore) LoadChannels(_ context.Context) ([]PersistedChannel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]PersistedChannel, 0, len(f.saved))
	for _, ch := range f.saved {
		out = append(out, ch)
	}
	return out, nil
}

func (f *fakeChannelStore) UpsertChannelMeta(_ context.Context, bcChannel, displayName, kind string, participantCount int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := f.saved[bcChannel]
	ch.BCChannel = bcChannel
	if displayName != "" {
		ch.DisplayName = displayName
	}
	if kind != "" {
		ch.Kind = kind
	}
	if participantCount != 0 {
		ch.ParticipantCount = participantCount
	}
	f.saved[bcChannel] = ch
	return nil
}

func (f *fakeChannelStore) UpdateChannelPlatformID(_ context.Context, bcChannel, platformID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.saved[bcChannel]; ok {
		c.PlatformID = platformID
		f.saved[bcChannel] = c
	}
	return nil
}

func (f *fakeChannelStore) get(bcChannel string) (PersistedChannel, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch, ok := f.saved[bcChannel]
	return ch, ok
}

// identityAdapter is a mock adapter that also implements ChannelIdentity.
type identityAdapter struct {
	meta map[string]ChannelMeta
	mockNotifAdapter
	channels []ChannelInfo
}

func (a *identityAdapter) Channels() []ChannelInfo { return a.channels }

func (a *identityAdapter) ResolveChannel(_ context.Context, platformID string) (ChannelMeta, error) {
	m, ok := a.meta[platformID]
	if !ok {
		return ChannelMeta{}, fmt.Errorf("unresolvable: %s", platformID)
	}
	return m, nil
}

// waitFor polls until cond returns true or the deadline passes.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within deadline")
}

// TestDiscoverChannels_PersistsResolvedMeta asserts that discovery stores
// identity-resolved metadata for adapters implementing ChannelIdentity.
func TestDiscoverChannels_PersistsResolvedMeta(t *testing.T) {
	store := newFakeChannelStore()
	m := NewManager()
	m.SetChannelStore(store)

	a := &identityAdapter{
		mockNotifAdapter: mockNotifAdapter{name: "whatsapp"},
		channels:         []ChannelInfo{{ID: "1234@g.us", Name: "family", Platform: "whatsapp"}},
		meta: map[string]ChannelMeta{
			"1234@g.us": {DisplayName: "Family Group", Kind: ChannelKindGroup, ParticipantCount: 12},
		},
	}
	m.Register(a)
	m.discoverChannels(a)

	waitFor(t, func() bool {
		ch, ok := store.get("whatsapp:family")
		return ok && ch.DisplayName == "Family Group" && ch.Kind == "group" && ch.ParticipantCount == 12
	})
}

// TestDiscoverChannels_FallbackMetaFromDiscovery asserts that adapters
// without ChannelIdentity still persist discovery-time name/kind.
func TestDiscoverChannels_FallbackMetaFromDiscovery(t *testing.T) {
	store := newFakeChannelStore()
	m := NewManager()
	m.SetChannelStore(store)

	a := &identityAdapter{
		mockNotifAdapter: mockNotifAdapter{name: "slack"},
		channels:         []ChannelInfo{{ID: "C01", Name: "general", Platform: "slack", Kind: ChannelKindChannel}},
		meta:             map[string]ChannelMeta{}, // resolution always fails → fallback
	}
	m.Register(a)
	m.discoverChannels(a)

	waitFor(t, func() bool {
		ch, ok := store.get("slack:general")
		return ok && ch.DisplayName == "general" && ch.Kind == "channel"
	})
}

// TestHandleNotification_NativeChannelIDAndMeta asserts that an inbound
// notification carrying a platform-native channel id maps the route with
// that id and persists resolved metadata.
func TestHandleNotification_NativeChannelIDAndMeta(t *testing.T) {
	store := newFakeChannelStore()
	m := NewManager()
	m.SetChannelStore(store)

	a := &identityAdapter{
		mockNotifAdapter: mockNotifAdapter{name: "whatsapp"},
		meta: map[string]ChannelMeta{
			"1234@g.us": {DisplayName: "Family Group", Kind: ChannelKindGroup, ParticipantCount: 4},
		},
	}
	m.Register(a)

	m.handleNotification("whatsapp", Notification{
		Channel:   "family",
		ChannelID: "1234@g.us",
		Platform:  "whatsapp",
		Sender:    "alice",
		Content:   "hi",
	})

	m.mu.RLock()
	route, ok := m.channelMap["whatsapp:family"]
	m.mu.RUnlock()
	if !ok {
		t.Fatal("channel not mapped")
	}
	if route.ChannelID != "1234@g.us" {
		t.Fatalf("route ChannelID = %q, want native JID", route.ChannelID)
	}

	waitFor(t, func() bool {
		ch, ok := store.get("whatsapp:family")
		return ok && ch.PlatformID == "1234@g.us" && ch.DisplayName == "Family Group"
	})
}

// TestHandleNotification_UpgradesFallbackRoute asserts that a route created
// with a fallback id is upgraded once a native channel id arrives.
func TestHandleNotification_UpgradesFallbackRoute(t *testing.T) {
	m := NewManager()
	a := &identityAdapter{mockNotifAdapter: mockNotifAdapter{name: "whatsapp"}}
	m.Register(a)

	// Restored/legacy route with a name-based fallback id.
	m.mu.Lock()
	m.channelMap["whatsapp:family"] = channelRoute{Platform: "whatsapp", ChannelID: "family", Adapter: a}
	m.mu.Unlock()

	m.handleNotification("whatsapp", Notification{
		Channel:   "family",
		ChannelID: "1234@g.us",
		Platform:  "whatsapp",
	})

	m.mu.RLock()
	route := m.channelMap["whatsapp:family"]
	m.mu.RUnlock()
	if route.ChannelID != "1234@g.us" {
		t.Fatalf("route not upgraded: ChannelID = %q", route.ChannelID)
	}
}

// TestRefreshChannelMeta re-resolves and persists metadata for all channels
// whose adapter implements ChannelIdentity.
func TestRefreshChannelMeta(t *testing.T) {
	store := newFakeChannelStore()
	m := NewManager()
	m.SetChannelStore(store)

	wa := &identityAdapter{
		mockNotifAdapter: mockNotifAdapter{name: "whatsapp"},
		meta: map[string]ChannelMeta{
			"1234@g.us":                  {DisplayName: "Family Group", Kind: ChannelKindGroup, ParticipantCount: 12},
			"14155551234@s.whatsapp.net": {DisplayName: "Alice", Kind: ChannelKindPerson},
		},
	}
	plain := &mockNotifAdapter{name: "slack"}

	m.mu.Lock()
	m.channelMap["whatsapp:family"] = channelRoute{Platform: "whatsapp", ChannelID: "1234@g.us", Adapter: wa}
	m.channelMap["whatsapp:alice"] = channelRoute{Platform: "whatsapp", ChannelID: "14155551234@s.whatsapp.net", Adapter: wa}
	m.channelMap["whatsapp:unknown"] = channelRoute{Platform: "whatsapp", ChannelID: "nope", Adapter: wa}
	m.channelMap["slack:general"] = channelRoute{Platform: "slack", ChannelID: "C01", Adapter: plain}
	m.mu.Unlock()

	n, err := m.RefreshChannelMeta(context.Background())
	if err != nil {
		t.Fatalf("RefreshChannelMeta: %v", err)
	}
	if n != 2 {
		t.Fatalf("refreshed = %d, want 2", n)
	}
	if ch, _ := store.get("whatsapp:family"); ch.DisplayName != "Family Group" || ch.ParticipantCount != 12 {
		t.Fatalf("family meta not refreshed: %+v", ch)
	}
	if ch, _ := store.get("whatsapp:alice"); ch.DisplayName != "Alice" || ch.Kind != "person" {
		t.Fatalf("alice meta not refreshed: %+v", ch)
	}
}

// blockingAdapter Start blocks until ctx is cancelled — models a real poll loop.
type blockingAdapter struct {
	mockNotifAdapter
	started chan struct{}
}

func (b *blockingAdapter) Start(ctx context.Context, _ func(Notification)) error {
	close(b.started)
	<-ctx.Done()
	return nil
}

func TestStartAdapterHotStart(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.SetStartContext(ctx)

	// Boot with zero adapters (the empty-manager path).
	done := make(chan struct{})
	go func() {
		_ = m.Start(ctx)
		close(done)
	}()

	// Give Start a moment to park on ctx.Done.
	time.Sleep(20 * time.Millisecond)

	started := make(chan struct{})
	adapter := &blockingAdapter{
		mockNotifAdapter: mockNotifAdapter{name: "telegram"},
		started:          started,
	}
	if err := m.StartAdapter(adapter); err != nil {
		t.Fatalf("StartAdapter: %v", err)
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("adapter Start was not invoked")
	}

	if got := m.GetAdapter("telegram"); got == nil {
		t.Fatal("expected telegram adapter registered")
	}

	// Idempotent: second StartAdapter must not error.
	if err := m.StartAdapter(adapter); err != nil {
		t.Fatalf("second StartAdapter: %v", err)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Manager.Start did not return after cancel")
	}
}

func TestStartAdapterRequiresContext(t *testing.T) {
	m := NewManager()
	err := m.StartAdapter(&mockNotifAdapter{name: "telegram"})
	if err == nil {
		t.Fatal("expected error when manager has no start context")
	}
}
