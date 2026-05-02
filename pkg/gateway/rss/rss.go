// Package rss implements a gateway.NotificationAdapter that polls
// RSS 2.0 and Atom feeds on a configurable interval.
package rss

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/log"
)

const defaultInterval = 300 // 5 minutes

// feedEntry is the normalized representation of a single feed item,
// marshaled into Notification.Raw as JSON.
type feedEntry struct {
	Title       string `json:"title"`
	Link        string `json:"link"`
	Description string `json:"description,omitempty"`
	PubDate     string `json:"pub_date,omitempty"`
	Author      string `json:"author,omitempty"`
	GUID        string `json:"guid,omitempty"`
}

// ---- RSS 2.0 XML structures ----

type rssRoot struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"author"`
	Creator     string `xml:"creator"` // dc:creator
	GUID        string `xml:"guid"`
}

// ---- Atom XML structures ----

type atomFeed struct {
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string     `xml:"title"`
	Link    atomLink   `xml:"link"`
	Summary string     `xml:"summary"`
	Content string     `xml:"content"`
	Updated string     `xml:"updated"`
	Author  atomAuthor `xml:"author"`
	ID      string     `xml:"id"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

// Adapter implements gateway.NotificationAdapter for RSS/Atom feeds.
type Adapter struct {
	httpClient   *http.Client
	handler      func(gateway.Notification)
	seen         map[string]struct{}
	lastFetchAt  time.Time
	name         string
	url          string
	feedTitle    string
	lastError    string
	messageCount atomic.Int64
	mu           sync.Mutex
	interval     int
	connected    bool
}

var _ gateway.NotificationAdapter = (*Adapter)(nil)

// New creates an RSS adapter with the default name "rss".
func New(url string, intervalSeconds int) *Adapter {
	return NewNamed("rss", url, intervalSeconds)
}

// NewNamed creates a named RSS adapter (e.g. "rss:blog").
func NewNamed(name, url string, intervalSeconds int) *Adapter {
	if intervalSeconds <= 0 {
		intervalSeconds = defaultInterval
	}
	return &Adapter{
		name:       name,
		url:        url,
		interval:   intervalSeconds,
		seen:       make(map[string]struct{}),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Adapter) Name() string              { return a.name }
func (a *Adapter) Type() gateway.AdapterType { return gateway.AdapterPoll }
func (a *Adapter) HTTPHandler() http.Handler { return nil }

// Start polls the feed on the configured interval until ctx is canceled.
func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
	a.handler = handler

	// Initial fetch.
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

// Channels returns a single channel representing the feed.
func (a *Adapter) Channels() []gateway.ChannelInfo {
	a.mu.Lock()
	title := a.feedTitle
	a.mu.Unlock()
	if title == "" {
		title = a.name
	}
	return []gateway.ChannelInfo{
		{
			ID:       a.url,
			Name:     title,
			Platform: "rss",
		},
	}
}

// Status returns connection state for the web UI.
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

// poll fetches the feed and dispatches new entries.
func (a *Adapter) poll(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.url, nil)
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

	title, entries := parseFeed(body)

	a.mu.Lock()
	a.connected = true
	a.lastFetchAt = time.Now()
	a.lastError = ""
	if title != "" {
		a.feedTitle = title
	}
	a.mu.Unlock()

	for i := range entries {
		e := &entries[i]
		key := entryKey(e)
		a.mu.Lock()
		_, dup := a.seen[key]
		if !dup {
			a.seen[key] = struct{}{}
		}
		a.mu.Unlock()
		if dup {
			continue
		}

		raw, _ := json.Marshal(e) //nolint:errcheck
		a.messageCount.Add(1)

		author := e.Author
		if author == "" {
			author = "rss"
		}

		if a.handler != nil {
			a.handler(gateway.Notification{
				Channel:   title,
				Platform:  "rss",
				Sender:    author,
				Timestamp: time.Now(),
				Raw:       raw,
			})
		}
	}

	log.Debug("rss: poll complete", "adapter", a.name, "entries", len(entries))
}

func (a *Adapter) setError(msg string) {
	a.mu.Lock()
	a.lastError = msg
	a.mu.Unlock()
	log.Warn("rss: "+msg, "adapter", a.name)
}

// parseFeed tries RSS 2.0 first, then Atom.
func parseFeed(data []byte) (title string, entries []feedEntry) {
	// Try RSS 2.0.
	var rss rssRoot
	if err := xml.Unmarshal(data, &rss); err == nil && len(rss.Channel.Items) > 0 {
		title = rss.Channel.Title
		entries = make([]feedEntry, 0, len(rss.Channel.Items))
		for _, it := range rss.Channel.Items {
			author := it.Author
			if author == "" {
				author = it.Creator
			}
			entries = append(entries, feedEntry{
				Title:       it.Title,
				Link:        it.Link,
				Description: it.Description,
				PubDate:     it.PubDate,
				Author:      author,
				GUID:        it.GUID,
			})
		}
		return title, entries
	}

	// Try Atom.
	var atom atomFeed
	if err := xml.Unmarshal(data, &atom); err == nil && len(atom.Entries) > 0 {
		title = atom.Title
		entries = make([]feedEntry, 0, len(atom.Entries))
		for _, e := range atom.Entries {
			desc := e.Summary
			if desc == "" {
				desc = e.Content
			}
			entries = append(entries, feedEntry{
				Title:       e.Title,
				Link:        e.Link.Href,
				Description: desc,
				PubDate:     e.Updated,
				Author:      e.Author.Name,
				GUID:        e.ID,
			})
		}
		return title, entries
	}

	return "", nil
}

// entryKey returns a dedup key: GUID if present, else link.
func entryKey(e *feedEntry) string {
	if e.GUID != "" {
		return e.GUID
	}
	return e.Link
}
