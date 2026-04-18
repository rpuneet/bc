// Package notion implements a gateway.NotificationAdapter that polls
// the Notion API for recently updated pages and databases.
package notion

import (
	"bytes"
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

const (
	defaultInterval = 300 // 5 minutes
	notionAPIURL    = "https://api.notion.com/v1/search"
	notionVersion   = "2022-06-28"
)

// searchResult represents a single result from Notion's search API.
type searchResult struct {
	ID             string `json:"id"`
	Object         string `json:"object"` // "page" or "database"
	LastEditedTime string `json:"last_edited_time"`
	LastEditedBy   struct {
		Name string `json:"name"`
	} `json:"last_edited_by"`
	// URL is used as part of the dedup key.
	URL string `json:"url"`
}

// Adapter implements gateway.NotificationAdapter for Notion polling.
type Adapter struct {
	httpClient  *http.Client
	handler     func(gateway.Notification)
	seen        map[string]string // id → last_edited_time
	lastPollAt  time.Time
	name        string
	token       string
	lastError   string
	mu          sync.Mutex
	interval    int
	connected   bool
	msgCount    atomic.Int64
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a Notion poll adapter with the default name.
func New(token string, intervalSeconds int) *Adapter {
	return NewNamed("notion", token, intervalSeconds)
}

// NewNamed creates a named Notion adapter (e.g. "notion:wiki").
func NewNamed(name, token string, intervalSeconds int) *Adapter {
	if intervalSeconds <= 0 {
		intervalSeconds = defaultInterval
	}
	return &Adapter{
		name:       name,
		token:      token,
		interval:   intervalSeconds,
		seen:       make(map[string]string),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterPoll }
func (a *Adapter) HTTPHandler() http.Handler { return nil }

// Start polls the Notion API on the configured interval until ctx is canceled.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler

	// Initial poll.
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

// Channels returns the discoverable event channels.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{
		{ID: "page_updated", Name: "page_updated", Platform: "notion"},
		{ID: "database_updated", Name: "database_updated", Platform: "notion"},
	}
}

// Status returns connection state for the web UI.
func (a *Adapter) Status() gateway.AdapterStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return gateway.AdapterStatus{
		Connected:     a.connected,
		LastMessageAt: a.lastPollAt,
		Error:         a.lastError,
		MessageCount:  a.msgCount.Load(),
	}
}

// poll calls Notion search API and dispatches notifications for new/updated items.
func (a *Adapter) poll(ctx context.Context) {
	payload := []byte(`{"sort":{"direction":"descending","timestamp":"last_edited_time"}}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, notionAPIURL, bytes.NewReader(payload))
	if err != nil {
		a.setError(fmt.Sprintf("build request: %v", err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", notionVersion)

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

	if resp.StatusCode != http.StatusOK {
		a.setError(fmt.Sprintf("notion API: %d", resp.StatusCode))
		return
	}

	var result struct {
		Results []searchResult `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		a.setError(fmt.Sprintf("parse: %v", err))
		return
	}

	a.mu.Lock()
	a.connected = true
	a.lastPollAt = time.Now()
	a.lastError = ""
	a.mu.Unlock()

	for i := range result.Results {
		r := &result.Results[i]
		a.mu.Lock()
		prev, exists := a.seen[r.ID]
		a.seen[r.ID] = r.LastEditedTime
		a.mu.Unlock()

		if exists && prev == r.LastEditedTime {
			continue // unchanged
		}

		channel := "page_updated"
		if r.Object == "database" {
			channel = "database_updated"
		}

		sender := r.LastEditedBy.Name
		if sender == "" {
			sender = "notion"
		}

		raw, _ := json.Marshal(r) //nolint:errcheck
		a.msgCount.Add(1)

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   channel,
				Platform:  "notion",
				Sender:    sender,
				Timestamp: time.Now(),
				Raw:       raw,
			})
		}
	}

	log.Debug("notion: poll complete", "adapter", a.name, "results", len(result.Results))
}

func (a *Adapter) setError(msg string) {
	a.mu.Lock()
	a.lastError = msg
	a.mu.Unlock()
	log.Warn("notion: "+msg, "adapter", a.name)
}
