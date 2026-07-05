package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/notify"
)

// identityStubAdapter is a stubAdapter that also resolves channel identities.
type identityStubAdapter struct {
	meta map[string]gateway.ChannelMeta
	stubAdapter
}

func (s *identityStubAdapter) ResolveChannel(_ context.Context, platformID string) (gateway.ChannelMeta, error) {
	if m, ok := s.meta[platformID]; ok {
		return m, nil
	}
	return gateway.ChannelMeta{}, http.ErrMissingFile
}

// notifyStoreChannelStore adapts notify.Store to gateway.ChannelStore for tests
// (mirrors the server's channelPersister wiring).
type notifyStoreChannelStore struct {
	store *notify.Store
}

func (p *notifyStoreChannelStore) SaveChannel(ctx context.Context, bcChannel, platform, platformID string) error {
	return p.store.SaveChannel(ctx, bcChannel, platform, platformID)
}

func (p *notifyStoreChannelStore) LoadChannels(ctx context.Context) ([]gateway.PersistedChannel, error) {
	ncs, err := p.store.LoadChannels(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]gateway.PersistedChannel, len(ncs))
	for i, c := range ncs {
		result[i] = gateway.PersistedChannel{
			BCChannel:        c.BCChannel,
			Platform:         c.Platform,
			PlatformID:       c.PlatformID,
			DisplayName:      c.DisplayName,
			Kind:             c.Kind,
			ParticipantCount: c.ParticipantCount,
		}
	}
	return result, nil
}

func (p *notifyStoreChannelStore) UpsertChannelMeta(ctx context.Context, bcChannel, displayName, kind string, participantCount int) error {
	return p.store.UpsertChannelMeta(ctx, bcChannel, displayName, kind, participantCount)
}

type overviewResponse struct {
	Apps []struct {
		Name             string `json:"name"`
		Platform         string `json:"platform"`
		DisconnectReason string `json:"disconnect_reason"`
		ChannelCount     int    `json:"channel_count"`
		Connected        bool   `json:"connected"`
	} `json:"apps"`
	Channels []struct {
		BCChannel        string `json:"bc_channel"`
		Platform         string `json:"platform"`
		DisplayName      string `json:"display_name"`
		Kind             string `json:"kind"`
		ParticipantCount int    `json:"participant_count"`
		SubscriberCount  int    `json:"subscriber_count"`
		MessageCount     int    `json:"message_count"`
	} `json:"channels"`
}

// TestNotificationsOverview exercises GET /api/notifications/overview:
// channel metadata, message/subscriber counts, and adapter status.
func TestNotificationsOverview(t *testing.T) {
	svc := newTestNotifyService(t)
	store := svc.Store()
	ctx := context.Background()

	// Seed a resolved WhatsApp group channel with messages + a subscription.
	if err := store.SaveChannel(ctx, "whatsapp:family", "whatsapp", "1234@g.us"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertChannelMeta(ctx, "whatsapp:family", "Family Group", "group", 12); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := store.SaveMessage(ctx, "whatsapp:family", "alice", "hi"); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Subscribe(ctx, "whatsapp:family", "eng-01", false); err != nil {
		t.Fatal(err)
	}
	// A channel persisted without resolved metadata falls back to its suffix.
	if err := store.SaveChannel(ctx, "slack:general", "slack", "C01"); err != nil {
		t.Fatal(err)
	}

	gw := gateway.NewManager()
	gw.Register(&stubAdapter{name: "whatsapp", status: gateway.AdapterStatus{Connected: true}})
	gw.Register(&stubAdapter{name: "slack", status: gateway.AdapterStatus{Connected: false, Error: "invalid_auth"}})

	h := NewGatewayHandler(gw, nil)
	h.SetNotifyService(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/notifications/overview", nil)
	rr := httptest.NewRecorder()
	h.notificationsOverview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp overviewResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Apps) != 2 {
		t.Fatalf("apps = %d, want 2: %+v", len(resp.Apps), resp.Apps)
	}
	appsByName := map[string]int{}
	for i, a := range resp.Apps {
		appsByName[a.Name] = i
	}
	wa := resp.Apps[appsByName["whatsapp"]]
	if !wa.Connected || wa.ChannelCount != 1 {
		t.Fatalf("whatsapp app = %+v, want connected with 1 channel", wa)
	}
	sl := resp.Apps[appsByName["slack"]]
	if sl.Connected || sl.DisconnectReason != "invalid_auth" || sl.ChannelCount != 1 {
		t.Fatalf("slack app = %+v, want disconnected/invalid_auth with 1 channel", sl)
	}

	if len(resp.Channels) != 2 {
		t.Fatalf("channels = %d, want 2: %+v", len(resp.Channels), resp.Channels)
	}
	// Sorted by message count desc: whatsapp:family first.
	fam := resp.Channels[0]
	if fam.BCChannel != "whatsapp:family" || fam.DisplayName != "Family Group" ||
		fam.Kind != "group" || fam.ParticipantCount != 12 ||
		fam.MessageCount != 3 || fam.SubscriberCount != 1 {
		t.Fatalf("whatsapp:family = %+v", fam)
	}
	gen := resp.Channels[1]
	if gen.BCChannel != "slack:general" || gen.DisplayName != "general" || gen.Kind != "" {
		t.Fatalf("slack:general = %+v", gen)
	}
}

// TestNotificationsOverview_MethodAndNilService covers the guards.
func TestNotificationsOverview_MethodAndNilService(t *testing.T) {
	h := &GatewayHandler{}

	req := httptest.NewRequest(http.MethodPost, "/api/notifications/overview", nil)
	rr := httptest.NewRecorder()
	h.notificationsOverview(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/notifications/overview", nil)
	rr = httptest.NewRecorder()
	h.notificationsOverview(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil-service status = %d, want 503", rr.Code)
	}
}

// TestRefreshChannelMetaEndpoint exercises POST /api/gateways/channels/refresh:
// an inbound notification maps a channel, refresh re-resolves its identity.
func TestRefreshChannelMetaEndpoint(t *testing.T) {
	svc := newTestNotifyService(t)
	store := svc.Store()

	gw := gateway.NewManager()
	gw.SetChannelStore(&notifyStoreChannelStore{store: store})
	gw.Register(&identityStubAdapter{
		stubAdapter: stubAdapter{name: "whatsapp"},
		meta: map[string]gateway.ChannelMeta{
			"1234@g.us": {DisplayName: "Family Group", Kind: "group", ParticipantCount: 7},
		},
	})
	gw.HandleNotification("whatsapp", gateway.Notification{
		Channel:   "family",
		ChannelID: "1234@g.us",
		Platform:  "whatsapp",
		Sender:    "alice",
		Content:   "hello",
	})

	h := NewGatewayHandler(gw, nil)
	h.SetNotifyService(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/gateways/channels/refresh", nil)
	rr := httptest.NewRecorder()
	h.refreshChannelMeta(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Refreshed int `json:"refreshed"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Refreshed != 1 {
		t.Fatalf("refreshed = %d, want 1", resp.Refreshed)
	}

	channels, err := store.LoadChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range channels {
		if c.BCChannel == "whatsapp:family" {
			found = true
			if c.DisplayName != "Family Group" || c.Kind != "group" || c.ParticipantCount != 7 {
				t.Fatalf("meta not refreshed: %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("whatsapp:family not persisted")
	}

	// GET is rejected.
	rr = httptest.NewRecorder()
	h.refreshChannelMeta(rr, httptest.NewRequest(http.MethodGet, "/api/gateways/channels/refresh", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", rr.Code)
	}
}
