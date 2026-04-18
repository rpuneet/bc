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

	"github.com/rpuneet/bc/pkg/gateway"
	"github.com/rpuneet/bc/pkg/log"
)

const defaultInterval = 10 // seconds

// Adapter implements gateway.NotificationAdapter for Matrix via /sync.
type Adapter struct {
	httpClient   *http.Client
	handler      func(gateway.Notification)
	lastFetchAt  time.Time
	name         string
	homeserver   string
	token        string
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

	var syncResp struct {
		NextBatch string `json:"next_batch"`
		Rooms     struct {
			Join map[string]struct {
				Timeline struct {
					Events []struct {
						Type   string `json:"type"`
						Sender string `json:"sender"`
					} `json:"events"`
				} `json:"timeline"`
			} `json:"join"`
		} `json:"rooms"`
	}
	if err := json.Unmarshal(body, &syncResp); err != nil {
		return
	}

	a.mu.Lock()
	a.nextBatch = syncResp.NextBatch
	a.mu.Unlock()

	var count int
	for roomID, room := range syncResp.Rooms.Join {
		for _, evt := range room.Timeline.Events {
			sender := evt.Sender
			if sender == "" {
				sender = "matrix"
			}

			raw, _ := json.Marshal(evt) //nolint:errcheck
			a.messageCount.Add(1)
			count++

			if a.handler != nil {
				a.handler(gateway.Notification{
					Channel:   roomID,
					Platform:  "matrix",
					Sender:    sender,
					Timestamp: time.Now(),
					Raw:       raw,
				})
			}
		}
	}

	log.Debug("matrix: poll complete", "adapter", a.name, "events", count)
}

func (a *Adapter) setError(msg string) {
	a.mu.Lock()
	a.lastError = msg
	a.mu.Unlock()
	log.Warn("matrix: "+msg, "adapter", a.name)
}
