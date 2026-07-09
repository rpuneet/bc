package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
)

// PersistedChannel is a saved bc_channel → platform_id mapping with
// optional display metadata.
type PersistedChannel struct {
	BCChannel        string
	Platform         string
	PlatformID       string
	DisplayName      string
	Kind             string
	ParticipantCount int
}

// ChannelStore persists channel mappings so they survive server restarts.
// Implemented by notify.Store via a wrapper.
type ChannelStore interface {
	SaveChannel(ctx context.Context, bcChannel, platform, platformID string) error
	LoadChannels(ctx context.Context) ([]PersistedChannel, error)
	UpsertChannelMeta(ctx context.Context, bcChannel, displayName, kind string, participantCount int) error
	// UpdateChannelPlatformID force-overwrites a channel's platform id —
	// used when a fallback route is upgraded to a native id (SaveChannel
	// deliberately preserves existing non-empty ids).
	UpdateChannelPlatformID(ctx context.Context, bcChannel, platformID string) error
}

// messageSender is checked at runtime for adapters that support outbound messaging.
type messageSender interface {
	Send(ctx context.Context, channelID, sender, content string) error
}

// fileSender is checked at runtime for adapters that support file uploads.
type fileSender interface {
	SendFile(ctx context.Context, channelID, sender, filename string, data []byte, mimeType string) error
}

// reactionSender is checked at runtime for adapters that support outbound reactions.
type reactionSender interface {
	SendReaction(ctx context.Context, channelID, senderJID, messageID, emoji string) error
}

// Manager orchestrates all gateway adapters and routes messages.
type Manager struct {
	// adapters holds all registered NotificationAdapter instances.
	adapters map[string]NotificationAdapter
	// running tracks adapters whose Start loop is already live so hot-start
	// does not spawn a second poll/socket for the same name.
	running map[string]bool
	// startCtx is the context passed to Start; used by StartAdapter to bind
	// late-registered adapters to the same lifetime.
	startCtx context.Context
	// channelMap maps "telegram:<group_name>" → channelRoute
	channelMap map[string]channelRoute
	// onInbound is called when a message arrives from an external platform.
	// Typically wired to ChannelService.Send + SSE hub.
	// senderID carries the platform-native sender identifier (e.g. WhatsApp JID)
	// so callers can use it for follow-up operations such as reactions.
	onInbound    func(bcChannel, sender, senderID, content, messageID string, mentions []string, raw json.RawMessage)
	channelStore ChannelStore
	mu           sync.RWMutex
	// adapterWG tracks boot-time and hot-started adapter goroutines so Stop/
	// Start can wait for clean shutdown of both.
	adapterWG sync.WaitGroup
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
		running:    make(map[string]bool),
		channelMap: make(map[string]channelRoute),
	}
}

// SetStartContext stores the process lifetime context used by StartAdapter
// before Start's goroutine runs. Without this, a PATCH that hot-starts an
// adapter can race Start and see a nil context.
func (m *Manager) SetStartContext(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCtx = ctx
	if m.running == nil {
		m.running = make(map[string]bool)
	}
}

// SetChannelStore sets the persistence store for channel mappings.
func (m *Manager) SetChannelStore(store ChannelStore) {
	m.channelStore = store
}

// SetInboundHandler sets the callback for inbound messages from external platforms.
// The callback receives the bc channel name, sender display name, platform-native
// sender id (e.g. WhatsApp JID — used for follow-up reactions), text content,
// platform-native message ID, pre-extracted mentions (e.g. WhatsApp JID user parts),
// and the raw platform payload.
func (m *Manager) SetInboundHandler(fn func(bcChannel, sender, senderID, content, messageID string, mentions []string, raw json.RawMessage)) {
	m.onInbound = fn
}

// Register adds a NotificationAdapter to the manager.
func (m *Manager) Register(adapter NotificationAdapter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.adapters[adapter.Name()] = adapter
}

// Start discovers channels from all adapters and begins receiving messages.
// It is safe to call with zero adapters; later StartAdapter calls share this
// context and wire into the same inbound pipeline.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.startCtx = ctx
	if m.running == nil {
		m.running = make(map[string]bool)
	}
	adapterList := make([]NotificationAdapter, 0, len(m.adapters))
	for _, a := range m.adapters {
		adapterList = append(adapterList, a)
	}
	m.mu.Unlock()

	// Restore persisted channel mappings so Send works immediately after restart.
	m.restorePersistedChannels(ctx)

	// Discover channels from all adapters
	for _, a := range adapterList {
		m.discoverChannels(a)
	}

	// Start all adapters in goroutines
	for _, a := range adapterList {
		if !m.markRunning(a.Name()) {
			continue // already started (e.g. via StartAdapter race)
		}
		m.adapterWG.Add(1)
		go func(adapter NotificationAdapter) {
			defer m.adapterWG.Done()
			defer m.clearRunning(adapter.Name())
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
	m.adapterWG.Wait()
	return nil
}

// StartAdapter registers and starts an adapter after Manager.Start is running.
// Used when a platform is connected via the API while the daemon is already up
// (previously required a full restart because buildGatewayManager returned nil
// with no adapters at boot, and PATCH only persisted config).
//
// If an adapter with the same name is already running, this is a no-op and
// returns nil. Callers that need to swap credentials should Stop the old
// adapter first (future work).
func (m *Manager) StartAdapter(adapter NotificationAdapter) error {
	if adapter == nil {
		return fmt.Errorf("gateway: nil adapter")
	}
	name := adapter.Name()
	if name == "" {
		return fmt.Errorf("gateway: adapter has empty name")
	}

	m.mu.RLock()
	ctx := m.startCtx
	already := m.running[name]
	m.mu.RUnlock()
	if ctx == nil {
		return fmt.Errorf("gateway: manager not started")
	}
	if already {
		log.Info("gateway: adapter already running", "name", name)
		return nil
	}

	m.Register(adapter)
	m.discoverChannels(adapter)

	if !m.markRunning(name) {
		return nil
	}
	m.adapterWG.Add(1)
	go func() {
		defer m.adapterWG.Done()
		defer m.clearRunning(name)
		handler := func(n Notification) {
			m.handleNotification(name, n)
		}
		if err := adapter.Start(ctx, handler); err != nil && ctx.Err() == nil {
			log.Error("gateway: adapter stopped with error", "adapter", name, "error", err)
		}
	}()
	log.Info("gateway: hot-started adapter", "name", name)
	return nil
}

// markRunning records that name is starting. Returns false if already running.
func (m *Manager) markRunning(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running == nil {
		m.running = make(map[string]bool)
	}
	if m.running[name] {
		return false
	}
	m.running[name] = true
	return true
}

func (m *Manager) clearRunning(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, name)
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
	type discovered struct {
		bc, platform, id string
		meta             ChannelMeta
	}
	toPersist := make([]discovered, 0, len(channels))
	m.mu.Lock()
	for _, ch := range channels {
		bcName := a.Name() + ":" + sanitizeChannelName(ch.Name)
		m.channelMap[bcName] = channelRoute{
			Platform:  a.Name(),
			ChannelID: ch.ID,
			Adapter:   a,
		}
		toPersist = append(toPersist, discovered{bcName, a.Name(), ch.ID, ChannelMeta{DisplayName: ch.Name, Kind: ch.Kind}})
		log.Info("gateway: discovered channel", "bc_channel", bcName, "platform_id", ch.ID)
	}
	m.mu.Unlock()
	for _, d := range toPersist {
		m.persistChannel(d.bc, d.platform, d.id)
		m.persistChannelMeta(a, d.bc, d.id, d.meta)
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
		type lateDiscovered struct {
			bc, platform, id string
			meta             ChannelMeta
		}
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
				latePersist = append(latePersist, lateDiscovered{bcName, a.Name(), ch.ID, ChannelMeta{DisplayName: ch.Name, Kind: ch.Kind}})
				log.Info("gateway: late-discovered channel", "bc_channel", bcName, "platform_id", ch.ID)
			}
		}
		m.mu.Unlock()
		for _, d := range latePersist {
			m.persistChannel(d.bc, d.platform, d.id)
			m.persistChannelMeta(a, d.bc, d.id, d.meta)
		}
	}

	// Backfill display metadata for restored channels that predate identity
	// resolution (rows persisted without a display_name). Runs synchronously
	// in this goroutine so platform lookups stay sequential — WhatsApp rate
	// limits info queries.
	m.resolveMissingMeta(ctx)
}

// resolveMissingMeta resolves display metadata for persisted channels that
// lack a display name, using adapters that implement ChannelIdentity.
func (m *Manager) resolveMissingMeta(ctx context.Context) {
	if m.channelStore == nil {
		return
	}
	saved, err := m.channelStore.LoadChannels(ctx)
	if err != nil {
		log.Warn("gateway: failed to load channels for meta backfill", "error", err)
		return
	}
	for _, ch := range saved {
		if ch.DisplayName != "" || ctx.Err() != nil {
			continue
		}
		m.mu.RLock()
		route, ok := m.channelMap[ch.BCChannel]
		m.mu.RUnlock()
		if !ok {
			continue
		}
		if _, isResolver := route.Adapter.(ChannelIdentity); !isResolver {
			continue
		}
		m.resolveAndStoreMeta(ctx, route.Adapter, ch.BCChannel, route.ChannelID, ChannelMeta{})
	}
}

// GetAdapter returns a registered adapter by name, or nil if not found.
func (m *Manager) GetAdapter(name string) NotificationAdapter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.adapters[name]
}

// AdapterNames returns the names of all registered adapters.
func (m *Manager) AdapterNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.adapters))
	for name := range m.adapters {
		names = append(names, name)
	}
	return names
}

// HandleNotification processes an inbound notification from a specific platform.
func (m *Manager) HandleNotification(platform string, n Notification) {
	m.handleNotification(platform, n)
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

// SendReaction routes an emoji reaction to a specific message in a gateway channel.
// senderJID is the platform-native id of the original message author (required by some
// platforms, e.g. WhatsApp); pass empty string for platforms that don't need it.
// Returns true if the channel is a gateway channel and the adapter handled the call.
func (m *Manager) SendReaction(ctx context.Context, bcChannel, senderJID, messageID, emoji string) (bool, error) {
	m.mu.RLock()
	route, ok := m.channelMap[bcChannel]
	m.mu.RUnlock()
	if !ok {
		return false, nil // not a gateway channel
	}

	if route.Adapter == nil {
		return true, fmt.Errorf("gateway react to %s: no adapter", bcChannel)
	}

	rs, ok := route.Adapter.(reactionSender)
	if !ok {
		return true, fmt.Errorf("gateway react to %s: adapter does not support reactions", bcChannel)
	}

	if err := rs.SendReaction(ctx, route.ChannelID, senderJID, messageID, emoji); err != nil {
		return true, fmt.Errorf("gateway react to %s: %w", bcChannel, err)
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

// persistChannelMeta resolves and saves display metadata for a channel
// (non-blocking, best-effort). If the adapter implements ChannelIdentity the
// resolved values win; otherwise the fallback (from discovery) is stored.
func (m *Manager) persistChannelMeta(a NotificationAdapter, bcChannel, platformID string, fallback ChannelMeta) {
	if m.channelStore == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		m.resolveAndStoreMeta(ctx, a, bcChannel, platformID, fallback)
	}()
}

// resolveAndStoreMeta synchronously resolves channel metadata and upserts it.
func (m *Manager) resolveAndStoreMeta(ctx context.Context, a NotificationAdapter, bcChannel, platformID string, fallback ChannelMeta) {
	meta := m.resolveChannelMeta(ctx, a, platformID, fallback)
	if meta == (ChannelMeta{}) {
		return
	}
	if err := m.channelStore.UpsertChannelMeta(ctx, bcChannel, meta.DisplayName, meta.Kind, meta.ParticipantCount); err != nil {
		log.Warn("gateway: failed to persist channel meta", "channel", bcChannel, "error", err)
	}
}

// resolveChannelMeta asks the adapter for channel identity, falling back to
// discovery-time metadata when the adapter cannot resolve.
func (m *Manager) resolveChannelMeta(ctx context.Context, a NotificationAdapter, platformID string, fallback ChannelMeta) ChannelMeta {
	meta := fallback
	if a == nil {
		return meta
	}
	resolver, ok := a.(ChannelIdentity)
	if !ok {
		return meta
	}
	resolved, err := resolver.ResolveChannel(ctx, platformID)
	if err != nil {
		log.Debug("gateway: channel identity resolution failed", "adapter", a.Name(), "platform_id", platformID, "error", err)
		return meta
	}
	if resolved.DisplayName != "" {
		meta.DisplayName = resolved.DisplayName
	}
	if resolved.Kind != "" {
		meta.Kind = resolved.Kind
	}
	if resolved.ParticipantCount > 0 {
		meta.ParticipantCount = resolved.ParticipantCount
	}
	return meta
}

// RefreshChannelMeta re-resolves display metadata for all known channels
// whose adapter implements ChannelIdentity, and persists the results.
// Returns the number of channels refreshed.
func (m *Manager) RefreshChannelMeta(ctx context.Context) (int, error) {
	if m.channelStore == nil {
		return 0, nil
	}

	type entry struct {
		bc    string
		route channelRoute
	}
	m.mu.RLock()
	entries := make([]entry, 0, len(m.channelMap))
	for bc, route := range m.channelMap {
		entries = append(entries, entry{bc, route})
	}
	m.mu.RUnlock()

	refreshed := 0
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return refreshed, err
		}
		resolver, ok := e.route.Adapter.(ChannelIdentity)
		if !ok {
			continue
		}
		// Per-channel timeout: one stuck adapter call must not hold the
		// whole refresh (mirrors persistChannelMeta's bound).
		resolveCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		meta, err := resolver.ResolveChannel(resolveCtx, e.route.ChannelID)
		cancel()
		if err != nil || meta == (ChannelMeta{}) {
			continue
		}
		if err := m.channelStore.UpsertChannelMeta(ctx, e.bc, meta.DisplayName, meta.Kind, meta.ParticipantCount); err != nil {
			log.Warn("gateway: failed to refresh channel meta", "channel", e.bc, "error", err)
			continue
		}
		refreshed++
	}
	return refreshed, nil
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

	// Determine the channel ID for routing. Prefer the platform-native id
	// supplied by the adapter (e.g., WhatsApp JID). Otherwise, for platforms
	// that need a numeric ID (e.g., Telegram chat_id), extract it from the
	// raw payload. Fall back to the channel name if extraction fails.
	channelID := n.ChannelID
	if channelID == "" {
		channelID = channelName
		if len(n.Raw) > 0 {
			var rawMsg struct {
				Message struct {
					Chat struct {
						ID int64 `json:"id"`
					} `json:"chat"`
				} `json:"message"`
			}
			if err := json.Unmarshal(n.Raw, &rawMsg); err == nil && rawMsg.Message.Chat.ID != 0 {
				channelID = strconv.FormatInt(rawMsg.Message.Chat.ID, 10)
			}
		}
	}

	// Ensure channel is in the map — only persist when first created. An
	// adapter-supplied native id also upgrades a route that was created (or
	// restored) with a fallback id, so identity resolution can work.
	m.mu.Lock()
	route, exists := m.channelMap[bcChannel]
	needMeta := false
	upgradedID := ""
	if !exists {
		route = channelRoute{
			Platform:  platform,
			ChannelID: channelID,
			Adapter:   m.adapters[platform],
		}
		m.channelMap[bcChannel] = route
		needMeta = true
		log.Info("gateway: dynamically mapped notification channel", "bc_channel", bcChannel, "platform", platform, "channel_id", channelID)
	} else if n.ChannelID != "" && route.ChannelID != n.ChannelID {
		route.ChannelID = n.ChannelID
		m.channelMap[bcChannel] = route
		needMeta = true
		upgradedID = n.ChannelID
		log.Info("gateway: upgraded channel route to native id", "bc_channel", bcChannel, "channel_id", n.ChannelID)
	}
	m.mu.Unlock()
	if !exists {
		m.persistChannel(bcChannel, platform, channelID)
	}
	if upgradedID != "" && m.channelStore != nil {
		// Persist the upgrade — without this a quiet channel reloads the
		// stale fallback id after restart and identity resolution breaks.
		go func(bc, id string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := m.channelStore.UpdateChannelPlatformID(ctx, bc, id); err != nil {
				log.Warn("gateway: failed to persist upgraded channel id", "channel", bc, "error", err)
			}
		}(bcChannel, upgradedID)
	}
	if needMeta {
		m.persistChannelMeta(route.Adapter, bcChannel, route.ChannelID, ChannelMeta{})
	}

	sender := fmt.Sprintf("[%s] %s", platform, n.Sender)

	// Use the adapter-extracted text content for display; fall back to raw JSON
	content := n.Content
	if content == "" {
		content = string(n.Raw)
	}
	if m.onInbound != nil {
		m.onInbound(bcChannel, sender, n.SenderID, content, n.MessageID, n.Mentions, n.Raw)
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
// Preserves ':' so adapters can pass compound names like
// "guildName:channelName" (Discord) without the segments concatenating.
func sanitizeChannelName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	// Remove any characters that aren't alphanumeric, dash, underscore, or colon
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ':' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
