// Package reddit implements a gateway.NotificationAdapter that polls
// the Reddit API for new posts and comments in a subreddit.
package reddit

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

// Adapter implements gateway.NotificationAdapter for Reddit API.
type Adapter struct {
	httpClient   *http.Client
	handler      func(gateway.Notification)
	seen         map[string]struct{}
	lastFetchAt  time.Time
	name         string
	subreddit    string
	bearerToken  string
	lastError    string
	messageCount atomic.Int64
	mu           sync.Mutex
	interval     int
	connected    bool
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates a Reddit adapter with the default name.
func New(subreddit, bearerToken string, intervalSeconds int) *Adapter {
	return NewNamed("reddit", subreddit, bearerToken, intervalSeconds)
}

// NewNamed creates a named Reddit adapter.
func NewNamed(name, subreddit, bearerToken string, intervalSeconds int) *Adapter {
	if intervalSeconds <= 0 {
		intervalSeconds = defaultInterval
	}
	return &Adapter{
		name:        name,
		subreddit:   subreddit,
		bearerToken: bearerToken,
		interval:    intervalSeconds,
		seen:        make(map[string]struct{}),
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterPoll }
func (a *Adapter) HTTPHandler() http.Handler { return nil }
func (a *Adapter) Stop() error               { return nil }

// Start polls Reddit for new posts until ctx is canceled.
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

// Channels returns a single channel for the subreddit.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{{ID: a.subreddit, Name: a.subreddit, Platform: "reddit"}}
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

// poll fetches new posts from a subreddit.
func (a *Adapter) poll(ctx context.Context) {
	url := fmt.Sprintf("https://oauth.reddit.com/r/%s/new.json?limit=10", a.subreddit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		a.setError(fmt.Sprintf("build request: %v", err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.bearerToken)
	req.Header.Set("User-Agent", "bc-gateway/1.0")

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

	var listing struct {
		Data struct {
			Children []struct {
				Data struct {
					ID     string `json:"id"`
					Author string `json:"author"`
					Title  string `json:"title"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &listing); err != nil {
		a.setError(fmt.Sprintf("decode: %v", err))
		return
	}

	a.mu.Lock()
	a.connected = true
	a.lastFetchAt = time.Now()
	a.lastError = ""
	a.mu.Unlock()

	var count int
	for _, child := range listing.Data.Children {
		post := child.Data
		a.mu.Lock()
		_, dup := a.seen[post.ID]
		if !dup {
			a.seen[post.ID] = struct{}{}
		}
		a.mu.Unlock()
		if dup {
			continue
		}

		sender := post.Author
		if sender == "" {
			sender = "reddit"
		}

		raw, _ := json.Marshal(post) //nolint:errcheck
		a.messageCount.Add(1)
		count++

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   a.subreddit,
				Platform:  "reddit",
				Sender:    sender,
				Timestamp: time.Now(),
				Raw:       raw,
			})
		}
	}

	log.Debug("reddit: poll complete", "adapter", a.name, "posts", count)
}

func (a *Adapter) setError(msg string) {
	a.mu.Lock()
	a.lastError = msg
	a.mu.Unlock()
	log.Warn("reddit: "+msg, "adapter", a.name)
}
