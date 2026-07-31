package signal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// TestPollSetsContent verifies that a signal-cli receive payload yields a
// notification whose Content is the parsed dataMessage text (previously
// dropped, leaving the agent with an empty body).
func TestPollSetsContent(t *testing.T) {
	const payload = `[{"envelope":{"source":"+15551234567","dataMessage":{"message":"hello there"}}}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload)) //nolint:errcheck
	}))
	defer srv.Close()

	a := New(srv.URL, 10)
	var got gateway.Notification
	a.handler = func(n gateway.Notification) { got = n }
	a.poll(context.Background())

	if got.Content != "hello there" {
		t.Errorf("Content = %q, want %q", got.Content, "hello there")
	}
	if got.Sender != "+15551234567" {
		t.Errorf("Sender = %q, want %q", got.Sender, "+15551234567")
	}
}
