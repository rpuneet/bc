// Package gmailgw implements a gateway.NotificationAdapter for Gmail.
//
// Inbound is poll-based: on a fixed interval it lists messages matching a
// Gmail search query (default "is:unread") in a label (default INBOX) and
// converts each new message into a normalized Notification. A per-connect
// internalDate cursor plus a seen-set guards against re-delivery.
//
// Outbound Send composes an RFC 5322 message and delivers it through
// users.messages.send. Authentication is OAuth2: the plugin supplies an
// offline (refresh-token) token source, so access tokens refresh
// automatically.
package gmailgw

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

const (
	platform        = "gmail"
	defaultLabel    = "INBOX"
	defaultQuery    = "is:unread"
	defaultInterval = 60 // seconds
	maxListResults  = 25
)

// Credentials is the resolved configuration for a Gmail adapter.
type Credentials struct {
	Name         string
	ClientID     string
	ClientSecret string
	RefreshToken string
	Label        string
	Query        string
	Interval     int
}

// mailEntry is the normalized representation of a single Gmail message,
// marshaled into Notification.Raw as JSON.
type mailEntry struct {
	ID       string `json:"id"`
	ThreadID string `json:"thread_id,omitempty"`
	From     string `json:"from"`
	Subject  string `json:"subject,omitempty"`
	Snippet  string `json:"snippet,omitempty"`
	Date     string `json:"date,omitempty"`
}

// Adapter implements gateway.NotificationAdapter for Gmail.
type Adapter struct {
	svc           *gmail.Service
	handler       func(gateway.Notification)
	seen          map[string]struct{}
	lastMessageAt time.Time
	name          string
	clientID      string
	clientSecret  string
	refreshToken  string
	label         string
	query         string
	fromAddr      string
	lastError     string
	sinceUnixMs   int64
	messageCount  atomic.Int64
	mu            sync.Mutex
	interval      int
	connected     bool
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a Gmail adapter from resolved credentials.
func New(c Credentials) *Adapter {
	name := c.Name
	if name == "" {
		name = platform
	}
	label := c.Label
	if label == "" {
		label = defaultLabel
	}
	query := c.Query
	if query == "" {
		query = defaultQuery
	}
	interval := c.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	return &Adapter{
		name:         name,
		clientID:     c.ClientID,
		clientSecret: c.ClientSecret,
		refreshToken: c.RefreshToken,
		label:        label,
		query:        query,
		interval:     interval,
		seen:         make(map[string]struct{}),
	}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterPoll }
func (a *Adapter) HTTPHandler() http.Handler { return nil }

// tokenSource builds an offline OAuth2 token source. Access tokens refresh
// automatically from the refresh token.
func (a *Adapter) tokenSource(ctx context.Context) oauth2.TokenSource {
	conf := &oauth2.Config{
		ClientID:     a.clientID,
		ClientSecret: a.clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{gmail.GmailReadonlyScope, gmail.GmailSendScope},
	}
	return conf.TokenSource(ctx, &oauth2.Token{RefreshToken: a.refreshToken})
}

// Start builds the Gmail service and polls on the configured interval until
// ctx is canceled.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler

	svc, err := gmail.NewService(ctx, option.WithTokenSource(a.tokenSource(ctx)))
	if err != nil {
		a.setError(fmt.Sprintf("build service: %v", err))
		return fmt.Errorf("gmail: build service: %w", err)
	}
	a.svc = svc

	if profile, perr := svc.Users.GetProfile("me").Context(ctx).Do(); perr == nil {
		a.mu.Lock()
		a.fromAddr = profile.EmailAddress
		a.connected = true
		a.mu.Unlock()
		log.Info("gmail: connected", "adapter", a.name, "email", profile.EmailAddress)
	} else {
		a.setError(fmt.Sprintf("get profile: %v", perr))
		return fmt.Errorf("gmail: get profile: %w", perr)
	}

	// Seed the cursor at connect time so pre-existing mail is not replayed.
	a.mu.Lock()
	a.sinceUnixMs = time.Now().UnixMilli()
	a.mu.Unlock()

	a.poll(ctx)

	ticker := time.NewTicker(time.Duration(a.interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			a.poll(ctx)
		}
	}
}

// Stop is a no-op; cancellation is via context.
func (a *Adapter) Stop() error { return nil }

// Channels returns the connected mailbox as a single channel.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	a.mu.Lock()
	from := a.fromAddr
	a.mu.Unlock()
	name := from
	if name == "" {
		name = a.name
	}
	return []gateway.ChannelInfo{
		{
			ID:       platform + ":" + strings.ToLower(a.label),
			Name:     name,
			Platform: platform,
			Kind:     gateway.ChannelKindFeed,
		},
	}
}

// Status returns connection state for the web UI.
func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{
		Connected:     a.connected,
		LastMessageAt: a.lastMessageAt,
		Error:         a.lastError,
		BotName:       a.fromAddr,
		MessageCount:  a.messageCount.Load(),
	}
}

// poll lists new messages and dispatches them as notifications.
func (a *Adapter) poll(ctx context.Context) {
	if a.svc == nil {
		return
	}

	list, err := a.svc.Users.Messages.List("me").
		LabelIds(a.label).
		Q(a.query).
		MaxResults(maxListResults).
		Context(ctx).
		Do()
	if err != nil {
		a.setError(fmt.Sprintf("list messages: %v", err))
		return
	}

	a.mu.Lock()
	a.connected = true
	a.lastError = ""
	since := a.sinceUnixMs
	a.mu.Unlock()

	for _, ref := range list.Messages {
		a.mu.Lock()
		_, dup := a.seen[ref.Id]
		a.mu.Unlock()
		if dup {
			continue
		}

		msg, gerr := a.svc.Users.Messages.Get("me", ref.Id).
			Format("metadata").
			MetadataHeaders("From", "Subject", "Date").
			Context(ctx).
			Do()
		if gerr != nil {
			a.setError(fmt.Sprintf("get message %s: %v", ref.Id, gerr))
			continue
		}

		a.mu.Lock()
		a.seen[msg.Id] = struct{}{}
		a.mu.Unlock()

		// Skip mail that predates the connect cursor.
		if msg.InternalDate != 0 && msg.InternalDate < since {
			continue
		}

		entry := parseMessage(msg)
		a.dispatch(entry)
	}

	log.Debug("gmail: poll complete", "adapter", a.name, "listed", len(list.Messages))
}

// dispatch converts an entry to a Notification and forwards it. The channel
// id is the sender address so replies route back to the correspondent.
func (a *Adapter) dispatch(e mailEntry) {
	addr := extractEmail(e.From)
	a.messageCount.Add(1)
	a.mu.Lock()
	a.lastMessageAt = time.Now()
	a.mu.Unlock()

	if a.handler == nil {
		return
	}

	raw, _ := json.Marshal(e) //nolint:errcheck
	content := e.Subject
	if e.Snippet != "" {
		if content != "" {
			content += "\n\n"
		}
		content += e.Snippet
	}

	channelName := addr
	if channelName == "" {
		channelName = a.label
	}

	a.handler(gateway.Notification{
		Timestamp: time.Now(),
		Raw:       raw,
		Channel:   channelName,
		ChannelID: addr,
		Platform:  platform,
		Sender:    e.From,
		SenderID:  addr,
		Content:   content,
		MessageID: e.ID,
	})
}

func (a *Adapter) setError(msg string) {
	a.mu.Lock()
	a.lastError = msg
	a.mu.Unlock()
	log.Warn("gmail: "+msg, "adapter", a.name)
}

// Send composes and delivers an email. channelID is the recipient address
// (optionally prefixed "gmail:"). content's first line becomes the subject
// and the remainder the body; a single-line content is used for both.
func (a *Adapter) Send(ctx context.Context, channelID, _, content string) error {
	if a.svc == nil {
		return fmt.Errorf("gmail: not connected")
	}
	to := recipientAddress(channelID)
	if to == "" {
		return fmt.Errorf("gmail: no recipient in channel id %q", channelID)
	}

	a.mu.Lock()
	from := a.fromAddr
	a.mu.Unlock()

	raw := buildMessage(from, to, content)
	if _, err := a.svc.Users.Messages.Send("me", &gmail.Message{Raw: raw}).Context(ctx).Do(); err != nil {
		return fmt.Errorf("gmail: send to %s: %w", to, err)
	}
	log.Info("gmail: sent message", "adapter", a.name, "to", to)
	return nil
}

// --- pure helpers (unit-tested) ---

// parseMessage converts a Gmail API message into a normalized mailEntry.
func parseMessage(m *gmail.Message) mailEntry {
	e := mailEntry{ID: m.Id, ThreadID: m.ThreadId, Snippet: m.Snippet}
	if m.Payload != nil {
		for _, h := range m.Payload.Headers {
			switch strings.ToLower(h.Name) {
			case "from":
				e.From = h.Value
			case "subject":
				e.Subject = decodeHeader(h.Value)
			case "date":
				e.Date = h.Value
			}
		}
	}
	return e
}

// decodeHeader decodes RFC 2047 encoded-word headers, falling back to the
// raw value when decoding fails.
func decodeHeader(s string) string {
	dec := new(mime.WordDecoder)
	if out, err := dec.DecodeHeader(s); err == nil {
		return out
	}
	return s
}

// extractEmail pulls the bare address out of a From header, handling both
// "Display Name <addr@host>" and a bare "addr@host".
func extractEmail(header string) string {
	header = strings.TrimSpace(header)
	if i := strings.LastIndex(header, "<"); i >= 0 {
		if j := strings.Index(header[i:], ">"); j >= 0 {
			return strings.TrimSpace(header[i+1 : i+j])
		}
	}
	return header
}

// recipientAddress extracts the destination address from a channel id,
// stripping an optional "gmail:" platform prefix and any display name.
func recipientAddress(channelID string) string {
	channelID = strings.TrimSpace(channelID)
	if rest, ok := strings.CutPrefix(channelID, platform+":"); ok {
		channelID = rest
	}
	return extractEmail(channelID)
}

// buildMessage assembles a base64url-encoded RFC 5322 message. The first
// line of content is the subject; the remainder is the body.
func buildMessage(from, to, content string) string {
	subject, body, ok := strings.Cut(content, "\n")
	if !ok {
		body = content
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "(no subject)"
	}
	body = strings.TrimLeft(body, "\n")

	var b strings.Builder
	if from != "" {
		fmt.Fprintf(&b, "From: %s\r\n", from)
	}
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)

	return base64.URLEncoding.EncodeToString([]byte(b.String()))
}
