package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// reactionStub wraps stubAdapter and records SendReaction calls.
//
//nolint:govet // fieldalignment: test-only struct, compactness is not a concern
type reactionStub struct {
	stubAdapter
	calls []reactionCall
	err   error
	mu    sync.Mutex
}

type reactionCall struct {
	ChannelID string
	SenderJID string
	MessageID string
	Emoji     string
}

func (r *reactionStub) SendReaction(_ context.Context, channelID, senderJID, messageID, emoji string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, reactionCall{channelID, senderJID, messageID, emoji})
	return r.err
}

func (r *reactionStub) getCalls() []reactionCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]reactionCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// buildReactRequest builds a POST /api/apps/whatsapp/react request.
func buildReactRequest(t *testing.T, body map[string]any) *http.Request {
	t.Helper()
	b, _ := json.Marshal(body) //nolint:errcheck
	req := httptest.NewRequest(http.MethodPost, "/api/apps/whatsapp/react", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// reactMux registers the apps router (which serves /api/apps/{name}/react
// by delegating to the gateway handler) plus the gateway routes.
func reactMux(mgr *gateway.Manager) *http.ServeMux {
	h := NewGatewayHandler(mgr, nil)
	mux := http.NewServeMux()
	h.Register(mux)
	NewAppsHandler(h, mgr, nil, nil).Register(mux)
	return mux
}

// TestGatewayReact_OK verifies that a well-formed react request calls the adapter.
func TestGatewayReact_OK(t *testing.T) {
	stub := &reactionStub{
		stubAdapter: stubAdapter{name: "whatsapp"},
	}

	mgr := gateway.NewManager()
	mgr.Register(stub)
	// Wire the channel route directly so we don't need a live connection.
	mgr.HandleNotification("whatsapp", gateway.Notification{
		Channel:   "family",
		ChannelID: "1234@g.us",
		Platform:  "whatsapp",
		Sender:    "alice",
		Content:   "hi",
	})

	mux := reactMux(mgr)

	req := buildReactRequest(t, map[string]any{
		"channel":    "whatsapp:family",
		"message_id": "msg-abc-123",
		"sender_jid": "9876543210@s.whatsapp.net",
		"emoji":      "👍",
	})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /react status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	calls := stub.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 SendReaction call, got %d", len(calls))
	}
	got := calls[0]
	if got.ChannelID != "1234@g.us" {
		t.Errorf("ChannelID = %q, want 1234@g.us", got.ChannelID)
	}
	if got.SenderJID != "9876543210@s.whatsapp.net" {
		t.Errorf("SenderJID = %q, want 9876543210@s.whatsapp.net", got.SenderJID)
	}
	if got.MessageID != "msg-abc-123" {
		t.Errorf("MessageID = %q, want msg-abc-123", got.MessageID)
	}
	if got.Emoji != "👍" {
		t.Errorf("Emoji = %q, want 👍", got.Emoji)
	}
}

// reactMissingFieldCase is a named type for TestGatewayReact_MissingFields cases.
type reactMissingFieldCase struct {
	body map[string]any
	name string
}

// TestGatewayReact_MissingFields verifies 400 on missing required fields.
// Note: empty emoji is intentionally NOT a validation error — empty string
// is the documented way to remove a reaction (whatsmeow BuildReaction(..., "")).
func TestGatewayReact_MissingFields(t *testing.T) {
	mgr := gateway.NewManager()
	mux := reactMux(mgr)

	tests := []reactMissingFieldCase{
		{map[string]any{"message_id": "mid", "emoji": "👍"}, "missing channel"},
		{map[string]any{"channel": "whatsapp:family", "emoji": "👍"}, "missing message_id"},
		{map[string]any{"channel": "whatsapp:family", "message_id": "mid", "emoji": "👍"}, "platform mismatch"},
	}
	// Override URL for the platform-mismatch case: POST to /api/apps/telegram/react
	// while body says channel="whatsapp:family".
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.name == "platform mismatch" {
				b, _ := json.Marshal(tt.body) //nolint:errcheck
				req = httptest.NewRequest(http.MethodPost, "/api/apps/telegram/react", bytes.NewReader(b))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = buildReactRequest(t, tt.body)
			}
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400; body: %s", tt.name, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestGatewayReact_RemoveReaction verifies that an empty emoji (reaction removal) is
// accepted and forwarded to the adapter — it must NOT be rejected with 400.
func TestGatewayReact_RemoveReaction(t *testing.T) {
	stub := &reactionStub{
		stubAdapter: stubAdapter{name: "whatsapp"},
	}

	mgr := gateway.NewManager()
	mgr.Register(stub)
	mgr.HandleNotification("whatsapp", gateway.Notification{
		Channel:   "family",
		ChannelID: "1234@g.us",
		Platform:  "whatsapp",
		Sender:    "alice",
		Content:   "hi",
	})

	mux := reactMux(mgr)

	req := buildReactRequest(t, map[string]any{
		"channel":    "whatsapp:family",
		"message_id": "msg-abc-123",
		"sender_jid": "9876543210@s.whatsapp.net",
		"emoji":      "", // empty = remove reaction
	})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("remove reaction: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	calls := stub.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 SendReaction call, got %d", len(calls))
	}
	if calls[0].Emoji != "" {
		t.Errorf("emoji = %q, want empty string for removal", calls[0].Emoji)
	}
}

// TestGatewayReact_UnknownChannel verifies 404 when channel is not a gateway channel.
func TestGatewayReact_UnknownChannel(t *testing.T) {
	mgr := gateway.NewManager()
	mux := reactMux(mgr)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, buildReactRequest(t, map[string]any{
		"channel":    "whatsapp:nonexistent",
		"message_id": "mid",
		"emoji":      "👍",
	}))
	if rr.Code != http.StatusNotFound {
		t.Errorf("unknown channel: status = %d, want 404", rr.Code)
	}
}

// TestGatewayReact_AdapterError verifies 500 when adapter returns an error.
func TestGatewayReact_AdapterError(t *testing.T) {
	stub := &reactionStub{
		stubAdapter: stubAdapter{name: "whatsapp"},
		err:         fmt.Errorf("whatsapp: not connected"),
	}

	mgr := gateway.NewManager()
	mgr.Register(stub)
	mgr.HandleNotification("whatsapp", gateway.Notification{
		Channel:   "family",
		ChannelID: "1234@g.us",
		Platform:  "whatsapp",
		Sender:    "alice",
		Content:   "hi",
	})

	mux := reactMux(mgr)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, buildReactRequest(t, map[string]any{
		"channel":    "whatsapp:family",
		"message_id": "mid",
		"emoji":      "👍",
	}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("adapter error: status = %d, want 500", rr.Code)
	}
}

// TestGatewayReact_MethodNotAllowed verifies non-POST methods are rejected.
func TestGatewayReact_MethodNotAllowed(t *testing.T) {
	mgr := gateway.NewManager()
	mux := reactMux(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/whatsapp/react", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /react: status = %d, want 405", rr.Code)
	}
}
