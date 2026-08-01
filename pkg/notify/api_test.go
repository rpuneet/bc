package notify_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/server/handlers"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// mockAgentSender records Send calls for assertion.
type mockAgentSender struct {
	errFn func(name string) error
	calls []agentSendCall
	mu    sync.Mutex
}

type agentSendCall struct {
	Name    string
	Message string
}

func (m *mockAgentSender) Send(_ context.Context, name, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, agentSendCall{Name: name, Message: message})
	if m.errFn != nil {
		return m.errFn(name)
	}
	return nil
}

func (m *mockAgentSender) SendAll(_ context.Context, message string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, agentSendCall{Name: "*", Message: message})
	return 1, nil
}

func (m *mockAgentSender) getCalls() []agentSendCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]agentSendCall, len(m.calls))
	copy(out, m.calls)
	return out
}

// mockBroadcaster discards publish events.
type mockBroadcaster struct{}

func (m *mockBroadcaster) Publish(_ string, _ map[string]any) {}

// ---------------------------------------------------------------------------
// Test setup helpers
// ---------------------------------------------------------------------------

// setupStore opens an in-memory SQLite database, registers it as shared,
// initializes the notify store and returns store + cleanup.
func setupStore(t *testing.T) *notify.Store {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	store, err := notify.OpenStore(d, "sqlite")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	return store
}

// setupService creates a Service over the given store with mock collaborators.
func setupService(store *notify.Store) (*notify.Service, *mockAgentSender) {
	sender := &mockAgentSender{}
	svc := notify.NewService(store, sender, &mockBroadcaster{})
	return svc, sender
}

// setupHandler wires a GatewayHandler with a notify service and registers
// all routes on a fresh ServeMux, plus the apps router that serves the
// /api/apps/{name}/channels/... subscription routes. Returns the
// httptest server.
func setupHandler(t *testing.T, svc *notify.Service) *httptest.Server {
	t.Helper()
	h := handlers.NewGatewayHandler(nil, nil)
	h.SetNotifyService(svc)

	mux := http.NewServeMux()
	h.Register(mux)
	handlers.NewAppsHandler(h, nil, nil, nil).Register(mux)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func doJSON(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	var rb *bytes.Reader
	if body != nil {
		rb = bytes.NewReader(buf.Bytes())
	} else {
		rb = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, url, rb)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req) //nolint:bodyclose // body closed via t.Cleanup below
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func decodeJSONSlice[T any](t *testing.T, resp *http.Response) []T {
	t.Helper()
	var result []T
	decodeJSON(t, resp, &result)
	return result
}

func decodeJSONMap(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var result map[string]any
	decodeJSON(t, resp, &result)
	return result
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("want HTTP %d, got %d", want, resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestGetChannels — GET /api/channels returns channels from subscriptions.
func TestGetChannels(t *testing.T) {
	tests := []struct {
		setup      func(ctx context.Context, store *notify.Store)
		name       string
		wantNames  []string
		wantCount  int
		wantStatus int
	}{
		{
			name:       "empty store returns empty array",
			setup:      func(_ context.Context, _ *notify.Store) {},
			wantCount:  0,
			wantStatus: http.StatusOK,
		},
		{
			name: "subscriptions appear as channels",
			setup: func(ctx context.Context, store *notify.Store) {
				_ = store.Subscribe(ctx, "slack:eng", "eng-01", false)
				_ = store.Subscribe(ctx, "telegram:ops", "ops-agent", false)
			},
			wantCount:  2,
			wantNames:  []string{"slack:eng", "telegram:ops"},
			wantStatus: http.StatusOK,
		},
		{
			name: "duplicate channel from multiple agents appears once",
			setup: func(ctx context.Context, store *notify.Store) {
				_ = store.Subscribe(ctx, "slack:eng", "eng-01", false)
				_ = store.Subscribe(ctx, "slack:eng", "eng-02", false)
			},
			wantCount:  1,
			wantNames:  []string{"slack:eng"},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupStore(t)
			svc, _ := setupService(store)
			ts := setupHandler(t, svc)

			tt.setup(context.Background(), store)

			resp := doJSON(t, http.MethodGet, ts.URL+"/api/channels", nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
			assertStatus(t, resp, tt.wantStatus)

			type legacyChannel struct {
				Name string `json:"name"`
			}
			var channels []legacyChannel
			decodeJSON(t, resp, &channels)

			if len(channels) != tt.wantCount {
				t.Fatalf("expected %d channels, got %d: %v", tt.wantCount, len(channels), channels)
			}

			if len(tt.wantNames) > 0 {
				nameSet := make(map[string]bool, len(channels))
				for _, ch := range channels {
					nameSet[ch.Name] = true
				}
				for _, want := range tt.wantNames {
					if !nameSet[want] {
						t.Errorf("expected channel %q in response, got %v", want, channels)
					}
				}
			}
		})
	}
}

// TestGetChannels_MethodNotAllowed verifies non-GET methods are rejected.
func TestGetChannels_MethodNotAllowed(t *testing.T) {
	store := setupStore(t)
	svc, _ := setupService(store)
	ts := setupHandler(t, svc)

	for _, method := range []string{http.MethodPost, http.MethodDelete, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			resp := doJSON(t, method, ts.URL+"/api/channels", nil)
			defer func() { _ = resp.Body.Close() }()
			assertStatus(t, resp, http.StatusMethodNotAllowed)
		})
	}
}

// TestPostSubscription — POST /api/notify/subscriptions subscribes an agent.
func TestPostSubscription(t *testing.T) {
	tests := []struct {
		body        any
		name        string
		wantStatus2 string
		wantStatus  int
	}{
		{
			name:        "valid subscription",
			body:        map[string]any{"channel": "slack:eng", "agent": "eng-01", "mention_only": false},
			wantStatus:  http.StatusCreated,
			wantStatus2: "subscribed",
		},
		{
			name:        "mention_only subscription",
			body:        map[string]any{"channel": "discord:alerts", "agent": "root", "mention_only": true},
			wantStatus:  http.StatusCreated,
			wantStatus2: "subscribed",
		},
		{
			name:       "missing channel",
			body:       map[string]any{"agent": "eng-01"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing agent",
			body:       map[string]any{"channel": "slack:eng"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty body",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON body",
			body:       nil, // we'll send raw bad JSON
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupStore(t)
			svc, _ := setupService(store)
			ts := setupHandler(t, svc)

			var resp *http.Response
			if tt.name == "invalid JSON body" {
				req, _ := http.NewRequestWithContext(context.Background(),
					http.MethodPost, ts.URL+"/api/notify/subscriptions",
					strings.NewReader("{bad json"))
				req.Header.Set("Content-Type", "application/json")
				var err error
				resp, err = http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				resp = doJSON(t, http.MethodPost, ts.URL+"/api/notify/subscriptions", tt.body)
			}

			assertStatus(t, resp, tt.wantStatus)

			if tt.wantStatus2 != "" {
				m := decodeJSONMap(t, resp)
				if m["status"] != tt.wantStatus2 {
					t.Errorf("expected status=%q, got %v", tt.wantStatus2, m["status"])
				}
			} else {
				_ = resp.Body.Close()
			}
		})
	}
}

// TestPostSubscription_Idempotent verifies re-subscribing updates mention_only.
func TestPostSubscription_Idempotent(t *testing.T) {
	store := setupStore(t)
	svc, _ := setupService(store)
	ts := setupHandler(t, svc)

	// First subscribe
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/notify/subscriptions",
		map[string]any{"channel": "slack:eng", "agent": "eng-01", "mention_only": false})
	assertStatus(t, resp, http.StatusCreated)
	_ = resp.Body.Close()

	// Re-subscribe with mention_only=true
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/notify/subscriptions",
		map[string]any{"channel": "slack:eng", "agent": "eng-01", "mention_only": true})
	assertStatus(t, resp, http.StatusCreated)
	_ = resp.Body.Close()

	// Verify one subscription with mention_only=true
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/notify/subscriptions", nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
	assertStatus(t, resp, http.StatusOK)
	var subs []notify.Subscription
	decodeJSON(t, resp, &subs)

	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
	if !subs[0].MentionOnly {
		t.Error("expected mention_only=true after update")
	}
}

// TestGetSubscriptions — GET /api/notify/subscriptions lists all subscriptions.
func TestGetSubscriptions(t *testing.T) {
	tests := []struct {
		setup      func(ctx context.Context, store *notify.Store)
		name       string
		wantCount  int
		wantStatus int
	}{
		{
			name:       "empty store",
			setup:      func(_ context.Context, _ *notify.Store) {},
			wantCount:  0,
			wantStatus: http.StatusOK,
		},
		{
			name: "multiple subscriptions across channels",
			setup: func(ctx context.Context, store *notify.Store) {
				_ = store.Subscribe(ctx, "slack:eng", "eng-01", false)
				_ = store.Subscribe(ctx, "slack:eng", "eng-02", true)
				_ = store.Subscribe(ctx, "telegram:ops", "ops-agent", false)
			},
			wantCount:  3,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupStore(t)
			svc, _ := setupService(store)
			ts := setupHandler(t, svc)

			tt.setup(context.Background(), store)

			resp := doJSON(t, http.MethodGet, ts.URL+"/api/notify/subscriptions", nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
			assertStatus(t, resp, tt.wantStatus)

			var subs []notify.Subscription
			decodeJSON(t, resp, &subs)

			if len(subs) != tt.wantCount {
				t.Fatalf("expected %d subscriptions, got %d", tt.wantCount, len(subs))
			}
		})
	}
}

// TestGetSubscriptionByChannel — GET /api/notify/subscriptions/{channel}.
func TestGetSubscriptionByChannel(t *testing.T) {
	tests := []struct {
		setup      func(ctx context.Context, store *notify.Store)
		name       string
		channel    string
		wantCount  int
		wantStatus int
	}{
		{
			name:       "empty channel returns empty array",
			channel:    "slack:empty",
			setup:      func(_ context.Context, _ *notify.Store) {},
			wantCount:  0,
			wantStatus: http.StatusOK,
		},
		{
			name:    "channel with subscribers",
			channel: "slack:eng",
			setup: func(ctx context.Context, store *notify.Store) {
				_ = store.Subscribe(ctx, "slack:eng", "eng-01", false)
				_ = store.Subscribe(ctx, "slack:eng", "eng-02", true)
				_ = store.Subscribe(ctx, "discord:other", "other-agent", false)
			},
			wantCount:  2, // only slack:eng subscribers
			wantStatus: http.StatusOK,
		},
		{
			name:    "channel with colon in name (platform:name format)",
			channel: "telegram:alerts",
			setup: func(ctx context.Context, store *notify.Store) {
				_ = store.Subscribe(ctx, "telegram:alerts", "root", false)
			},
			wantCount:  1,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupStore(t)
			svc, _ := setupService(store)
			ts := setupHandler(t, svc)

			tt.setup(context.Background(), store)

			resp := doJSON(t, http.MethodGet, //nolint:bodyclose // closed via t.Cleanup in doJSON
				ts.URL+"/api/notify/subscriptions/"+tt.channel, nil)
			assertStatus(t, resp, tt.wantStatus)

			var subs []notify.Subscription
			decodeJSON(t, resp, &subs)

			if len(subs) != tt.wantCount {
				t.Fatalf("want %d subs for %s, got %d", tt.wantCount, tt.channel, len(subs))
			}

			// Verify all returned subs are for the correct channel
			for _, s := range subs {
				if s.Channel != tt.channel {
					t.Errorf("unexpected channel %q in subscription (want %q)", s.Channel, tt.channel)
				}
			}
		})
	}
}

// TestDeleteSubscription — DELETE /api/notify/subscriptions/{channel}?agent=X.
func TestDeleteSubscription(t *testing.T) {
	tests := []struct {
		name        string
		channel     string
		agent       string
		queryParam  string
		wantStatus2 string
		wantStatus  int
	}{
		{
			name:        "valid unsubscribe",
			channel:     "slack:eng",
			agent:       "eng-01",
			queryParam:  "eng-01",
			wantStatus:  http.StatusOK,
			wantStatus2: "unsubscribed",
		},
		{
			name:       "missing agent query param returns 400",
			channel:    "slack:eng",
			agent:      "eng-01",
			queryParam: "", // no ?agent= param
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupStore(t)
			svc, _ := setupService(store)
			ts := setupHandler(t, svc)

			ctx := context.Background()
			_ = store.Subscribe(ctx, tt.channel, tt.agent, false)

			url := ts.URL + "/api/notify/subscriptions/" + tt.channel
			if tt.queryParam != "" {
				url += "?agent=" + tt.queryParam
			}

			resp := doJSON(t, http.MethodDelete, url, nil)
			assertStatus(t, resp, tt.wantStatus)

			if tt.wantStatus2 != "" {
				m := decodeJSONMap(t, resp)
				if m["status"] != tt.wantStatus2 {
					t.Errorf("expected status=%q, got %v", tt.wantStatus2, m["status"])
				}
			} else {
				_ = resp.Body.Close()
			}

			// Verify agent is actually removed when successful
			if tt.wantStatus == http.StatusOK {
				subs, err := store.Subscribers(ctx, tt.channel)
				if err != nil {
					t.Fatal(err)
				}
				for _, s := range subs {
					if s.Agent == tt.agent {
						t.Errorf("agent %q still subscribed after DELETE", tt.agent)
					}
				}
			}
		})
	}
}

// TestPatchSubscription — PATCH /api/notify/subscriptions/{channel} toggles mention_only.
func TestPatchSubscription(t *testing.T) {
	tests := []struct {
		body        map[string]any
		name        string
		channel     string
		agent       string
		wantStatus2 string
		wantStatus  int
		initMO      bool
		wantMO      bool
	}{
		{
			name:        "enable mention_only",
			channel:     "slack:eng",
			agent:       "eng-01",
			initMO:      false,
			body:        map[string]any{"agent": "eng-01", "mention_only": true},
			wantStatus:  http.StatusOK,
			wantStatus2: "updated",
			wantMO:      true,
		},
		{
			name:        "disable mention_only",
			channel:     "slack:eng",
			agent:       "eng-01",
			initMO:      true,
			body:        map[string]any{"agent": "eng-01", "mention_only": false},
			wantStatus:  http.StatusOK,
			wantStatus2: "updated",
			wantMO:      false,
		},
		{
			name:       "missing agent returns 400",
			channel:    "slack:eng",
			agent:      "eng-01",
			initMO:     false,
			body:       map[string]any{"mention_only": true}, // no agent field
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON returns 400",
			channel:    "slack:eng",
			agent:      "eng-01",
			initMO:     false,
			body:       nil, // signals bad JSON
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupStore(t)
			svc, _ := setupService(store)
			ts := setupHandler(t, svc)

			ctx := context.Background()
			_ = store.Subscribe(ctx, tt.channel, tt.agent, tt.initMO)

			url := ts.URL + "/api/notify/subscriptions/" + tt.channel

			var resp *http.Response
			if tt.body == nil {
				// Send bad JSON
				req, _ := http.NewRequestWithContext(context.Background(),
					http.MethodPatch, url, strings.NewReader("{invalid"))
				req.Header.Set("Content-Type", "application/json")
				var err error
				resp, err = http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				resp = doJSON(t, http.MethodPatch, url, tt.body)
			}

			assertStatus(t, resp, tt.wantStatus)

			if tt.wantStatus2 != "" {
				m := decodeJSONMap(t, resp)
				if m["status"] != tt.wantStatus2 {
					t.Errorf("expected status=%q, got %v", tt.wantStatus2, m["status"])
				}
			} else {
				_ = resp.Body.Close()
			}

			// Verify mention_only was updated
			if tt.wantStatus == http.StatusOK {
				subs, err := store.Subscribers(ctx, tt.channel)
				if err != nil {
					t.Fatal(err)
				}
				var found bool
				for _, s := range subs {
					if s.Agent == tt.agent {
						found = true
						if s.MentionOnly != tt.wantMO {
							t.Errorf("mention_only: got %v, want %v", s.MentionOnly, tt.wantMO)
						}
					}
				}
				if !found {
					t.Errorf("agent %q not found in subscribers", tt.agent)
				}
			}
		})
	}
}

// TestGetNotifyActivity — GET /api/notify/activity/{channel} returns delivery log.
func TestGetNotifyActivity(t *testing.T) {
	tests := []struct {
		setup      func(ctx context.Context, store *notify.Store)
		name       string
		channel    string
		wantCount  int
		wantStatus int
	}{
		{
			name:       "empty log returns empty array",
			channel:    "slack:eng",
			setup:      func(_ context.Context, _ *notify.Store) {},
			wantCount:  0,
			wantStatus: http.StatusOK,
		},
		{
			name:    "returns delivery entries for channel",
			channel: "slack:eng",
			setup: func(ctx context.Context, store *notify.Store) {
				for range 3 {
					_ = store.LogDelivery(ctx, notify.DeliveryEntry{
						Channel: "slack:eng",
						Agent:   "eng-01",
						Status:  notify.StatusDelivered,
						Preview: "test message",
					})
				}
				// Entry for another channel — should not appear
				_ = store.LogDelivery(ctx, notify.DeliveryEntry{
					Channel: "discord:other",
					Agent:   "other",
					Status:  notify.StatusDelivered,
					Preview: "other channel",
				})
			},
			wantCount:  3,
			wantStatus: http.StatusOK,
		},
		{
			name:    "limit query param is respected",
			channel: "slack:eng",
			setup: func(ctx context.Context, store *notify.Store) {
				for range 10 {
					_ = store.LogDelivery(ctx, notify.DeliveryEntry{
						Channel: "slack:eng",
						Agent:   "eng-01",
						Status:  notify.StatusDelivered,
						Preview: "msg",
					})
				}
			},
			wantCount:  5, // will use ?limit=5 in request
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupStore(t)
			svc, _ := setupService(store)
			ts := setupHandler(t, svc)

			tt.setup(context.Background(), store)

			url := ts.URL + "/api/notify/activity/" + tt.channel
			if tt.name == "limit query param is respected" {
				url += "?limit=5"
			}

			resp := doJSON(t, http.MethodGet, url, nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
			assertStatus(t, resp, tt.wantStatus)

			var entries []notify.DeliveryEntry
			decodeJSON(t, resp, &entries)

			if len(entries) != tt.wantCount {
				t.Fatalf("want %d entries, got %d", tt.wantCount, len(entries))
			}
		})
	}
}

// TestGetNotifyActivity_NoChannel verifies 400 when channel is omitted.
func TestGetNotifyActivity_NoChannel(t *testing.T) {
	store := setupStore(t)
	svc, _ := setupService(store)
	ts := setupHandler(t, svc)

	// /api/notify/activity/ with no channel name
	resp := doJSON(t, http.MethodGet, ts.URL+"/api/notify/activity/", nil)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestGatewayChannelAgentsGet — GET /api/apps/{name}/channels/{ch}/agents.
func TestGatewayChannelAgentsGet(t *testing.T) {
	tests := []struct {
		setup      func(ctx context.Context, store *notify.Store)
		name       string
		gw         string
		channel    string
		wantCount  int
		wantStatus int
	}{
		{
			name:       "no subscribers returns empty array",
			gw:         "slack",
			channel:    "eng",
			setup:      func(_ context.Context, _ *notify.Store) {},
			wantCount:  0,
			wantStatus: http.StatusOK,
		},
		{
			name:    "returns subscribers for gateway:channel",
			gw:      "slack",
			channel: "eng",
			setup: func(ctx context.Context, store *notify.Store) {
				_ = store.Subscribe(ctx, "slack:eng", "eng-01", false)
				_ = store.Subscribe(ctx, "slack:eng", "eng-02", true)
				// Different gateway — should not appear
				_ = store.Subscribe(ctx, "discord:eng", "discord-agent", false)
			},
			wantCount:  2,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupStore(t)
			svc, _ := setupService(store)
			ts := setupHandler(t, svc)

			tt.setup(context.Background(), store)

			url := ts.URL + "/api/apps/" + tt.gw + "/channels/" + tt.channel + "/agents"
			resp := doJSON(t, http.MethodGet, url, nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
			assertStatus(t, resp, tt.wantStatus)

			var subs []notify.Subscription
			decodeJSON(t, resp, &subs)

			if len(subs) != tt.wantCount {
				t.Fatalf("want %d agents, got %d: %v", tt.wantCount, len(subs), subs)
			}

			// Verify the subscriptions use the full channel key (gw:channel)
			channelKey := tt.gw + ":" + tt.channel
			for _, s := range subs {
				if s.Channel != channelKey {
					t.Errorf("unexpected channel %q (want %q)", s.Channel, channelKey)
				}
			}
		})
	}
}

// TestGatewayChannelAgentsPost — POST /api/apps/{name}/channels/{ch}/agents.
func TestGatewayChannelAgentsPost(t *testing.T) {
	tests := []struct {
		body        any
		name        string
		gw          string
		channel     string
		wantStatus2 string
		wantStatus  int
	}{
		{
			name:        "valid subscribe",
			gw:          "slack",
			channel:     "eng",
			body:        map[string]any{"agent": "eng-01", "mention_only": false},
			wantStatus:  http.StatusCreated,
			wantStatus2: "subscribed",
		},
		{
			name:        "mention_only subscribe",
			gw:          "telegram",
			channel:     "alerts",
			body:        map[string]any{"agent": "root", "mention_only": true},
			wantStatus:  http.StatusCreated,
			wantStatus2: "subscribed",
		},
		{
			name:       "missing agent returns 400",
			gw:         "slack",
			channel:    "eng",
			body:       map[string]any{"mention_only": false},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupStore(t)
			svc, _ := setupService(store)
			ts := setupHandler(t, svc)

			url := ts.URL + "/api/apps/" + tt.gw + "/channels/" + tt.channel + "/agents"
			resp := doJSON(t, http.MethodPost, url, tt.body)
			assertStatus(t, resp, tt.wantStatus)

			if tt.wantStatus2 != "" {
				m := decodeJSONMap(t, resp)
				if m["status"] != tt.wantStatus2 {
					t.Errorf("expected status=%q, got %v", tt.wantStatus2, m["status"])
				}
				// Verify subscription was stored with correct channel key
				ctx := context.Background()
				channelKey := tt.gw + ":" + tt.channel
				subs, err := store.Subscribers(ctx, channelKey)
				if err != nil {
					t.Fatal(err)
				}
				if len(subs) == 0 {
					t.Errorf("expected subscription to be stored for %q", channelKey)
				}
			} else {
				_ = resp.Body.Close()
			}
		})
	}
}

// TestGatewayChannelAgentsDelete — DELETE /api/apps/{name}/channels/{ch}/agents?agent=X.
func TestGatewayChannelAgentsDelete(t *testing.T) {
	tests := []struct {
		name        string
		gw          string
		channel     string
		subscribeAs string
		queryAgent  string
		wantStatus2 string
		wantStatus  int
	}{
		{
			name:        "valid unsubscribe via query param",
			gw:          "slack",
			channel:     "eng",
			subscribeAs: "eng-01",
			queryAgent:  "eng-01",
			wantStatus:  http.StatusOK,
			wantStatus2: "unsubscribed",
		},
		{
			name:        "missing agent returns 400",
			gw:          "slack",
			channel:     "eng",
			subscribeAs: "eng-01",
			queryAgent:  "", // no param
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupStore(t)
			svc, _ := setupService(store)
			ts := setupHandler(t, svc)

			ctx := context.Background()
			channelKey := tt.gw + ":" + tt.channel
			_ = store.Subscribe(ctx, channelKey, tt.subscribeAs, false)

			url := ts.URL + "/api/apps/" + tt.gw + "/channels/" + tt.channel + "/agents"
			if tt.queryAgent != "" {
				url += "?agent=" + tt.queryAgent
			}

			resp := doJSON(t, http.MethodDelete, url, nil)
			assertStatus(t, resp, tt.wantStatus)

			if tt.wantStatus2 != "" {
				m := decodeJSONMap(t, resp)
				if m["status"] != tt.wantStatus2 {
					t.Errorf("expected status=%q, got %v", tt.wantStatus2, m["status"])
				}
				// Verify actually removed
				subs, err := store.Subscribers(ctx, channelKey)
				if err != nil {
					t.Fatal(err)
				}
				for _, s := range subs {
					if s.Agent == tt.subscribeAs {
						t.Errorf("agent %q still subscribed after DELETE", tt.subscribeAs)
					}
				}
			} else {
				_ = resp.Body.Close()
			}
		})
	}
}

// TestGatewayChannelAgentsPatch — PATCH /api/apps/{name}/channels/{ch}/agents/{agent}.
func TestGatewayChannelAgentsPatch(t *testing.T) {
	tests := []struct {
		body        map[string]any
		name        string
		gw          string
		channel     string
		agent       string
		wantStatus2 string
		wantStatus  int
		wantMO      bool
	}{
		{
			name:        "enable mention_only",
			gw:          "slack",
			channel:     "eng",
			agent:       "eng-01",
			body:        map[string]any{"mention_only": true},
			wantStatus:  http.StatusOK,
			wantStatus2: "updated",
			wantMO:      true,
		},
		{
			name:        "disable mention_only",
			gw:          "slack",
			channel:     "eng",
			agent:       "eng-01",
			body:        map[string]any{"mention_only": false},
			wantStatus:  http.StatusOK,
			wantStatus2: "updated",
			wantMO:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupStore(t)
			svc, _ := setupService(store)
			ts := setupHandler(t, svc)

			ctx := context.Background()
			channelKey := tt.gw + ":" + tt.channel
			initMO := !tt.wantMO // start opposite
			_ = store.Subscribe(ctx, channelKey, tt.agent, initMO)

			url := ts.URL + "/api/apps/" + tt.gw + "/channels/" + tt.channel + "/agents/" + tt.agent
			resp := doJSON(t, http.MethodPatch, url, tt.body)
			assertStatus(t, resp, tt.wantStatus)

			if tt.wantStatus2 != "" {
				m := decodeJSONMap(t, resp)
				if m["status"] != tt.wantStatus2 {
					t.Errorf("expected status=%q, got %v", tt.wantStatus2, m["status"])
				}
			} else {
				_ = resp.Body.Close()
			}

			// Verify mention_only was updated
			subs, err := store.Subscribers(ctx, channelKey)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range subs {
				if s.Agent == tt.agent {
					if s.MentionOnly != tt.wantMO {
						t.Errorf("mention_only: got %v, want %v", s.MentionOnly, tt.wantMO)
					}
				}
			}
		})
	}
}

// TestMessageStorage — verify SaveMessage + GetMessages store and retrieve correctly.
func TestMessageStorage(t *testing.T) {
	tests := []struct {
		name         string
		queryChannel string
		wantFirst    string
		messages     []struct{ channel, sender, content string }
		queryLimit   int
		queryBefore  int64
		wantCount    int
	}{
		{
			name: "save and retrieve messages",
			messages: []struct{ channel, sender, content string }{
				{"slack:eng", "alice", "first message"},
				{"slack:eng", "bob", "second message"},
				{"slack:eng", "carol", "third message"},
			},
			queryChannel: "slack:eng",
			queryLimit:   10,
			wantCount:    3,
			wantFirst:    "third message", // newest first
		},
		{
			name: "messages scoped to channel",
			messages: []struct{ channel, sender, content string }{
				{"slack:eng", "alice", "eng message"},
				{"discord:ops", "bob", "ops message"},
			},
			queryChannel: "slack:eng",
			queryLimit:   10,
			wantCount:    1,
			wantFirst:    "eng message",
		},
		{
			name: "limit is respected",
			messages: []struct{ channel, sender, content string }{
				{"slack:eng", "a", "msg1"},
				{"slack:eng", "b", "msg2"},
				{"slack:eng", "c", "msg3"},
				{"slack:eng", "d", "msg4"},
				{"slack:eng", "e", "msg5"},
			},
			queryChannel: "slack:eng",
			queryLimit:   3,
			wantCount:    3,
			wantFirst:    "msg5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupStore(t)
			ctx := context.Background()

			for _, m := range tt.messages {
				if err := store.SaveMessage(ctx, m.channel, m.sender, "", m.content); err != nil {
					t.Fatalf("SaveMessage: %v", err)
				}
			}

			msgs, err := store.GetMessages(ctx, tt.queryChannel, tt.queryLimit, tt.queryBefore)
			if err != nil {
				t.Fatalf("GetMessages: %v", err)
			}

			if len(msgs) != tt.wantCount {
				t.Fatalf("want %d messages, got %d", tt.wantCount, len(msgs))
			}

			if tt.wantFirst != "" && len(msgs) > 0 {
				if msgs[0].Content != tt.wantFirst {
					t.Errorf("first message content: got %q, want %q", msgs[0].Content, tt.wantFirst)
				}
			}

			// Verify fields are populated
			for _, m := range msgs {
				if m.ID == 0 {
					t.Error("message ID should be non-zero")
				}
				if m.Channel == "" {
					t.Error("message channel should not be empty")
				}
				if m.Sender == "" {
					t.Error("message sender should not be empty")
				}
				if m.Content == "" {
					t.Error("message content should not be empty")
				}
			}
		})
	}
}

// TestMessageStorage_Before verifies the before-cursor pagination.
func TestMessageStorage_Before(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()

	// Save 5 messages
	for i := range 5 {
		_ = store.SaveMessage(ctx, "slack:eng", "sender", "", "msg")
		_ = i // suppress unused warning
	}

	// Get all to find the third message's ID
	all, err := store.GetMessages(ctx, "slack:eng", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(all))
	}

	// Get messages before the middle one (index 2 = 3rd newest)
	beforeID := all[2].ID
	older, err := store.GetMessages(ctx, "slack:eng", 10, beforeID)
	if err != nil {
		t.Fatal(err)
	}

	// Should get 2 messages (the two oldest)
	if len(older) != 2 {
		t.Fatalf("expected 2 messages before ID %d, got %d", beforeID, len(older))
	}
	for _, m := range older {
		if m.ID >= beforeID {
			t.Errorf("message ID %d should be < %d", m.ID, beforeID)
		}
	}
}

// TestDispatchMentionFilter_ViaHTTP verifies @mention filtering end-to-end:
// subscribe one agent with mention_only=true, one without, dispatch a message
// with and without the @mention and verify delivery counts.
func TestDispatchMentionFilter_ViaHTTP(t *testing.T) {
	store := setupStore(t)
	svc, sender := setupService(store)
	ts := setupHandler(t, svc)

	// eng-01: gets all messages
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/notify/subscriptions",
		map[string]any{"channel": "slack:eng", "agent": "eng-01", "mention_only": false})
	assertStatus(t, resp, http.StatusCreated)
	_ = resp.Body.Close()

	// eng-02: mention-only
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/notify/subscriptions",
		map[string]any{"channel": "slack:eng", "agent": "eng-02", "mention_only": true})
	assertStatus(t, resp, http.StatusCreated)
	_ = resp.Body.Close()

	// Verify both subscriptions exist
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/notify/subscriptions/slack:eng", nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
	assertStatus(t, resp, http.StatusOK)
	subs := decodeJSONSlice[notify.Subscription](t, resp)
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", len(subs))
	}

	t.Run("message without mention — only eng-01 gets it", func(t *testing.T) {
		sender.mu.Lock()
		sender.calls = nil
		sender.mu.Unlock()

		svc.Dispatch("slack:eng", "slack", "external-user", "U001", "",
			"hey everyone, new deployment done", "msg-no-mention", nil, nil, nil)
		time.Sleep(150 * time.Millisecond)

		calls := sender.getCalls()
		if len(calls) != 1 {
			t.Fatalf("expected 1 delivery (eng-01), got %d: %v", len(calls), calls)
		}
		if calls[0].Name != "eng-01" {
			t.Errorf("expected delivery to eng-01, got %s", calls[0].Name)
		}
	})

	t.Run("message with @eng-02 mention — both get it", func(t *testing.T) {
		sender.mu.Lock()
		sender.calls = nil
		sender.mu.Unlock()

		svc.Dispatch("slack:eng", "slack", "external-user", "U001", "",
			"@eng-02 please review the PR", "msg-with-mention", nil, nil, nil)
		time.Sleep(150 * time.Millisecond)

		calls := sender.getCalls()
		if len(calls) != 2 {
			t.Fatalf("expected 2 deliveries, got %d: %v", len(calls), calls)
		}
		recipients := make(map[string]bool)
		for _, c := range calls {
			recipients[c.Name] = true
		}
		if !recipients["eng-01"] {
			t.Error("expected eng-01 to receive message")
		}
		if !recipients["eng-02"] {
			t.Error("expected eng-02 to receive message (was mentioned)")
		}
	})

	// Verify messages were stored in the activity feed
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/notify/activity/slack:eng", nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
	assertStatus(t, resp, http.StatusOK)
	entries := decodeJSONSlice[notify.DeliveryEntry](t, resp)
	if len(entries) == 0 {
		t.Error("expected delivery log entries after dispatch")
	}
}

// TestSelfSkip_ViaHTTP verifies an agent does not receive their own message.
func TestSelfSkip_ViaHTTP(t *testing.T) {
	store := setupStore(t)
	svc, sender := setupService(store)
	ts := setupHandler(t, svc)

	ctx := context.Background()

	for _, agentName := range []string{"eng-01", "eng-02"} {
		resp := doJSON(t, http.MethodPost, ts.URL+"/api/notify/subscriptions",
			map[string]any{"channel": "slack:eng", "agent": agentName, "mention_only": false})
		assertStatus(t, resp, http.StatusCreated)
		_ = resp.Body.Close()
	}

	subs, err := store.Subscribers(ctx, "slack:eng")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", len(subs))
	}

	// eng-01 sends — should NOT receive it back (self-skip)
	svc.Dispatch("slack:eng", "slack", "[slack] eng-01", "U001", "",
		"I just pushed a fix", "msg-self", nil, nil, nil)
	time.Sleep(150 * time.Millisecond)

	calls := sender.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 delivery (eng-02 only), got %d: %v", len(calls), calls)
	}
	if calls[0].Name != "eng-02" {
		t.Errorf("expected delivery to eng-02, got %q", calls[0].Name)
	}
}

// TestNotifyServiceUnavailable verifies 503 when service is not wired up.
func TestNotifyServiceUnavailable(t *testing.T) {
	h := handlers.NewGatewayHandler(nil, nil)
	// Intentionally NOT calling h.SetNotifyService(svc)

	mux := http.NewServeMux()
	h.Register(mux)
	handlers.NewAppsHandler(h, nil, nil, nil).Register(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/notify/subscriptions"},
		{http.MethodPost, "/api/notify/subscriptions"},
		{http.MethodGet, "/api/notify/subscriptions/slack:eng"},
		{http.MethodDelete, "/api/notify/subscriptions/slack:eng"},
		{http.MethodPatch, "/api/notify/subscriptions/slack:eng"},
		{http.MethodGet, "/api/notify/activity/slack:eng"},
		{http.MethodGet, "/api/apps/slack/channels/eng/agents"},
		{http.MethodPost, "/api/apps/slack/channels/eng/agents"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			body := strings.NewReader("{}")
			req, err := http.NewRequestWithContext(context.Background(), ep.method, ts.URL+ep.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			assertStatus(t, resp, http.StatusServiceUnavailable)
		})
	}
}

// TestSubscriptionRoundtrip verifies the full CRUD lifecycle via HTTP.
func TestSubscriptionRoundtrip(t *testing.T) {
	store := setupStore(t)
	svc, _ := setupService(store)
	ts := setupHandler(t, svc)

	// 1. Start with no subscriptions
	resp := doJSON(t, http.MethodGet, ts.URL+"/api/notify/subscriptions", nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
	assertStatus(t, resp, http.StatusOK)
	subs := decodeJSONSlice[notify.Subscription](t, resp)
	if len(subs) != 0 {
		t.Fatalf("expected 0 initial subscriptions, got %d", len(subs))
	}

	// 2. Subscribe eng-01
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/notify/subscriptions",
		map[string]any{"channel": "slack:eng", "agent": "eng-01", "mention_only": false})
	assertStatus(t, resp, http.StatusCreated)
	_ = resp.Body.Close()

	// 3. List channel — should have 1
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/notify/subscriptions/slack:eng", nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
	assertStatus(t, resp, http.StatusOK)
	subs = decodeJSONSlice[notify.Subscription](t, resp)
	if len(subs) != 1 || subs[0].Agent != "eng-01" {
		t.Fatalf("expected 1 subscription for eng-01, got %v", subs)
	}
	if subs[0].MentionOnly {
		t.Error("expected mention_only=false initially")
	}

	// 4. PATCH mention_only → true
	resp = doJSON(t, http.MethodPatch, ts.URL+"/api/notify/subscriptions/slack:eng",
		map[string]any{"agent": "eng-01", "mention_only": true})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 5. Verify mention_only is now true
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/notify/subscriptions/slack:eng", nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
	assertStatus(t, resp, http.StatusOK)
	subs = decodeJSONSlice[notify.Subscription](t, resp)
	if len(subs) != 1 || !subs[0].MentionOnly {
		t.Fatalf("expected mention_only=true, got %v", subs)
	}

	// 6. Also verify via GET /api/channels (should include slack:eng)
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/channels", nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
	assertStatus(t, resp, http.StatusOK)
	type legacyChannel struct {
		Name string `json:"name"`
	}
	var channels []legacyChannel
	decodeJSON(t, resp, &channels)
	var found bool
	for _, ch := range channels {
		if ch.Name == "slack:eng" {
			found = true
		}
	}
	if !found {
		t.Error("slack:eng should appear in /api/channels after subscription")
	}

	// 7. DELETE the subscription
	resp = doJSON(t, http.MethodDelete,
		ts.URL+"/api/notify/subscriptions/slack:eng?agent=eng-01", nil)
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 8. Verify it's gone
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/notify/subscriptions/slack:eng", nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
	assertStatus(t, resp, http.StatusOK)
	subs = decodeJSONSlice[notify.Subscription](t, resp)
	if len(subs) != 0 {
		t.Fatalf("expected 0 subscriptions after DELETE, got %d", len(subs))
	}
}

// TestDeliveryLogFields verifies that delivery log entries have expected fields.
func TestDeliveryLogFields(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()

	if err := store.LogDelivery(ctx, notify.DeliveryEntry{
		Channel: "slack:eng",
		Agent:   "eng-01",
		Status:  notify.StatusDelivered,
		Preview: "hello world",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.LogDelivery(ctx, notify.DeliveryEntry{
		Channel: "slack:eng",
		Agent:   "eng-02",
		Status:  notify.StatusFailed,
		Error:   "tmux session not found",
		Preview: "hello world",
	}); err != nil {
		t.Fatal(err)
	}

	svc, _ := setupService(store)
	ts := setupHandler(t, svc)

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/notify/activity/slack:eng", nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
	assertStatus(t, resp, http.StatusOK)

	var entries []notify.DeliveryEntry
	decodeJSON(t, resp, &entries)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	for _, e := range entries {
		if e.ID == 0 {
			t.Error("entry ID should be non-zero")
		}
		if e.Channel != "slack:eng" {
			t.Errorf("unexpected channel %q", e.Channel)
		}
		if e.Agent == "" {
			t.Error("agent should not be empty")
		}
		if e.Status == "" {
			t.Error("status should not be empty")
		}
		if e.Preview != "hello world" {
			t.Errorf("unexpected preview %q", e.Preview)
		}
	}

	// Verify status values are valid
	statuses := make(map[notify.DeliveryStatus]bool)
	for _, e := range entries {
		statuses[e.Status] = true
	}
	if !statuses[notify.StatusDelivered] {
		t.Error("expected StatusDelivered in entries")
	}
	if !statuses[notify.StatusFailed] {
		t.Error("expected StatusFailed in entries")
	}
}

// TestGatewayChannelAgents_MethodNotAllowed verifies methods not handled return 405.
func TestGatewayChannelAgents_MethodNotAllowed(t *testing.T) {
	store := setupStore(t)
	svc, _ := setupService(store)
	ts := setupHandler(t, svc)

	resp := doJSON(t, http.MethodPut, ts.URL+"/api/apps/slack/channels/eng/agents", nil)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusMethodNotAllowed)
}

// TestMultipleChannelsIndependence verifies channels don't share subscriber state.
func TestMultipleChannelsIndependence(t *testing.T) {
	store := setupStore(t)
	svc, _ := setupService(store)
	ts := setupHandler(t, svc)

	ctx := context.Background()

	// Subscribe agents to different channels
	channels := map[string]string{
		"slack:eng":    "eng-agent",
		"discord:ops":  "ops-agent",
		"telegram:mgr": "mgr-agent",
	}
	for ch, agent := range channels {
		_ = store.Subscribe(ctx, ch, agent, false)
	}

	// Verify each channel is independent
	for ch, expectedAgent := range channels {
		resp := doJSON(t, http.MethodGet, ts.URL+"/api/notify/subscriptions/"+ch, nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
		assertStatus(t, resp, http.StatusOK)

		var subs []notify.Subscription
		decodeJSON(t, resp, &subs)

		if len(subs) != 1 {
			t.Errorf("channel %q: expected 1 subscriber, got %d", ch, len(subs))
			continue
		}
		if subs[0].Agent != expectedAgent {
			t.Errorf("channel %q: expected agent %q, got %q", ch, expectedAgent, subs[0].Agent)
		}
	}

	// Total subscriptions should be 3
	resp := doJSON(t, http.MethodGet, ts.URL+"/api/notify/subscriptions", nil) //nolint:bodyclose // closed via t.Cleanup in doJSON
	assertStatus(t, resp, http.StatusOK)
	allSubs := decodeJSONSlice[notify.Subscription](t, resp)
	if len(allSubs) != 3 {
		t.Fatalf("expected 3 total subscriptions, got %d", len(allSubs))
	}
}
