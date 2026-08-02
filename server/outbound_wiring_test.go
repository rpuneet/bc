package server_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/server"
)

// stubSendingAdapter is a minimal gateway adapter that accepts outbound
// messages, standing in for Slack or WhatsApp.
type stubSendingAdapter struct {
	name string
	sent int
}

func (s *stubSendingAdapter) Name() string                                            { return s.name }
func (s *stubSendingAdapter) Type() gateway.AdapterType                               { return gateway.AdapterSocket }
func (s *stubSendingAdapter) Start(context.Context, func(gateway.Notification)) error { return nil }
func (s *stubSendingAdapter) Stop() error                                             { return nil }
func (s *stubSendingAdapter) HTTPHandler() http.Handler                               { return nil }
func (s *stubSendingAdapter) Channels() []gateway.ChannelInfo                         { return nil }
func (s *stubSendingAdapter) Status() gateway.AdapterStatus                           { return gateway.AdapterStatus{} }

func (s *stubSendingAdapter) Send(_ context.Context, _, _, _ string) error {
	s.sent++
	return nil
}

// TestServerWiresOutboundRecording covers the one line of production code that
// connects gateway sends to notify's message store (server.New →
// SetOutboundHandler). The unit tests on either side of that seam both pass
// even if the wiring is deleted, which would silently restore the one-sided
// history of #3462 — so the seam itself needs a test.
func TestServerWiresOutboundRecording(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	store, err := notify.OpenStore(d, "sqlite")
	if err != nil {
		t.Fatal(err)
	}

	gw := gateway.NewManager()
	adapter := &stubSendingAdapter{name: "slack"}
	gw.Register(adapter)

	// server.New performs the wiring under test.
	_ = server.New(
		server.Config{Addr: "127.0.0.1:0"},
		server.Services{Gateway: gw, Notify: notify.NewService(store, nil, nil)},
		nil, nil,
	)

	sent, err := gw.Send(context.Background(), "slack:general", "fast-crane", "wired end to end")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !sent {
		t.Fatal("Send() reported the channel as unhandled")
	}
	if adapter.sent != 1 {
		t.Fatalf("adapter received %d sends, want 1", adapter.sent)
	}

	msgs, err := store.GetMessages(context.Background(), "slack:general", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("outbound message was not recorded: got %d rows in channel history", len(msgs))
	}
	if msgs[0].Sender != "fast-crane" || msgs[0].Content != "wired end to end" {
		t.Errorf("recorded message = %q from %q", msgs[0].Content, msgs[0].Sender)
	}
}

// TestServerOutboundWiringToleratesMissingNotify: a degraded boot can leave
// Notify nil, and a send must still work rather than panic on the hook.
func TestServerOutboundWiringToleratesMissingNotify(t *testing.T) {
	gw := gateway.NewManager()
	adapter := &stubSendingAdapter{name: "slack"}
	gw.Register(adapter)

	_ = server.New(
		server.Config{Addr: "127.0.0.1:0"},
		server.Services{Gateway: gw},
		nil, nil,
	)

	sent, err := gw.Send(context.Background(), "slack:general", "fast-crane", "no notify service")
	if err != nil || !sent {
		t.Fatalf("Send() = (%v, %v), want (true, nil) with Notify nil", sent, err)
	}
}
