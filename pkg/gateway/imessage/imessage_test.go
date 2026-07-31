package imessage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// TestPollSetsContent verifies that a BlueBubbles message payload yields a
// notification whose Content is the message text (the decode struct previously
// omitted the text field, so the agent saw no body).
func TestPollSetsContent(t *testing.T) {
	const payload = `{"data":[{"handle":{"address":"+15550001111"},"text":"hi from imessage","dateCreated":12345}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload)) //nolint:errcheck
	}))
	defer srv.Close()

	a := New(srv.URL, "pw", 10)
	var got gateway.Notification
	a.handler = func(n gateway.Notification) { got = n }
	a.poll(context.Background())

	if got.Content != "hi from imessage" {
		t.Errorf("Content = %q, want %q", got.Content, "hi from imessage")
	}
	if got.Sender != "+15550001111" {
		t.Errorf("Sender = %q, want %q", got.Sender, "+15550001111")
	}
}
