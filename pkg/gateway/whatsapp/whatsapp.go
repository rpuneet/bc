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
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3" // register sqlite3 driver for whatsmeow session store
	qrcode "github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/rpuneet/bc/pkg/gateway"
	"github.com/rpuneet/bc/pkg/log"
)

// Adapter implements gateway.NotificationAdapter for WhatsApp via whatsmeow.
type Adapter struct {
	client        *whatsmeow.Client
	container     *sqlstore.Container
	handler       func(gateway.Notification)
	lastMessageAt time.Time
	name          string
	stateDir      string
	lastError     string
	mu            sync.Mutex
	connected     bool
	messageCount  atomic.Int64
	// qrChan receives QR codes during pairing.
	qrChan chan string
}

func init() {
	// Set device properties to mimic Chrome WhatsApp Web — reduces rate limiting.
	store.SetOSInfo("bc", [3]uint32{2, 26, 4})
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

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterSocket }
func (a *Adapter) HTTPHandler() http.Handler { return nil } //nolint:revive

// PairStatus represents the current state of WhatsApp pairing.
type PairStatus struct {
	State      string `json:"state"` // "idle", "qr_ready", "connected", "error"
	QRDataURL  string `json:"qr_data_url,omitempty"`
	Error      string `json:"error,omitempty"`
	Phone      string `json:"phone,omitempty"`
}

// StartPairing initiates a QR code pairing flow. Returns immediately with
// the QR code as a data URL. Does not require bcd restart.
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
		if err := client.Connect(); err == nil {
			a.mu.Lock()
			a.client = client
			a.container = container
			a.connected = true
			a.mu.Unlock()
			return &PairStatus{State: "connected", Phone: deviceStore.ID.User}, nil
		}
	}

	// Start fresh pairing.
	client := whatsmeow.NewClient(deviceStore, nil)

	qrChan, _ := client.GetQRChannel(ctx)

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
			if evt.Event == "success" {
				a.mu.Lock()
				a.connected = true
				a.lastError = ""
				a.mu.Unlock()
				log.Info("whatsapp: paired successfully")
				// Register event handler for messages.
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
	// Skip own messages.
	if msg.Info.IsFromMe {
		return
	}

	// Skip status broadcasts.
	if msg.Info.Chat.Server == "broadcast" {
		return
	}

	sender := formatSender(msg.Info)
	content := extractContent(msg.Message)
	channel := formatChannel(msg.Info)

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
			Platform:  "whatsapp",
			Sender:    sender,
			Content:   content,
			Timestamp: now,
			Raw:       raw,
		})
	}
}

// formatSender returns a display name for the message sender.
func formatSender(info types.MessageInfo) string {
	if info.PushName != "" {
		return info.PushName
	}
	return info.Sender.User
}

// formatChannel returns a channel identifier for the chat.
func formatChannel(info types.MessageInfo) string {
	if info.IsGroup {
		// Use group JID as channel name.
		return info.Chat.User
	}
	// DM — use sender number as channel.
	return info.Sender.User
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
	return "[unsupported message type]"
}
