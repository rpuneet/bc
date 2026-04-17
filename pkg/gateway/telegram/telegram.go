// Package telegram implements the gateway.Adapter for Telegram Bot API.
package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/rpuneet/bc/pkg/gateway"
	"github.com/rpuneet/bc/pkg/log"
)

// Adapter implements gateway.Adapter for Telegram.
type Adapter struct {
	lastMessageAt time.Time
	bot           *tgbotapi.BotAPI
	chatMap       map[int64]string
	token         string
	mode          string
	name          string
	lastError     string
	chatMu        sync.RWMutex
	connected     bool
}

// Ensure Adapter implements gateway.Adapter.
var _ gateway.Adapter = (*Adapter)(nil)
var _ gateway.StatusReporter = (*Adapter)(nil)

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

func (a *Adapter) Start(ctx context.Context, onMessage func(gateway.InboundMessage)) error {
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
			bot.StopReceivingUpdates()
			return nil
		case update := <-updates:
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
				// Skip non-text messages for now
				continue
			}

			senderID := ""
			if update.Message.From != nil {
				senderID = strconv.FormatInt(update.Message.From.ID, 10)
			}

			msg := gateway.InboundMessage{
				ChannelID:   strconv.FormatInt(chatID, 10),
				ChannelName: chatTitle,
				Sender:      sender,
				SenderID:    senderID,
				Content:     content,
				MessageID:   strconv.Itoa(update.Message.MessageID),
				Timestamp:   update.Message.Time(),
			}

			log.Info("telegram: received message",
				"chat", chatTitle,
				"sender", sender,
				"content", gateway.Truncate(content, 50))

			a.chatMu.Lock()
			a.lastMessageAt = time.Now()
			a.chatMu.Unlock()

			if onMessage != nil {
				onMessage(msg)
			}
		}
	}
}

func (a *Adapter) Stop(_ context.Context) error {
	if a.bot != nil {
		a.bot.StopReceivingUpdates()
	}
	return nil
}

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

func (a *Adapter) Channels(_ context.Context) ([]gateway.ExternalChannel, error) {
	a.chatMu.RLock()
	defer a.chatMu.RUnlock()

	channels := make([]gateway.ExternalChannel, 0, len(a.chatMap))
	for id, name := range a.chatMap {
		channels = append(channels, gateway.ExternalChannel{
			ID:   strconv.FormatInt(id, 10),
			Name: name,
			Type: "group",
		})
	}
	return channels, nil
}

func (a *Adapter) Health(_ context.Context) error {
	if a.bot == nil {
		return fmt.Errorf("telegram: not connected")
	}
	// Live probe: call getMe to verify the connection
	if _, err := a.bot.GetMe(); err != nil {
		a.chatMu.Lock()
		a.connected = false
		a.lastError = err.Error()
		a.chatMu.Unlock()
		return fmt.Errorf("telegram: getMe failed: %w", err)
	}
	a.chatMu.Lock()
	a.connected = true
	a.lastError = ""
	a.chatMu.Unlock()
	return nil
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
	}
}

// DiscoverViaUpdate processes a single getUpdates call to discover groups
// the bot has been added to. Called before Start to populate initial channels.
func (a *Adapter) DiscoverViaUpdate() error {
	bot, err := tgbotapi.NewBotAPI(a.token)
	if err != nil {
		return fmt.Errorf("telegram: connect for discovery: %w", err)
	}
	a.bot = bot

	// Do a quick getUpdates with short timeout to discover groups
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 2
	updates, err := bot.GetUpdates(u)
	if err != nil {
		// Not fatal — bot may just have no pending updates
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
// Used when the bot hasn't received any messages yet but we know about the group.
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
