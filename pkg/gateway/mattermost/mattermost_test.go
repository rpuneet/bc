package mattermost

import (
	"encoding/json"
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// frame builds a raw Mattermost "posted" websocket frame carrying a post from
// the given user.
func frame(t *testing.T, userID, message string) []byte {
	t.Helper()
	post, err := json.Marshal(map[string]string{
		"message":    message,
		"channel_id": "c1",
		"user_id":    userID,
	})
	if err != nil {
		t.Fatalf("marshal post: %v", err)
	}
	evt, err := json.Marshal(map[string]any{
		"event": "posted",
		"data": map[string]string{
			"post":         string(post),
			"channel_name": "town-square",
			"sender_name":  "someone",
		},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return evt
}

// TestHandleRawSelfFilter verifies the bot does not echo its own posts: a post
// from the bot's own user id produces no notification, while a post from any
// other user does and carries the message text as Content.
func TestHandleRawSelfFilter(t *testing.T) {
	a := New("mm", Config{URL: "https://mm.example.com", Token: "tok"})
	a.botUserID = "botid"

	var count int
	var got gateway.Notification
	a.handler = func(n gateway.Notification) {
		count++
		got = n
	}

	// Own post — must be dropped.
	a.handleRaw(frame(t, "botid", "my own message"))
	if count != 0 {
		t.Fatalf("bot's own post was not filtered: got %d notifications", count)
	}

	// Someone else's post — must be forwarded with Content set.
	a.handleRaw(frame(t, "u2", "hello team"))
	if count != 1 {
		t.Fatalf("got %d notifications, want 1", count)
	}
	if got.Content != "hello team" {
		t.Errorf("Content = %q, want %q", got.Content, "hello team")
	}
}
