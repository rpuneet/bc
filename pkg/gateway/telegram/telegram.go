// Package telegram implements the gateway.NotificationAdapter for Telegram Bot API.
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

// Adapter implements gateway.NotificationAdapter for Telegram.
// It also supports outbound messaging via the Send method.
type Adapter struct {
	lastMessageAt time.Time
	bot           *tgbotapi.BotAPI
	chatMap       map[int64]string
	token         string
	mode          string
	name          string
	lastError     string
	chatMu        sync.RWMutex
	stopOnce      sync.Once
	connected     bool
	messageCount  atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a new Telegram adapter with the default name "telegram".
func New(token, mode string) *Adapter {
	return NewNamed("telegram", token, mode)
}

// NewNamed creates a Telegram adapter with a custom name (e.g. "telegram:trade_research").
// This allows registering multiple Telegram bots in the same gateway manager.
func NewNamed(name, token, mode string) *Adapter {
	if mode == "" {
		mode = "polling"
	}
	if name == "" {
		name = "telegram"
	}
	return &Adapter{
		name:    name,
		token:   token,
		mode:    mode,
		chatMap: make(map[int64]string),
	}
}

func (a *Adapter) Name() string { return a.name }

// Type returns AdapterSocket since Telegram uses long-polling that blocks like a socket.
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterSocket }

// Start connects to Telegram and forwards events as Notifications.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	bot, err := tgbotapi.NewBotAPI(a.token)
	if err != nil {
		return fmt.Errorf("telegram: failed to connect: %w", err)
	}
	a.bot = bot
	a.chatMu.Lock()
	a.connected = true
	a.chatMu.Unlock()
	log.Info("telegram: connected", "bot", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			a.stopBot()
			return nil
		case update, ok := <-updates:
			if !ok {
				// StopReceivingUpdates closed the channel — exit so clearRunning can run.
				a.stopBot()
				return nil
			}
			if update.Message == nil {
				continue
			}

			chatID := update.Message.Chat.ID
			chatTitle := update.Message.Chat.Title
			if chatTitle == "" {
				chatTitle = update.Message.Chat.UserName
			}

			// Track discovered groups
			if chatTitle != "" {
				a.chatMu.Lock()
				a.chatMap[chatID] = chatTitle
				a.chatMu.Unlock()
			}

			// Skip messages from the bot itself
			if update.Message.From != nil && update.Message.From.ID == bot.Self.ID {
				continue
			}

			sender := "unknown"
			if update.Message.From != nil {
				sender = update.Message.From.FirstName
				if update.Message.From.LastName != "" {
					sender += " " + update.Message.From.LastName
				}
				if sender == "" {
					sender = update.Message.From.UserName
				}
			}

			content := update.Message.Text
			if content == "" {
				// Use caption for photos/documents/videos, or describe the media type
				if update.Message.Caption != "" {
					content = update.Message.Caption
				} else if update.Message.Photo != nil {
					content = "[photo]"
				} else if update.Message.Document != nil {
					content = "[document: " + update.Message.Document.FileName + "]"
				} else if update.Message.Video != nil {
					content = "[video]"
				} else if update.Message.Voice != nil {
					content = "[voice message]"
				} else if update.Message.Sticker != nil {
					content = "[sticker]"
				} else if update.Message.Location != nil {
					content = "[location]"
				}
			}
			if content == "" {
				// Skip truly empty messages (edits, service messages, etc.)
				continue
			}

			now := time.Now()
			a.chatMu.Lock()
			a.lastMessageAt = now
			a.chatMu.Unlock()

			a.messageCount.Add(1)

			log.Info("telegram: received message",
				"chat", chatTitle,
				"sender", sender,
				"content", gateway.Truncate(content, 50))

			// Build raw JSON from the update
			// Marshal entire update — preserves photos, documents, stickers,
			// audio, location, and all platform fields (raw passthrough).
			raw, marshalErr := json.Marshal(update)
			if marshalErr != nil {
				log.Warn("telegram: failed to marshal update", "error", marshalErr)
				continue
			}

			channelName := chatTitle
			if channelName == "" {
				channelName = strconv.FormatInt(chatID, 10)
			}

			handler(gateway.Notification{
				Channel:   channelName,
				Platform:  a.name,
				Sender:    sender,
				Content:   content,
				Timestamp: update.Message.Time(),
				Raw:       raw,
			})
		}
	}
}

// Stop gracefully disconnects.
func (a *Adapter) Stop() error {
	a.stopBot()
	return nil
}

// stopBot stops the bot's update polling exactly once. Both the
// context-cancel path in Start and Stop route through it: the underlying
// library closes a channel unconditionally, so a second call panics with
// "close of closed channel" (seen when Services.Close cancels the gateway
// context and then calls Manager.Stop during daemon shutdown).
func (a *Adapter) stopBot() {
	a.stopOnce.Do(func() {
		if a.bot != nil {
			a.bot.StopReceivingUpdates()
		}
	})
}

// HTTPHandler returns nil since Telegram uses polling, not webhooks.
func (a *Adapter) HTTPHandler() http.Handler { return nil }

// Channels returns discovered channels.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	a.chatMu.RLock()
	defer a.chatMu.RUnlock()

	channels := make([]gateway.ChannelInfo, 0, len(a.chatMap))
	for id, name := range a.chatMap {
		// Telegram group/supergroup/channel chat ids are negative; positive
		// ids are private chats with a person.
		kind := gateway.ChannelKindPerson
		if id < 0 {
			kind = gateway.ChannelKindGroup
		}
		channels = append(channels, gateway.ChannelInfo{
			ID:       strconv.FormatInt(id, 10),
			Name:     name,
			Platform: a.name,
			Kind:     kind,
		})
	}
	return channels
}

// Status returns the current connection state.
func (a *Adapter) Status() gateway.AdapterStatus {
	a.chatMu.RLock()
	defer a.chatMu.RUnlock()
	botName := ""
	if a.bot != nil {
		botName = a.bot.Self.UserName
	}
	return gateway.AdapterStatus{
		Connected:     a.connected,
		LastMessageAt: a.lastMessageAt,
		Error:         a.lastError,
		BotName:       botName,
		MessageCount:  a.messageCount.Load(),
	}
}

// --- MessageSender (outbound messaging) ---

// Send delivers a message to a Telegram chat.
func (a *Adapter) Send(_ context.Context, channelID, sender, content string) error {
	if a.bot == nil {
		return fmt.Errorf("telegram: bot not connected")
	}

	chatID, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return fmt.Errorf("telegram: invalid chat_id %q: %w", channelID, err)
	}

	// Format: <b>agent_name</b>: message
	text := fmt.Sprintf("<b>%s</b>: %s", escapeHTML(sender), escapeHTML(content))

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML

	if _, err := a.bot.Send(msg); err != nil {
		return fmt.Errorf("telegram: send failed: %w", err)
	}

	log.Info("telegram: sent message", "chat_id", channelID, "sender", sender)
	return nil
}

// DiscoverViaUpdate processes a single getUpdates call to discover groups
// the bot has been added to. Called before Start to populate initial channels.
func (a *Adapter) DiscoverViaUpdate() error {
	bot, err := tgbotapi.NewBotAPI(a.token)
	if err != nil {
		return fmt.Errorf("telegram: connect for discovery: %w", err)
	}
	a.bot = bot

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 2
	updates, err := bot.GetUpdates(u)
	if err != nil {
		log.Warn("telegram: discovery getUpdates failed", "error", err)
		return nil
	}

	for _, update := range updates {
		if update.Message != nil && (update.Message.Chat.IsGroup() || update.Message.Chat.IsSuperGroup()) {
			chatID := update.Message.Chat.ID
			chatTitle := update.Message.Chat.Title
			if chatTitle != "" {
				a.chatMu.Lock()
				a.chatMap[chatID] = chatTitle
				a.chatMu.Unlock()
				log.Info("telegram: discovered group via update", "chat_id", chatID, "title", chatTitle)
			}
		}
	}

	return nil
}

// AddChat manually registers a chat ID → name mapping.
func (a *Adapter) AddChat(chatID int64, name string) {
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	a.chatMap[chatID] = name
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
