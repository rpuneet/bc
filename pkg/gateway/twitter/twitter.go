// Package twitter implements a gateway.NotificationAdapter that polls
// the Twitter API v2 mentions endpoint for new mentions.
package twitter

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

const defaultInterval = 60 // seconds

// Adapter implements gateway.NotificationAdapter for Twitter API v2.
type Adapter struct {
	httpClient   *http.Client
	handler      func(gateway.Notification)
	lastFetchAt  time.Time
	name         string
	bearerToken  string
	userID       string
	sinceID      string
	lastError    string
	messageCount atomic.Int64
	mu           sync.Mutex
	interval     int
	connected    bool
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a Twitter adapter with the default name.
func New(bearerToken, userID string, intervalSeconds int) *Adapter {
	return NewNamed("twitter", bearerToken, userID, intervalSeconds)
}

// NewNamed creates a named Twitter adapter.
func NewNamed(name, bearerToken, userID string, intervalSeconds int) *Adapter {
	if intervalSeconds <= 0 {
		intervalSeconds = defaultInterval
	}
	return &Adapter{
		name:        name,
		bearerToken: bearerToken,
		userID:      userID,
		interval:    intervalSeconds,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType  { return gateway.AdapterPoll }
func (a *Adapter) HTTPHandler() http.Handler  { return nil }
func (a *Adapter) Stop() error                { return nil }

// Start polls the Twitter mentions endpoint until ctx is canceled.
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

// Channels returns a single "mentions" channel.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{{ID: "mentions", Name: "mentions", Platform: "twitter"}}
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

// poll fetches recent mentions from the Twitter API v2.
func (a *Adapter) poll(ctx context.Context) {
	url := fmt.Sprintf("https://api.twitter.com/2/users/%s/mentions?tweet.fields=author_id,created_at", a.userID)
	if a.sinceID != "" {
		url += "&since_id=" + a.sinceID
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		a.setError(fmt.Sprintf("build request: %v", err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.bearerToken)

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
			ID       string `json:"id"`
			AuthorID string `json:"author_id"`
			Text     string `json:"text"`
		} `json:"data"`
		Meta struct {
			NewestID string `json:"newest_id"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return
	}

	for _, tweet := range result.Data {
		sender := tweet.AuthorID
		if sender == "" {
			sender = "twitter"
		}

		raw, _ := json.Marshal(tweet) //nolint:errcheck
		a.messageCount.Add(1)

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   "mentions",
				Platform:  "twitter",
				Sender:    sender,
				Timestamp: time.Now(),
				Raw:       raw,
			})
		}
	}

	if result.Meta.NewestID != "" {
		a.mu.Lock()
		a.sinceID = result.Meta.NewestID
		a.mu.Unlock()
	}

	log.Debug("twitter: poll complete", "adapter", a.name, "tweets", len(result.Data))
}

func (a *Adapter) setError(msg string) {
	a.mu.Lock()
	a.lastError = msg
	a.mu.Unlock()
	log.Warn("twitter: "+msg, "adapter", a.name)
}
