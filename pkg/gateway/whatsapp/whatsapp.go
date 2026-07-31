// Package whatsapp implements a gateway.NotificationAdapter using whatsmeow
// (WhatsApp Web multi-device protocol). Connects via QR code pairing — no
// business account needed.
package whatsapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3" // register sqlite3 driver for whatsmeow session store
	qrcode "github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

// Adapter implements gateway.NotificationAdapter for WhatsApp via whatsmeow.
type Adapter struct { //nolint:govet
	client              *whatsmeow.Client
	container           *sqlstore.Container
	handler             func(gateway.Notification)
	lastMessageAt       time.Time
	name                string
	stateDir            string
	lastError           string
	mu                  sync.RWMutex
	connected           bool
	includeSelfMessages bool
	messageCount        atomic.Int64
	// qrChan receives QR codes during pairing.
	qrChan     chan string
	groupCache map[string]string
	// idClient overrides the whatsmeow-backed identity client in tests.
	idClient  identityClient
	metaMu    sync.Mutex
	metaCache map[string]cachedMeta
}

func init() {
	// Fetch latest WA web version and set device identity to match WhatsApp Web.
	if ver, err := whatsmeow.GetLatestVersion(context.Background(), nil); err == nil {
		store.SetWAVersion(*ver)
		log.Info("whatsapp: using WA version", "version", ver.String())
	}
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
	store.SetOSInfo("Chrome", store.GetWAVersion())
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a WhatsApp adapter. stateDir is where session data is stored
// (typically .bc/gateways/whatsapp/).
func New(stateDir string) *Adapter {
	return &Adapter{name: "whatsapp", stateDir: stateDir}
}

// NewNamed creates a named WhatsApp adapter for multi-account setups.
func NewNamed(name, stateDir string) *Adapter {
	return &Adapter{name: name, stateDir: stateDir}
}

// SetIncludeSelfMessages controls whether messages sent by the paired account
// itself are ingested. Off by default; enable to discover groups the user
// created where no other party has yet sent a message.
func (a *Adapter) SetIncludeSelfMessages(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.includeSelfMessages = v
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterSocket }
func (a *Adapter) HTTPHandler() http.Handler { return nil } //nolint:revive

// SetHandler sets the notification handler for messages received via QR pairing.
// Called by the gateway handler after pair completes to wire messages into the
// notification system without going through Start().
func (a *Adapter) SetHandler(handler func(gateway.Notification)) {
	a.mu.Lock()
	a.handler = handler
	a.mu.Unlock()
}

// PairStatus represents the current state of WhatsApp pairing.
type PairStatus struct {
	State     string `json:"state"` // "idle", "qr_ready", "connected", "error"
	QRDataURL string `json:"qr_data_url,omitempty"`
	Error     string `json:"error,omitempty"`
	Phone     string `json:"phone,omitempty"`
}

// StartPairing initiates a QR code pairing flow. Returns immediately with
// the QR code as a data URL. Does not require the daemon restart.
func (a *Adapter) StartPairing(ctx context.Context) (*PairStatus, error) {
	if err := os.MkdirAll(a.stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("whatsapp: create state dir: %w", err)
	}

	dbPath := filepath.Join(a.stateDir, "whatsapp.db")
	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbPath), nil)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: open session store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: get device: %w", err)
	}

	// Already paired?
	if deviceStore.ID != nil {
		client := whatsmeow.NewClient(deviceStore, nil)
		client.AddEventHandler(func(evt interface{}) {
			a.handleEvent(evt)
		})
		if err := client.Connect(); err == nil {
			a.mu.Lock()
			a.client = client
			a.container = container
			a.connected = true
			a.mu.Unlock()
			return &PairStatus{State: "connected", Phone: deviceStore.ID.User}, nil
		}
	}

	// Start fresh pairing with verbose logging.
	waLogger := waLog.Stdout("whatsapp", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, waLogger)

	// Use background context — the HTTP request context ends when we return the QR,
	// but the QR channel must stay alive until the user scans.
	bgCtx := context.Background()
	qrChan, _ := client.GetQRChannel(bgCtx)

	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("whatsapp: connect: %w", err)
	}

	// Wait for first QR code (with timeout).
	var qrCode string
	timeout := time.After(15 * time.Second)
	for {
		select {
		case evt := <-qrChan:
			if evt.Event == "code" {
				qrCode = evt.Code
				goto gotQR
			}
			if evt.Event == "success" {
				a.mu.Lock()
				a.client = client
				a.container = container
				a.connected = true
				a.mu.Unlock()
				return &PairStatus{State: "connected"}, nil
			}
		case <-timeout:
			client.Disconnect()
			return nil, fmt.Errorf("whatsapp: timeout waiting for QR code")
		case <-ctx.Done():
			client.Disconnect()
			return nil, ctx.Err()
		}
	}

gotQR:
	// Generate QR code as PNG base64.
	png, qrErr := qrcode.Encode(qrCode, qrcode.Medium, 256)
	if qrErr != nil {
		client.Disconnect()
		return nil, fmt.Errorf("whatsapp: generate QR image: %w", qrErr)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

	// Store client for background — wait for scan in goroutine.
	a.mu.Lock()
	a.client = client
	a.container = container
	a.mu.Unlock()

	go func() {
		for evt := range qrChan {
			log.Info("whatsapp: qr channel event", "event", evt.Event, "code_len", len(evt.Code))
			if evt.Event == "success" {
				a.mu.Lock()
				a.connected = true
				a.lastError = ""
				a.mu.Unlock()
				log.Info("whatsapp: paired successfully")
				client.AddEventHandler(func(evt interface{}) {
					a.handleEvent(evt)
				})
				return
			}
		}
	}()

	return &PairStatus{State: "qr_ready", QRDataURL: dataURL}, nil
}

// GetPairStatus returns the current pairing/connection state.
func (a *Adapter) GetPairStatus() *PairStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.connected {
		phone := ""
		if a.client != nil && a.client.Store.ID != nil {
			phone = a.client.Store.ID.User
		}
		return &PairStatus{State: "connected", Phone: phone}
	}
	if a.lastError != "" {
		return &PairStatus{State: "error", Error: a.lastError}
	}
	if a.client != nil {
		return &PairStatus{State: "pairing"}
	}
	return &PairStatus{State: "idle"}
}

// Start connects to WhatsApp. If no session exists, it initiates QR pairing.
// Blocks until ctx is canceled.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler
	a.qrChan = make(chan string, 5)

	if err := os.MkdirAll(a.stateDir, 0o700); err != nil {
		return fmt.Errorf("whatsapp: create state dir: %w", err)
	}

	dbPath := filepath.Join(a.stateDir, "whatsapp.db")
	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbPath), nil)
	if err != nil {
		return fmt.Errorf("whatsapp: open session store: %w", err)
	}
	a.container = container

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("whatsapp: get device: %w", err)
	}

	client := whatsmeow.NewClient(deviceStore, nil)
	a.client = client

	// Register event handler for incoming messages.
	client.AddEventHandler(func(evt interface{}) {
		a.handleEvent(evt)
	})

	// Connect — if not logged in, this will emit QR events.
	if client.Store.ID == nil {
		// Not paired yet — need QR code flow.
		qrChan, _ := client.GetQRChannel(ctx)
		go func() {
			for evt := range qrChan {
				if evt.Event == "code" {
					select {
					case a.qrChan <- evt.Code:
					default:
					}
					log.Info("whatsapp: QR code generated, waiting for scan...")
				}
			}
		}()
		if err := client.Connect(); err != nil {
			return fmt.Errorf("whatsapp: connect for pairing: %w", err)
		}
		log.Info("whatsapp: waiting for QR scan...", "adapter", a.name)
	} else {
		// Already paired — just connect.
		if err := client.Connect(); err != nil {
			return fmt.Errorf("whatsapp: connect: %w", err)
		}
		a.mu.Lock()
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()
		log.Info("whatsapp: connected (existing session)", "adapter", a.name)
	}

	// Block until context is done.
	<-ctx.Done()
	client.Disconnect()
	return nil
}

// Stop disconnects the client.
func (a *Adapter) Stop() error {
	if a.client != nil {
		a.client.Disconnect()
	}
	a.mu.Lock()
	a.connected = false
	a.mu.Unlock()
	return nil
}

// Send delivers an outbound text message to a WhatsApp chat. channelID must
// be a platform-native JID (e.g. "1234@s.whatsapp.net", "1234@lid" or
// "1234@g.us"); the gateway manager stores it on the channel route from
// inbound messages. The sender name is ignored — messages go out from the
// paired personal account, so callers should attribute the author in the
// content itself.
func (a *Adapter) Send(ctx context.Context, channelID, _, content string) error {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()
	if client == nil || !connected {
		return fmt.Errorf("whatsapp: not connected")
	}

	jid, err := parseSendJID(channelID)
	if err != nil {
		return err
	}

	// Bound the send so a stalled network round-trip can't block the caller
	// indefinitely. Only apply the fallback when the caller's context carries
	// no deadline of its own, so an explicit deadline is always respected.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, sendTimeout)
		defer cancel()
	}

	if _, err := client.SendMessage(ctx, jid, &waE2E.Message{Conversation: proto.String(content)}); err != nil {
		return fmt.Errorf("whatsapp: send to %s: %w", jid, err)
	}
	return nil
}

// sendTimeout bounds a single outbound WhatsApp send when the caller supplies
// no deadline of its own.
const sendTimeout = 30 * time.Second

// parseSendJID converts a stored channel id into a routable JID. Bare ids
// without a server part are ambiguous between phone-number and hidden-lid
// chats, so they are rejected: the route upgrades to a native JID on the
// next inbound message from that chat.
func parseSendJID(channelID string) (types.JID, error) {
	if !strings.Contains(channelID, "@") {
		return types.JID{}, fmt.Errorf("whatsapp: channel id %q has no JID server — wait for an inbound message to upgrade the route", channelID)
	}
	jid, err := types.ParseJID(channelID)
	if err != nil {
		return types.JID{}, fmt.Errorf("whatsapp: invalid JID %q: %w", channelID, err)
	}
	if jid.User == "" {
		return types.JID{}, fmt.Errorf("whatsapp: invalid JID %q: empty user", channelID)
	}
	return jid, nil
}

// SendReaction sends a reaction emoji to a previously-received message.
// channelID is the chat JID (from Notification.ChannelID), senderJID is the
// JID of the original message author (from Notification.SenderID), messageID
// is the platform message ID (from Notification.MessageID), and emoji is the
// reaction character (empty string removes any existing reaction).
func (a *Adapter) SendReaction(ctx context.Context, channelID, senderJID, messageID, emoji string) error {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()
	if client == nil || !connected {
		return fmt.Errorf("whatsapp: not connected")
	}

	chat, err := parseSendJID(channelID)
	if err != nil {
		return err
	}
	msgSender, err := types.ParseJID(senderJID)
	if err != nil {
		return fmt.Errorf("whatsapp: invalid sender JID %q: %w", senderJID, err)
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, sendTimeout)
		defer cancel()
	}

	reaction := client.BuildReaction(chat, msgSender, messageID, emoji)
	if _, err := client.SendMessage(ctx, chat, reaction); err != nil {
		return fmt.Errorf("whatsapp: send reaction to %s: %w", chat, err)
	}
	return nil
}

// Channels returns discovered chats (empty until connected).
func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{{ID: "messages", Name: "messages", Platform: "whatsapp"}}
}

// Status returns the adapter's connection state.
func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{
		Connected:     a.connected,
		LastMessageAt: a.lastMessageAt,
		Error:         a.lastError,
		MessageCount:  a.messageCount.Load(),
	}
}

// handleEvent processes whatsmeow events.
func (a *Adapter) handleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		a.handleMessage(v)
	case *events.Connected:
		a.mu.Lock()
		a.connected = true
		a.lastError = ""
		a.mu.Unlock()
		log.Info("whatsapp: session connected")
	case *events.Disconnected:
		a.mu.Lock()
		a.connected = false
		a.mu.Unlock()
		log.Warn("whatsapp: disconnected")
	case *events.LoggedOut:
		a.mu.Lock()
		a.connected = false
		a.lastError = "logged out — re-scan QR to reconnect"
		a.mu.Unlock()
		log.Warn("whatsapp: logged out")
	}
}

// handleMessage processes an incoming WhatsApp message.
func (a *Adapter) handleMessage(msg *events.Message) {
	// Skip own messages unless explicitly opted in.
	if msg.Info.IsFromMe {
		a.mu.Lock()
		include := a.includeSelfMessages
		a.mu.Unlock()
		if !include {
			return
		}
	}

	// Skip status broadcasts.
	if msg.Info.Chat.Server == "broadcast" {
		return
	}

	sender := formatSender(msg.Info)
	content := extractContent(msg.Message)
	channel := a.resolveChannel(msg.Info)

	now := time.Now()
	a.mu.Lock()
	a.lastMessageAt = now
	a.mu.Unlock()
	a.messageCount.Add(1)

	log.Info("whatsapp: message", "sender", sender, "channel", channel,
		"content", gateway.Truncate(content, 50))

	if a.handler != nil {
		raw, _ := json.Marshal(msg) //nolint:errcheck
		a.handler(gateway.Notification{
			Channel:   channel,
			ChannelID: msg.Info.Chat.String(), // native JID so identity resolution works
			Platform:  "whatsapp",
			Sender:    sender,
			SenderID:  msg.Info.Sender.String(), // native JID for reactions
			Content:   content,
			MessageID: msg.Info.ID,
			Mentions:  extractWAMentions(msg.Message),
			Timestamp: now,
			Raw:       raw,
		})
	}
}

// extractWAMentions returns the JID user parts (phone numbers) from a WhatsApp
// message's ContextInfo.MentionedJID (e.g. "1234567890@s.whatsapp.net" → "1234567890").
//
// These phone numbers are passed as extraMentions to notify.Dispatch but will NOT
// match mycel agent names in the mention_only gate — agent names are strings like
// "zen-zebra", not phone numbers. Agent mention filtering uses the text @name
// extraction path (extractMentions in notify/service.go), which parses typed
// "@agentname" tokens from the message content. The JID user parts are included in
// the merged mention set for any future use cases where a subscriber is registered
// under a phone-number identity.
func extractWAMentions(msg *waE2E.Message) []string {
	if msg == nil {
		return nil
	}
	var ci *waE2E.ContextInfo
	switch {
	case msg.ExtendedTextMessage != nil:
		ci = msg.ExtendedTextMessage.ContextInfo
	case msg.ImageMessage != nil:
		ci = msg.ImageMessage.ContextInfo
	case msg.VideoMessage != nil:
		ci = msg.VideoMessage.ContextInfo
	case msg.DocumentMessage != nil:
		ci = msg.DocumentMessage.ContextInfo
	case msg.AudioMessage != nil:
		ci = msg.AudioMessage.ContextInfo
	}
	if ci == nil || len(ci.MentionedJID) == 0 {
		return nil
	}
	mentions := make([]string, 0, len(ci.MentionedJID))
	for _, raw := range ci.MentionedJID {
		j, err := types.ParseJID(raw)
		if err == nil && j.User != "" {
			mentions = append(mentions, j.User)
		}
	}
	return mentions
}

// formatSender returns a display name for the message sender.
func formatSender(info types.MessageInfo) string {
	if info.PushName != "" {
		return info.PushName
	}
	return info.Sender.User
}

// resolveChannel returns a human-readable channel name. For groups, fetches
// the group subject via the WhatsApp API. Caches results.
func (a *Adapter) resolveChannel(info types.MessageInfo) string {
	jid := info.Chat

	a.mu.Lock()
	if a.groupCache == nil {
		a.groupCache = make(map[string]string)
	}
	if cached, ok := a.groupCache[jid.String()]; ok {
		a.mu.Unlock()
		return cached
	}
	a.mu.Unlock()

	if info.IsGroup && a.client != nil {
		if groupInfo, err := a.client.GetGroupInfo(context.Background(), jid); err == nil && groupInfo.Name != "" {
			a.mu.Lock()
			a.groupCache[jid.String()] = groupInfo.Name
			a.mu.Unlock()
			return groupInfo.Name
		}
	}

	// Fallback: use phone number for DMs, JID user part for groups
	name := jid.User
	a.mu.Lock()
	a.groupCache[jid.String()] = name
	a.mu.Unlock()
	return name
}

// extractContent pulls text from a WhatsApp message proto.
func extractContent(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Conversation != nil {
		return *msg.Conversation
	}
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.Text != nil {
		return *msg.ExtendedTextMessage.Text
	}
	if msg.ImageMessage != nil {
		caption := ""
		if msg.ImageMessage.Caption != nil {
			caption = *msg.ImageMessage.Caption
		}
		if caption != "" {
			return "[photo] " + caption
		}
		return "[photo]"
	}
	if msg.VideoMessage != nil {
		caption := ""
		if msg.VideoMessage.Caption != nil {
			caption = *msg.VideoMessage.Caption
		}
		if caption != "" {
			return "[video] " + caption
		}
		return "[video]"
	}
	if msg.DocumentMessage != nil {
		name := "file"
		if msg.DocumentMessage.FileName != nil {
			name = *msg.DocumentMessage.FileName
		}
		return "[document: " + name + "]"
	}
	if msg.AudioMessage != nil {
		return "[voice message]"
	}
	if msg.StickerMessage != nil {
		return "[sticker]"
	}
	if msg.ContactMessage != nil {
		return "[contact]"
	}
	if msg.LocationMessage != nil {
		return "[location]"
	}
	if msg.PollCreationMessage != nil {
		q := "poll"
		if msg.PollCreationMessage.Name != nil {
			q = *msg.PollCreationMessage.Name
		}
		return "[poll: " + q + "]"
	}
	if msg.ReactionMessage != nil {
		emoji := ""
		if msg.ReactionMessage.Text != nil {
			emoji = *msg.ReactionMessage.Text
		}
		if emoji == "" {
			return "[reaction removed]"
		}
		return "[reaction: " + emoji + "]"
	}
	if msg.EditedMessage != nil && msg.EditedMessage.Message != nil {
		edited := extractContent(msg.EditedMessage.Message)
		if edited != "" {
			return "[edited] " + edited
		}
		return "[edited message]"
	}
	if msg.ProtocolMessage != nil && msg.ProtocolMessage.EditedMessage != nil {
		edited := extractContent(msg.ProtocolMessage.EditedMessage)
		if edited != "" {
			return "[edited] " + edited
		}
		return "[edited message]"
	}
	if msg.ListMessage != nil {
		title := "list"
		if msg.ListMessage.Title != nil {
			title = *msg.ListMessage.Title
		}
		return "[list: " + title + "]"
	}
	if msg.ButtonsMessage != nil {
		text := "buttons"
		if msg.ButtonsMessage.ContentText != nil {
			text = *msg.ButtonsMessage.ContentText
		}
		return "[buttons: " + text + "]"
	}
	if msg.TemplateMessage != nil {
		return "[template]"
	}
	return "[unsupported message type]"
}
