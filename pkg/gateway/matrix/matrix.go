// Package matrix implements a gateway.NotificationAdapter that polls
// the Matrix client-server API /sync endpoint for new events.
package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

const defaultInterval = 10 // seconds

// Adapter implements gateway.NotificationAdapter for Matrix via /sync.
type Adapter struct { //nolint:govet
	httpClient   *http.Client
	handler      func(gateway.Notification)
	lastFetchAt  time.Time
	name         string
	homeserver   string
	token        string
	userID       string // the bot's own MXID, resolved lazily via /whoami; used to skip its own echoes
	nextBatch    string
	lastError    string
	messageCount atomic.Int64
	mu           sync.Mutex
	interval     int
	connected    bool
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a Matrix adapter with the default name.
func New(homeserver, token string, intervalSeconds int) *Adapter {
	return NewNamed("matrix", homeserver, token, intervalSeconds)
}

// NewNamed creates a named Matrix adapter.
func NewNamed(name, homeserver, token string, intervalSeconds int) *Adapter {
	if intervalSeconds <= 0 {
		intervalSeconds = defaultInterval
	}
	return &Adapter{
		name:       name,
		homeserver: homeserver,
		token:      token,
		interval:   intervalSeconds,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterPoll }
func (a *Adapter) HTTPHandler() http.Handler { return nil }
func (a *Adapter) Stop() error               { return nil }

// Start polls the Matrix /sync endpoint until ctx is canceled.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler

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

// Channels returns a single channel.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{{ID: "rooms", Name: "rooms", Platform: "matrix"}}
}

// Status returns the adapter's connection state.
func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{
		Connected:     a.connected,
		LastMessageAt: a.lastFetchAt,
		Error:         a.lastError,
		MessageCount:  a.messageCount.Load(),
	}
}

// poll fetches new events from the Matrix /sync endpoint.
func (a *Adapter) poll(ctx context.Context) {
	url := fmt.Sprintf("%s/_matrix/client/v3/sync?timeout=0", a.homeserver)
	if a.nextBatch != "" {
		url += "&since=" + a.nextBatch
	}

	// Resolve the bot's own MXID once so we can skip echoing its own messages.
	if a.userID == "" {
		a.userID = a.fetchUserID(ctx)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		a.setError(fmt.Sprintf("build request: %v", err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.token)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		a.setError(fmt.Sprintf("fetch: %v", err))
		return
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		a.setError(fmt.Sprintf("read body: %v", err))
		return
	}

	a.mu.Lock()
	a.connected = true
	a.lastFetchAt = time.Now()
	a.lastError = ""
	a.mu.Unlock()

	notes, nextBatch, err := parseSync(body, a.userID)
	if err != nil {
		return
	}

	a.mu.Lock()
	a.nextBatch = nextBatch
	a.mu.Unlock()

	for _, n := range notes {
		a.messageCount.Add(1)
		if a.handler != nil {
			a.handler(n)
		}
	}

	log.Debug("matrix: poll complete", "adapter", a.name, "events", len(notes))
}

// fetchUserID resolves the bot's own MXID via the /whoami endpoint. Best
// effort: on any failure it returns "" and self-filtering is simply skipped
// for that poll (the next poll retries).
func (a *Adapter) fetchUserID(ctx context.Context) string {
	url := fmt.Sprintf("%s/_matrix/client/v3/account/whoami", a.homeserver)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close() //nolint:errcheck
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	var who struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(body, &who); err != nil {
		return ""
	}
	return who.UserID
}

// parseSync decodes a Matrix /sync response into notifications. It keeps only
// m.room.message events, skips messages sent by botUserID (the bot's own
// echoes), sets Content from the event's content.body, and preserves the FULL
// raw event JSON in Raw. It returns the notifications and the next_batch token.
func parseSync(body []byte, botUserID string) (notes []gateway.Notification, nextBatch string, err error) {
	var syncResp struct { //nolint:govet // inline JSON decode struct; field order matches JSON schema
		NextBatch string `json:"next_batch"`
		Rooms     struct {
			Join map[string]struct {
				Timeline struct {
					Events []json.RawMessage `json:"events"`
				} `json:"timeline"`
			} `json:"join"`
		} `json:"rooms"`
	}
	if err := json.Unmarshal(body, &syncResp); err != nil {
		return nil, "", err
	}

	for roomID, room := range syncResp.Rooms.Join {
		for _, rawEvt := range room.Timeline.Events {
			var evt struct {
				Type    string `json:"type"`
				Sender  string `json:"sender"`
				Content struct {
					Body string `json:"body"`
				} `json:"content"`
			}
			if err := json.Unmarshal(rawEvt, &evt); err != nil {
				continue
			}
			if evt.Type != "m.room.message" {
				continue // ignore membership, typing, receipts, etc.
			}
			if botUserID != "" && evt.Sender == botUserID {
				continue // don't echo the bot's own messages
			}
			sender := evt.Sender
			if sender == "" {
				sender = "matrix"
			}
			notes = append(notes, gateway.Notification{
				Channel:   roomID,
				Platform:  "matrix",
				Sender:    sender,
				Content:   evt.Content.Body,
				Timestamp: time.Now(),
				Raw:       append(json.RawMessage(nil), rawEvt...),
			})
		}
	}
	return notes, syncResp.NextBatch, nil
}

func (a *Adapter) setError(msg string) {
	a.mu.Lock()
	a.lastError = msg
	a.mu.Unlock()
	log.Warn("matrix: "+msg, "adapter", a.name)
}
