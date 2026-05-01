package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/workspace"
	"github.com/rpuneet/mycel/server/mcp"
)

// ─── #2967: MCP sender spoof regression ──────────────────────────────────────

// fakeAdapter implements gateway.NotificationAdapter + the unexported
// messageSender contract by exposing a Send method that captures the sender
// passed by the caller. We register one channel ("fake:room") so the gateway
// Manager has a route to dispatch to.
type fakeAdapter struct {
	lastSender  string
	lastContent string
	lastChannel string
}

func (a *fakeAdapter) Name() string                  { return "fake" }
func (a *fakeAdapter) Type() gateway.AdapterType     { return gateway.AdapterPoll }
func (a *fakeAdapter) Stop() error                   { return nil }
func (a *fakeAdapter) HTTPHandler() http.Handler     { return nil }
func (a *fakeAdapter) Status() gateway.AdapterStatus { return gateway.AdapterStatus{Connected: true} }
func (a *fakeAdapter) Channels() []gateway.ChannelInfo {
	return []gateway.ChannelInfo{{ID: "room", Name: "room", Platform: "fake"}}
}
func (a *fakeAdapter) Start(ctx context.Context, _ func(gateway.Notification)) error {
	<-ctx.Done()
	return nil
}

// Send satisfies the unexported messageSender interface checked at runtime
// inside gateway.Manager.Send.
func (a *fakeAdapter) Send(_ context.Context, channelID, sender, content string) error {
	a.lastChannel = channelID
	a.lastSender = sender
	a.lastContent = content
	return nil
}

// newServerWithFakeGateway returns an mcp.Server backed by a real
// gateway.Manager whose only adapter is a fakeAdapter with a discovered
// channel "fake:room". Because Manager.Start() is what triggers
// discoverChannels but blocks until ctx is canceled, we instead drive
// discovery by calling Start in a cancellable goroutine and waiting one
// scheduler turn — but the simpler path is to call HandleNotification on
// nothing and rely on the existing discovery happening synchronously
// inside Start before it begins to wait. The cleanest is to start the
// manager with an immediately-canceled context: Start runs discovery
// before <-ctx.Done().
func newServerWithFakeGateway(t *testing.T) (*mcp.Server, *fakeAdapter) {
	t.Helper()

	wsDir := makeWorkspace(t)
	ws, err := workspace.Load(wsDir)
	if err != nil {
		t.Fatalf("workspace.Load: %v", err)
	}

	mgr := gateway.NewManager()
	fa := &fakeAdapter{}
	mgr.Register(fa)

	// Drive channel discovery: Manager.Start runs restorePersistedChannels
	// + discoverChannels synchronously, then enters a select on ctx.Done().
	// With an already-canceled ctx, Start returns immediately AFTER
	// discovery runs.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = mgr.Start(ctx) //nolint:errcheck // discovery side-effect only

	srv, err := mcp.New(mcp.Config{Workspace: ws, Gateway: mgr, Version: "test"})
	if err != nil {
		t.Fatalf("mcp.New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv, fa
}

// rpcWithCtx mirrors the rpc() helper but lets the caller supply a context
// so we can attach an authenticated agent identity.
func rpcWithCtx(t *testing.T, ctx context.Context, srv *mcp.Server, method string, params any, dst any) {
	t.Helper()
	id := json.RawMessage(`1`)
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rawParams = b
	}
	req := mcp.Request{JSONRPC: "2.0", ID: &id, Method: method, Params: rawParams}
	resp := srv.Handle(ctx, req)
	if resp.Error != nil {
		t.Fatalf("rpc error: %s (code %d)", resp.Error.Message, resp.Error.Code)
	}
	if dst == nil {
		return
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		t.Fatalf("unmarshal into %T: %v", dst, err)
	}
}

// TestSendMessage_SenderSpoofIsOverridden ensures a client passing an
// arbitrary "sender" cannot impersonate another agent: the authenticated
// context agent always wins. Regression for issue #2967.
func TestSendMessage_SenderSpoofIsOverridden(t *testing.T) {
	srv, fa := newServerWithFakeGateway(t)

	ctx := mcp.ContextWithAgent(context.Background(), "alice")

	var result struct {
		Content []mcp.ToolContent `json:"content"`
		IsError bool              `json:"isError"`
	}
	rpcWithCtx(t, ctx, srv, "tools/call", map[string]any{
		"name": "send_message",
		"arguments": map[string]any{
			"channel": "fake:room",
			"message": "hello",
			"sender":  "bob", // spoof attempt — must be ignored
		},
	}, &result)

	if result.IsError {
		t.Fatalf("unexpected isError=true: %+v", result.Content)
	}
	if fa.lastSender != "alice" {
		t.Fatalf("sender = %q, want alice (client-supplied %q must be ignored)",
			fa.lastSender, "bob")
	}
	if fa.lastContent != "hello" {
		t.Errorf("content = %q, want hello", fa.lastContent)
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "as alice") {
		t.Errorf("response should attribute send to alice, got %+v", result.Content)
	}
}

// TestSendMessage_NoCtxAgent_FallsBackToClient verifies the legacy fallback
// path: when no agent is bound to the context (e.g., stdio transport), the
// client value is honored for backward compatibility.
func TestSendMessage_NoCtxAgent_FallsBackToClient(t *testing.T) {
	srv, fa := newServerWithFakeGateway(t)

	var result struct {
		Content []mcp.ToolContent `json:"content"`
		IsError bool              `json:"isError"`
	}
	rpcWithCtx(t, context.Background(), srv, "tools/call", map[string]any{
		"name": "send_message",
		"arguments": map[string]any{
			"channel": "fake:room",
			"message": "hi",
			"sender":  "carol",
		},
	}, &result)

	if result.IsError {
		t.Fatalf("unexpected isError=true: %+v", result.Content)
	}
	if fa.lastSender != "carol" {
		t.Fatalf("sender = %q, want carol (no ctx agent → trust client value)",
			fa.lastSender)
	}
}

// TestSendMessage_CtxAgent_BlankClientUsesCtx covers the ordinary case: no
// client sender supplied, ctx agent is used.
func TestSendMessage_CtxAgent_BlankClientUsesCtx(t *testing.T) {
	srv, fa := newServerWithFakeGateway(t)

	ctx := mcp.ContextWithAgent(context.Background(), "alice")

	var result struct {
		Content []mcp.ToolContent `json:"content"`
		IsError bool              `json:"isError"`
	}
	rpcWithCtx(t, ctx, srv, "tools/call", map[string]any{
		"name": "send_message",
		"arguments": map[string]any{
			"channel": "fake:room",
			"message": "hi",
		},
	}, &result)

	if result.IsError {
		t.Fatalf("unexpected isError=true: %+v", result.Content)
	}
	if fa.lastSender != "alice" {
		t.Fatalf("sender = %q, want alice", fa.lastSender)
	}
}
