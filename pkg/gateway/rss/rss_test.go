package rss

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/gateway"
)

const testRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Blog</title>
    <item>
      <title>First Post</title>
      <link>https://example.com/1</link>
      <description>Hello world</description>
      <pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
      <author>alice</author>
      <guid>guid-1</guid>
    </item>
    <item>
      <title>Second Post</title>
      <link>https://example.com/2</link>
      <description>Another post</description>
      <author>bob</author>
      <guid>guid-2</guid>
    </item>
  </channel>
</rss>`

const testAtom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Feed</title>
  <entry>
    <title>Atom Entry</title>
    <link href="https://example.com/atom/1"/>
    <summary>Atom summary</summary>
    <updated>2024-01-01T00:00:00Z</updated>
    <author><name>carol</name></author>
    <id>atom-1</id>
  </entry>
</feed>`

func TestParseFeed_RSS(t *testing.T) {
	title, entries := parseFeed([]byte(testRSS))
	if title != "Test Blog" {
		t.Fatalf("expected title %q, got %q", "Test Blog", title)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	e := entries[0]
	if e.Title != "First Post" {
		t.Errorf("title = %q", e.Title)
	}
	if e.Link != "https://example.com/1" {
		t.Errorf("link = %q", e.Link)
	}
	if e.Author != "alice" {
		t.Errorf("author = %q", e.Author)
	}
	if e.GUID != "guid-1" {
		t.Errorf("guid = %q", e.GUID)
	}
}

func TestParseFeed_Atom(t *testing.T) {
	title, entries := parseFeed([]byte(testAtom))
	if title != "Atom Feed" {
		t.Fatalf("expected title %q, got %q", "Atom Feed", title)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Title != "Atom Entry" {
		t.Errorf("title = %q", e.Title)
	}
	if e.Link != "https://example.com/atom/1" {
		t.Errorf("link = %q", e.Link)
	}
	if e.Author != "carol" {
		t.Errorf("author = %q", e.Author)
	}
	if e.GUID != "atom-1" {
		t.Errorf("guid = %q", e.GUID)
	}
}

func TestDuplicateDetection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testRSS))
	}))
	defer srv.Close()

	a := New(srv.URL, 60)

	var mu sync.Mutex
	var got []gateway.Notification
	handler := func(n gateway.Notification) {
		mu.Lock()
		got = append(got, n)
		mu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a.handler = handler

	// First poll should emit 2 entries.
	a.poll(ctx)
	mu.Lock()
	count1 := len(got)
	mu.Unlock()
	if count1 != 2 {
		t.Fatalf("first poll: expected 2 notifications, got %d", count1)
	}

	// Second poll with same content should emit 0.
	a.poll(ctx)
	mu.Lock()
	count2 := len(got)
	mu.Unlock()
	if count2 != 2 {
		t.Fatalf("second poll: expected still 2 notifications, got %d", count2)
	}
}

func TestNotificationConstruction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testRSS))
	}))
	defer srv.Close()

	a := New(srv.URL, 60)

	var mu sync.Mutex
	var got []gateway.Notification
	handler := func(n gateway.Notification) {
		mu.Lock()
		got = append(got, n)
		mu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a.handler = handler
	a.poll(ctx)

	mu.Lock()
	defer mu.Unlock()

	if len(got) < 1 {
		t.Fatal("no notifications received")
	}

	n := got[0]
	if n.Platform != "rss" {
		t.Errorf("platform = %q, want %q", n.Platform, "rss")
	}
	if n.Channel != "Test Blog" {
		t.Errorf("channel = %q, want %q", n.Channel, "Test Blog")
	}
	if n.Sender != "alice" {
		t.Errorf("sender = %q, want %q", n.Sender, "alice")
	}
	if n.Timestamp.IsZero() {
		t.Error("timestamp is zero")
	}
	if len(n.Raw) == 0 {
		t.Error("raw is empty")
	}
}

func TestAdapterMeta(t *testing.T) {
	a := NewNamed("rss:blog", "https://example.com/feed.xml", 0)
	if a.Name() != "rss:blog" {
		t.Errorf("name = %q", a.Name())
	}
	if a.Type() != gateway.AdapterPoll {
		t.Errorf("type = %q", a.Type())
	}
	if a.HTTPHandler() != nil {
		t.Error("HTTPHandler should be nil for poll adapter")
	}

	// Default interval should be 300.
	if a.interval != defaultInterval {
		t.Errorf("interval = %d, want %d", a.interval, defaultInterval)
	}
}

func TestStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testRSS))
	}))
	defer srv.Close()

	a := New(srv.URL, 60)

	// Before any poll.
	s := a.Status()
	if s.Connected {
		t.Error("should not be connected before first poll")
	}

	ctx := context.Background()
	a.handler = func(_ gateway.Notification) {}
	a.poll(ctx)

	s = a.Status()
	if !s.Connected {
		t.Error("should be connected after successful poll")
	}
	if s.MessageCount != 2 {
		t.Errorf("message count = %d, want 2", s.MessageCount)
	}
	if s.LastMessageAt.Before(time.Now().Add(-5 * time.Second)) {
		t.Error("last message time too old")
	}
}

func TestChannels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testRSS))
	}))
	defer srv.Close()

	a := New(srv.URL, 60)
	a.handler = func(_ gateway.Notification) {}
	a.poll(context.Background())

	chs := a.Channels()
	if len(chs) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(chs))
	}
	if chs[0].Name != "Test Blog" {
		t.Errorf("channel name = %q", chs[0].Name)
	}
	if chs[0].ID != srv.URL {
		t.Errorf("channel ID = %q", chs[0].ID)
	}
	if chs[0].Platform != "rss" {
		t.Errorf("platform = %q", chs[0].Platform)
	}
}
