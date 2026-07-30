package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// stubAdapter implements gateway.NotificationAdapter for testing.
type stubAdapter struct {
	name     string
	channels []gateway.ChannelInfo
	handler  http.Handler
	status   gateway.AdapterStatus
}

func (s *stubAdapter) Name() string                                                { return s.name }
func (s *stubAdapter) Type() gateway.AdapterType                                   { return gateway.AdapterSocket }
func (s *stubAdapter) Start(_ context.Context, _ func(gateway.Notification)) error { return nil }
func (s *stubAdapter) Stop() error                                                 { return nil }
func (s *stubAdapter) HTTPHandler() http.Handler                                   { return s.handler }
func (s *stubAdapter) Channels() []gateway.ChannelInfo                             { return s.channels }
func (s *stubAdapter) Status() gateway.AdapterStatus                               { return s.status }

func TestGatewayLegacyChannelHistoryLimitCapping(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantLimit int
	}{
		{"default limit", "", 50},
		{"custom limit within range", "limit=30", 30},
		{"limit capped at 200", "limit=99999", 200},
		{"limit=1 stays at 1", "limit=1", 1},
		{"limit=200 stays at 200", "limit=200", 200},
		{"limit=201 capped at 200", "limit=201", 200},
		{"negative limit uses default", "limit=-5", 50},
		{"non-numeric limit uses default", "limit=abc", 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We verify indirectly: the handler calls clampInt(limit, 1, 200).
			// Since notifySvc is nil, the handler returns empty array early,
			// but we can verify the limit parsing logic via clampInt directly
			// and confirm the handler doesn't crash with extreme values.
			h := &GatewayHandler{}

			url := "/api/channels/test-channel/history"
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			rr := httptest.NewRecorder()

			h.channelHistory(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("got status %d, want 200", rr.Code)
			}

			// Also verify the clampInt logic directly for the limit value
			limit := 50
			if s := req.URL.Query().Get("limit"); s != "" {
				if n, err := parseInt(s); err == nil && n > 0 {
					limit = n
				}
			}
			clamped := clampInt(limit, 1, 200)
			if clamped != tt.wantLimit {
				t.Errorf("clamped limit = %d, want %d", clamped, tt.wantLimit)
			}
		})
	}
}

// parseInt is a test helper matching the handler's strconv.Atoi usage.
func parseInt(s string) (int, error) {
	var n int
	_, err := parseIntFmt(s, &n)
	return n, err
}

func parseIntFmt(s string, n *int) (int, error) {
	// Simple wrapper to avoid importing strconv in this test helper.
	// We use json.Unmarshal as a quick int parser.
	return 0, json.Unmarshal([]byte(s), n)
}

func TestGatewayAPIProxyRequestCloning(t *testing.T) {
	// Verify that gatewayAPIProxy clones the request so the original URL is not mutated.
	originalPath := "/api/gateways/test/api/v1/messages"

	var proxiedPath string
	proxyHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		proxiedPath = r.URL.Path
	})

	adapter := &stubAdapter{
		name:    "test",
		handler: proxyHandler,
	}
	gw := gateway.NewManager()
	gw.Register(adapter)

	h := &GatewayHandler{gw: gw}

	req := httptest.NewRequest(http.MethodGet, originalPath, nil)
	savedURL := req.URL.Path // capture before call
	rr := httptest.NewRecorder()

	h.gatewayAPIProxy(rr, req, "test", "/v1/messages")

	// Original request URL must not be mutated
	if req.URL.Path != savedURL {
		t.Errorf("original request URL was mutated: got %q, want %q", req.URL.Path, savedURL)
	}

	// Proxied request should have the subpath
	if proxiedPath != "/v1/messages" {
		t.Errorf("proxied path = %q, want %q", proxiedPath, "/v1/messages")
	}
}

func TestGatewayListPopulatesChannels(t *testing.T) {
	// Create a gateway manager with a stub adapter that has discovered channels.
	gw := gateway.NewManager()

	adapter := &stubAdapter{
		name: "slack",
		channels: []gateway.ChannelInfo{
			{ID: "C001", Name: "engineering", Platform: "slack"},
			{ID: "C002", Name: "general", Platform: "slack"},
		},
		status: gateway.AdapterStatus{Connected: true, BotName: "bc-bot"},
	}
	gw.Register(adapter)

	// Trigger channel discovery by calling Start briefly.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so Start returns
	_ = gw.Start(ctx)

	// Verify the gateway manager discovered the channels
	discovered := gw.DiscoveredSources()
	if len(discovered) == 0 {
		t.Fatal("expected discovered channels from adapter, got none")
	}

	// Verify channels are prefixed with adapter name
	for _, ch := range discovered {
		if !strings.HasPrefix(ch, "slack:") {
			t.Errorf("discovered channel %q should have slack: prefix", ch)
		}
	}

	// Now test the list handler. Without workspace config, dynamic adapters appear
	// but the enrichment loop for channels only runs for config-based platforms.
	// Test that the dynamic adapter entry at least appears with correct metadata.
	h := &GatewayHandler{gw: gw}

	req := httptest.NewRequest(http.MethodGet, "/api/gateways", nil)
	rr := httptest.NewRecorder()
	h.list(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rr.Code)
	}

	var platforms []struct { //nolint:govet // test-only struct, field order matches JSON
		Platform string   `json:"platform"`
		Channels []string `json:"channels"`
		BotName  string   `json:"bot_name"`
		Enabled  bool     `json:"enabled"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&platforms); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	found := false
	for _, p := range platforms {
		if p.Platform == "slack" {
			found = true
			if p.BotName != "bc-bot" {
				t.Errorf("bot_name = %q, want %q", p.BotName, "bc-bot")
			}
			if !p.Enabled {
				t.Error("dynamically registered adapter should be enabled=true")
			}
			// Channels array must never be null
			if p.Channels == nil {
				t.Error("channels should not be nil (should be [] in JSON)")
			}
		}
	}
	if !found {
		t.Error("slack platform not found in gateway list")
	}

	// Verify via the per-platform channels endpoint that channels are discoverable
	req2 := httptest.NewRequest(http.MethodGet, "/api/gateways/slack/channels", nil)
	rr2 := httptest.NewRecorder()
	h.gatewayChannels(rr2, req2, "slack", "")

	if rr2.Code != http.StatusOK {
		t.Fatalf("channels endpoint: got status %d, want 200", rr2.Code)
	}

	var channels []struct {
		ChannelKey string `json:"channel_key"`
		Name       string `json:"name"`
		Platform   string `json:"platform"`
	}
	if err := json.NewDecoder(rr2.Body).Decode(&channels); err != nil {
		t.Fatalf("decode channels: %v", err)
	}

	if len(channels) < 2 {
		t.Errorf("expected at least 2 channels, got %d", len(channels))
	}
	for _, ch := range channels {
		if ch.Platform != "slack" {
			t.Errorf("channel platform = %q, want %q", ch.Platform, "slack")
		}
		if !strings.HasPrefix(ch.ChannelKey, "slack:") {
			t.Errorf("channel_key %q should have slack: prefix", ch.ChannelKey)
		}
	}
}

func TestGatewayListNoNullChannels(t *testing.T) {
	// Verify channels is never null in JSON output (always []).
	gw := gateway.NewManager()
	adapter := &stubAdapter{
		name:     "discord",
		channels: nil, // no channels discovered
		status:   gateway.AdapterStatus{Connected: false},
	}
	gw.Register(adapter)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = gw.Start(ctx)

	h := &GatewayHandler{gw: gw}

	req := httptest.NewRequest(http.MethodGet, "/api/gateways", nil)
	rr := httptest.NewRecorder()
	h.list(rr, req)

	body := rr.Body.String()
	// Check that channels is [] not null
	if strings.Contains(body, `"channels":null`) {
		t.Error("channels should never be null in JSON response, should be []")
	}
}

// TestNotify503IncludesDegradedReason verifies that a nil notify service
// produces a 503 whose message carries the construction-time failure
// reason from Services.Degraded instead of a bare "not available"
// (issue #3240 — the 2026-07-03 Slack-delivery outage was undiagnosable
// from the generic message).
func TestNotify503IncludesDegradedReason(t *testing.T) {
	SetDegraded(map[string]string{
		"notify": "notify store unavailable: notify store requires shared database",
	})
	t.Cleanup(func() { SetDegraded(nil) })

	h := &GatewayHandler{} // no notify service wired

	req := httptest.NewRequest(http.MethodGet, "/api/notify/subscriptions", nil)
	rr := httptest.NewRecorder()
	h.notifySubscriptions(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "notify service not available") {
		t.Errorf("503 body lost the generic message: %s", body)
	}
	if !strings.Contains(body, "shared database") {
		t.Errorf("503 body must include the degradation reason: %s", body)
	}
}

// TestNotify503FallsBackWithoutReason verifies the generic message is kept
// when no degradation reason was recorded.
func TestNotify503FallsBackWithoutReason(t *testing.T) {
	SetDegraded(nil)

	h := &GatewayHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/notify/subscriptions", nil)
	rr := httptest.NewRecorder()
	h.notifySubscriptions(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "notify service not available") {
		t.Errorf("unexpected 503 body: %s", rr.Body.String())
	}
}

// --- Vault write tests ---

// openTestVault creates a temporary secrets vault for testing and returns it
// along with a cleanup function.
func openTestVault(t *testing.T) *secret.Store {
	t.Helper()
	vaultPath := filepath.Join(t.TempDir(), "secrets.vault")
	v, err := secret.OpenVaultFile(vaultPath, "test-passphrase")
	if err != nil {
		t.Fatalf("open test vault: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })
	return v
}

// setupTestWorkspace creates a minimal workspace directory suitable for
// gateway config updates. Uses a sandboxed MYCEL_HOME to avoid polluting the
// caller's real registry.
func setupTestWorkspace(t *testing.T) *workspace.Workspace {
	t.Helper()
	// Sandbox global state so workspace.Init doesn't write to the real registry.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", filepath.Join(home, ".bc"))

	dir := t.TempDir()
	// workspace.Init requires a git repository.
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("create .git: %v", err)
	}
	wks, err := workspace.Init(dir)
	if err != nil {
		t.Fatalf("workspace.Init: %v", err)
	}
	return wks
}

// ─── channelSend ─────────────────────────────────────────────────────────────

// sendStub extends stubAdapter with outbound Send support.
//
//nolint:govet // fieldalignment: test-only struct
type sendStub struct {
	stubAdapter
	calls []sendCall
	err   error
}

type sendCall struct {
	ChannelID string
	Sender    string
	Content   string
}

func (s *sendStub) Send(_ context.Context, channelID, sender, content string) error {
	s.calls = append(s.calls, sendCall{channelID, sender, content})
	return s.err
}

// TestChannelSend_OK verifies that POST /api/channels/send delivers via gateway.
func TestChannelSend_OK(t *testing.T) {
	stub := &sendStub{stubAdapter: stubAdapter{name: "slack"}}

	mgr := gateway.NewManager()
	mgr.Register(stub)
	// Seed the channel route via an inbound notification so Send can resolve it.
	mgr.HandleNotification("slack", gateway.Notification{
		Channel:   "general",
		ChannelID: "C1234",
		Platform:  "slack",
		Sender:    "bot",
		Content:   "hello",
	})

	h := NewGatewayHandler(mgr, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	body, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"channel": "slack:general",
		"message": "hello world",
		"sender":  "alice",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/channels/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]bool
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp["sent"] {
		t.Error("expected sent=true")
	}
	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 Send call, got %d", len(stub.calls))
	}
	if stub.calls[0].ChannelID != "C1234" {
		t.Errorf("ChannelID = %q, want C1234", stub.calls[0].ChannelID)
	}
	if stub.calls[0].Sender != "alice" {
		t.Errorf("Sender = %q, want alice", stub.calls[0].Sender)
	}
	if stub.calls[0].Content != "hello world" {
		t.Errorf("Content = %q, want hello world", stub.calls[0].Content)
	}
}

// TestChannelSend_NoGateway verifies 503 when the gateway is not wired.
func TestChannelSend_NoGateway(t *testing.T) {
	h := &GatewayHandler{}
	body, _ := json.Marshal(map[string]string{"channel": "slack:general", "message": "hi"}) //nolint:errcheck
	req := httptest.NewRequest(http.MethodPost, "/api/channels/send", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.channelSend(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

// TestChannelSend_MissingFields verifies 400 for missing channel or message.
func TestChannelSend_MissingFields(t *testing.T) {
	mgr := gateway.NewManager()
	h := NewGatewayHandler(mgr, nil)

	cases := []struct {
		name string
		body string
	}{
		{"missing channel", `{"message":"hi"}`},
		{"missing message", `{"channel":"slack:general"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/channels/send", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()
			h.channelSend(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rr.Code)
			}
		})
	}
}
