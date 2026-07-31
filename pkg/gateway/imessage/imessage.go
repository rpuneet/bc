// Package imessage implements a gateway.NotificationAdapter that polls
// the BlueBubbles API for new iMessage messages.
package imessage

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

// Adapter implements gateway.NotificationAdapter for iMessage via BlueBubbles.
type Adapter struct {
	httpClient   *http.Client
	handler      func(gateway.Notification)
	lastFetchAt  time.Time
	name         string
	apiURL       string
	password     string
	lastError    string
	lastTS       int64
	messageCount atomic.Int64
	mu           sync.Mutex
	interval     int
	connected    bool
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates an iMessage adapter with the default name.
func New(apiURL, password string, intervalSeconds int) *Adapter {
	return NewNamed("imessage", apiURL, password, intervalSeconds)
}

// NewNamed creates a named iMessage adapter.
func NewNamed(name, apiURL, password string, intervalSeconds int) *Adapter {
	if intervalSeconds <= 0 {
		intervalSeconds = defaultInterval
	}
	return &Adapter{
		name:       name,
		apiURL:     apiURL,
		password:   password,
		interval:   intervalSeconds,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterPoll }
func (a *Adapter) HTTPHandler() http.Handler { return nil }
func (a *Adapter) Stop() error               { return nil }

// Start polls the BlueBubbles API until ctx is canceled.
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
	return []gateway.ChannelInfo{{ID: "messages", Name: "messages", Platform: "imessage"}}
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

// poll fetches recent messages from the BlueBubbles API.
func (a *Adapter) poll(ctx context.Context) {
	url := fmt.Sprintf("%s/api/v1/message?password=%s&limit=10&sort=desc", a.apiURL, a.password)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		a.setError(fmt.Sprintf("build request: %v", err))
		return
	}

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

	var result struct {
		Data []struct {
			Handle struct {
				Address string `json:"address"`
			} `json:"handle"`
			Text        string `json:"text"`
			DateCreated int64  `json:"dateCreated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return
	}

	var count int
	for _, msg := range result.Data {
		if msg.DateCreated <= a.lastTS {
			continue
		}

		sender := msg.Handle.Address
		if sender == "" {
			sender = "imessage"
		}

		raw, _ := json.Marshal(msg) //nolint:errcheck
		a.messageCount.Add(1)
		count++

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   "messages",
				Platform:  "imessage",
				Sender:    sender,
				Content:   msg.Text,
				Timestamp: time.Now(),
				Raw:       raw,
			})
		}
	}

	// Update last timestamp.
	if len(result.Data) > 0 && result.Data[0].DateCreated > a.lastTS {
		a.mu.Lock()
		a.lastTS = result.Data[0].DateCreated
		a.mu.Unlock()
	}

	log.Debug("imessage: poll complete", "adapter", a.name, "messages", count)
}

func (a *Adapter) setError(msg string) {
	a.mu.Lock()
	a.lastError = msg
	a.mu.Unlock()
	log.Warn("imessage: "+msg, "adapter", a.name)
}
