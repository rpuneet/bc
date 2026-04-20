package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/bc/pkg/log"
)

// PersistedChannel is a saved bc_channel → platform_id mapping.
type PersistedChannel struct {
	BCChannel  string
	Platform   string
	PlatformID string
}

// ChannelStore persists channel mappings so they survive server restarts.
// Implemented by notify.Store via a wrapper.
type ChannelStore interface {
	SaveChannel(ctx context.Context, bcChannel, platform, platformID string) error
	LoadChannels(ctx context.Context) ([]PersistedChannel, error)
}

// messageSender is checked at runtime for adapters that support outbound messaging.
type messageSender interface {
	Send(ctx context.Context, channelID, sender, content string) error
}

// fileSender is checked at runtime for adapters that support file uploads.
type fileSender interface {
	SendFile(ctx context.Context, channelID, sender, filename string, data []byte, mimeType string) error
}

// Manager orchestrates all gateway adapters and routes messages.
type Manager struct {
	// adapters holds all registered NotificationAdapter instances.
	adapters map[string]NotificationAdapter
	// channelMap maps "telegram:<group_name>" → channelRoute
	channelMap map[string]channelRoute
	// onInbound is called when a message arrives from an external platform.
	// Typically wired to ChannelService.Send + SSE hub.
	onInbound    func(bcChannel, sender, content string, raw json.RawMessage)
	channelStore ChannelStore
	mu           sync.RWMutex
}

type channelRoute struct {
	Adapter   NotificationAdapter
	Platform  string
	ChannelID string
}

// NewManager creates a new gateway manager.
func NewManager() *Manager {
	return &Manager{
		adapters:   make(map[string]NotificationAdapter),
		channelMap: make(map[string]channelRoute),
	}
}

// SetChannelStore sets the persistence store for channel mappings.
func (m *Manager) SetChannelStore(store ChannelStore) {
	m.channelStore = store
}

// SetInboundHandler sets the callback for inbound messages from external platforms.
func (m *Manager) SetInboundHandler(fn func(bcChannel, sender, content string, raw json.RawMessage)) {
	m.onInbound = fn
}

// Register adds a NotificationAdapter to the manager.
func (m *Manager) Register(adapter NotificationAdapter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.adapters[adapter.Name()] = adapter
}

// Start discovers channels from all adapters and begins receiving messages.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.RLock()
	adapterList := make([]NotificationAdapter, 0, len(m.adapters))
	for _, a := range m.adapters {
		adapterList = append(adapterList, a)
	}
	m.mu.RUnlock()

	// Restore persisted channel mappings so Send works immediately after restart.
	m.restorePersistedChannels(ctx)

	// Discover channels from all adapters
	for _, a := range adapterList {
		m.discoverChannels(a)
	}

	// Start all adapters in goroutines
	var wg sync.WaitGroup
	for _, a := range adapterList {
		wg.Add(1)
		go func(adapter NotificationAdapter) {
			defer wg.Done()
			platformName := adapter.Name()
			handler := func(n Notification) {
				m.handleNotification(platformName, n)
			}
			if err := adapter.Start(ctx, handler); err != nil && ctx.Err() == nil {
				log.Error("gateway: adapter stopped with error", "adapter", adapter.Name(), "error", err)
			}
		}(a)
	}

	// Re-discover channels after adapters have connected (5s delay)
	go m.lateDiscovery(ctx)

	<-ctx.Done()
	wg.Wait()
	return nil
}

// restorePersistedChannels loads saved channel mappings from the store.
func (m *Manager) restorePersistedChannels(ctx context.Context) {
	if m.channelStore == nil {
		return
	}
	saved, err := m.channelStore.LoadChannels(ctx)
	if err != nil {
		log.Warn("gateway: failed to load persisted channels", "error", err)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range saved {
		if _, exists := m.channelMap[ch.BCChannel]; exists {
			continue
		}
		if a, ok := m.adapters[ch.Platform]; ok {
			m.channelMap[ch.BCChannel] = channelRoute{
				Platform:  ch.Platform,
				ChannelID: ch.PlatformID,
				Adapter:   a,
			}
			log.Info("gateway: restored channel", "bc_channel", ch.BCChannel, "platform_id", ch.PlatformID)
		}
	}
}

// discoverChannels discovers channels from an adapter.
func (m *Manager) discoverChannels(a NotificationAdapter) {
	channels := a.Channels()
	type discovered struct{ bc, platform, id string }
	toPersist := make([]discovered, 0, len(channels))
	m.mu.Lock()
	for _, ch := range channels {
		bcName := a.Name() + ":" + sanitizeChannelName(ch.Name)
		m.channelMap[bcName] = channelRoute{
			Platform:  a.Name(),
			ChannelID: ch.ID,
			Adapter:   a,
		}
		toPersist = append(toPersist, discovered{bcName, a.Name(), ch.ID})
		log.Info("gateway: discovered channel", "bc_channel", bcName, "platform_id", ch.ID)
	}
	m.mu.Unlock()
	for _, d := range toPersist {
		m.persistChannel(d.bc, d.platform, d.id)
	}
}

// lateDiscovery re-discovers channels after adapters have had time to connect.
func (m *Manager) lateDiscovery(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Second):
	}

	m.mu.RLock()
	adapterList := make([]NotificationAdapter, 0, len(m.adapters))
	for _, a := range m.adapters {
		adapterList = append(adapterList, a)
	}
	m.mu.RUnlock()

	for _, a := range adapterList {
		channels := a.Channels()
		type lateDiscovered struct{ bc, platform, id string }
		var latePersist []lateDiscovered
		m.mu.Lock()
		for _, ch := range channels {
			bcName := a.Name() + ":" + sanitizeChannelName(ch.Name)
			if _, exists := m.channelMap[bcName]; !exists {
				m.channelMap[bcName] = channelRoute{
					Platform:  a.Name(),
					ChannelID: ch.ID,
					Adapter:   a,
				}
				latePersist = append(latePersist, lateDiscovered{bcName, a.Name(), ch.ID})
				log.Info("gateway: late-discovered channel", "bc_channel", bcName, "platform_id", ch.ID)
			}
		}
		m.mu.Unlock()
		for _, d := range latePersist {
			m.persistChannel(d.bc, d.platform, d.id)
		}
	}
}

// GetAdapter returns a registered adapter by name, or nil if not found.
func (m *Manager) GetAdapter(name string) NotificationAdapter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.adapters[name]
}

// AdapterStatus returns the connection status for a specific adapter.
func (m *Manager) AdapterStatus(platform string) AdapterStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	a, ok := m.adapters[platform]
	if !ok {
		return AdapterStatus{Error: "adapter not registered"}
	}
	return a.Status()
}

// Stop gracefully shuts down all adapters.
func (m *Manager) Stop(_ context.Context) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, a := range m.adapters {
		if err := a.Stop(); err != nil {
			log.Warn("gateway: stop error", "adapter", a.Name(), "error", err)
		}
	}
}

// Send routes a message from a bc channel to the appropriate external platform.
// Returns true if the channel is an external gateway channel and was handled.
func (m *Manager) Send(ctx context.Context, bcChannel, sender, content string) (bool, error) {
	m.mu.RLock()
	route, ok := m.channelMap[bcChannel]
	m.mu.RUnlock()
	if !ok {
		return false, nil // not a gateway channel
	}

	if route.Adapter == nil {
		return true, fmt.Errorf("gateway send to %s: no adapter", bcChannel)
	}

	ms, ok := route.Adapter.(messageSender)
	if !ok {
		return true, fmt.Errorf("gateway send to %s: adapter does not support outbound messaging", bcChannel)
	}

	if err := ms.Send(ctx, route.ChannelID, sender, content); err != nil {
		return true, fmt.Errorf("gateway send to %s: %w", bcChannel, err)
	}
	return true, nil
}

// SendFile uploads a file to a gateway channel. Returns false if the channel
// is not a gateway channel or the adapter doesn't support file uploads.
func (m *Manager) SendFile(ctx context.Context, bcChannel, sender, filename string, data []byte, mimeType string) (bool, error) {
	m.mu.RLock()
	route, ok := m.channelMap[bcChannel]
	m.mu.RUnlock()
	if !ok {
		return false, nil
	}

	if route.Adapter == nil {
		return true, fmt.Errorf("gateway %s: no adapter", bcChannel)
	}

	fs, ok := route.Adapter.(fileSender)
	if !ok {
		return true, fmt.Errorf("gateway %s does not support file uploads", bcChannel)
	}

	if err := fs.SendFile(ctx, route.ChannelID, sender, filename, data, mimeType); err != nil {
		return true, fmt.Errorf("gateway send file to %s: %w", bcChannel, err)
	}
	return true, nil
}

// IsGatewayChannel returns true if the channel name belongs to an external gateway.
func (m *Manager) IsGatewayChannel(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.channelMap[name]
	return ok
}

// SeedChannel adds a known gateway channel to the channel map.
// Used on startup to restore mappings for channels that were dynamically
// discovered in previous sessions. The channelID is set to the channel name
// suffix (e.g., "all-bc" for "slack:all-bc") since the platform adapter
// will resolve it.
//
// Channel names follow the pattern "adapter_name:channel_name" where
// adapter_name may itself contain a colon (e.g. "telegram:foo:general").
func (m *Manager) SeedChannel(bcChannel string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Don't overwrite existing mappings (from adapter discovery)
	if _, exists := m.channelMap[bcChannel]; exists {
		return
	}

	// Find the registered adapter whose name is a prefix of bcChannel.
	// Try longest match first to handle "telegram:foo" before "telegram".
	var bestAdapter NotificationAdapter
	var bestPlatform string

	for name, a := range m.adapters {
		prefix := name + ":"
		if strings.HasPrefix(bcChannel, prefix) && len(name) > len(bestPlatform) {
			bestAdapter = a
			bestPlatform = name
		}
	}

	if bestPlatform == "" {
		return
	}

	channelSuffix := bcChannel[len(bestPlatform)+1:]
	m.channelMap[bcChannel] = channelRoute{
		Platform:  bestPlatform,
		ChannelID: channelSuffix, // will be resolved by adapter on first send
		Adapter:   bestAdapter,
	}
	log.Info("gateway: seeded channel from store", "bc_channel", bcChannel, "platform", bestPlatform)
}

// persistChannel saves a channel mapping to the store (non-blocking, best-effort).
func (m *Manager) persistChannel(bcChannel, platform, platformID string) {
	if m.channelStore == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.channelStore.SaveChannel(ctx, bcChannel, platform, platformID); err != nil {
			log.Warn("gateway: failed to persist channel", "channel", bcChannel, "error", err)
		}
	}()
}

// DiscoveredSources returns all discovered external channels.
func (m *Manager) DiscoveredSources() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.channelMap))
	for name := range m.channelMap {
		names = append(names, name)
	}
	return names
}

// handleNotification processes an event from a NotificationAdapter into bc.
func (m *Manager) handleNotification(platform string, n Notification) {
	channelName := n.Channel
	if channelName == "" {
		channelName = "default"
	}
	bcChannel := platform + ":" + sanitizeChannelName(channelName)

	// Ensure channel is in the map
	m.mu.Lock()
	if _, exists := m.channelMap[bcChannel]; !exists {
		adapter := m.adapters[platform]
		m.channelMap[bcChannel] = channelRoute{
			Platform:  platform,
			ChannelID: channelName,
			Adapter:   adapter,
		}
		log.Info("gateway: dynamically mapped notification channel", "bc_channel", bcChannel, "platform", platform)
	}
	m.mu.Unlock()
	m.persistChannel(bcChannel, platform, channelName)

	sender := fmt.Sprintf("[%s] %s", platform, n.Sender)

	// Use the adapter-extracted text content for display; fall back to raw JSON
	content := n.Content
	if content == "" {
		content = string(n.Raw)
	}
	if m.onInbound != nil {
		m.onInbound(bcChannel, sender, content, n.Raw)
	}
}

// WebhookHandlers returns HTTP handlers for all webhook-type adapters,
// keyed by adapter name. The server mounts these at /hooks/{name}.
func (m *Manager) WebhookHandlers() map[string]http.Handler {
	handlers := make(map[string]http.Handler)
	for name, a := range m.adapters {
		if h := a.HTTPHandler(); h != nil {
			handlers[name] = h
		}
	}
	return handlers
}

// sanitizeChannelName converts a group name to a valid bc channel name.
func sanitizeChannelName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	// Remove any characters that aren't alphanumeric, dash, or underscore
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
