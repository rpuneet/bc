package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/notify"
)

// newTestNotifyService opens an in-memory notify store wrapped in a Service.
func newTestNotifyService(t *testing.T) *notify.Service {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	store, err := notify.OpenStore(d, "sqlite")
	if err != nil {
		t.Fatalf("open notify store: %v", err)
	}
	return notify.NewService(store, nil, nil)
}

// TestStatsChannels exercises GET /api/stats/channels: method guard,
// nil-service fallback, empty store, and aggregation of seeded messages
// and subscriptions.
func TestStatsChannels(t *testing.T) {
	type seedMsg struct{ channel, sender string }
	type seedSub struct{ channel, agent string }

	tests := []struct {
		name       string
		method     string
		msgs       []seedMsg
		subs       []seedSub
		want       []notify.ChannelStat
		wantStatus int
		nilService bool
	}{
		{
			name:       "method not allowed",
			method:     http.MethodPost,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "nil notify service returns empty array",
			method:     http.MethodGet,
			nilService: true,
			wantStatus: http.StatusOK,
			want:       []notify.ChannelStat{},
		},
		{
			name:       "empty store returns empty array",
			method:     http.MethodGet,
			wantStatus: http.StatusOK,
			want:       []notify.ChannelStat{},
		},
		{
			name:   "aggregates messages and subscriptions",
			method: http.MethodGet,
			msgs: []seedMsg{
				{"slack:eng", "alice"},
				{"slack:eng", "bob"},
				{"telegram:ops", "carol"},
			},
			subs:       []seedSub{{"slack:eng", "eng-01"}},
			wantStatus: http.StatusOK,
			want: []notify.ChannelStat{
				{
					Name:         "slack:eng",
					MessageCount: 2,
					MemberCount:  1,
					TopSenders:   []notify.TopSender{{Sender: "alice", Count: 1}, {Sender: "bob", Count: 1}},
				},
				{
					Name:         "telegram:ops",
					MessageCount: 1,
					TopSenders:   []notify.TopSender{{Sender: "carol", Count: 1}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &StatsHandler{}
			if !tt.nilService {
				svc := newTestNotifyService(t)
				ctx := context.Background()
				for _, m := range tt.msgs {
					if err := svc.Store().SaveMessage(ctx, m.channel, m.sender, "", "hi"); err != nil {
						t.Fatalf("SaveMessage: %v", err)
					}
				}
				for _, s := range tt.subs {
					if err := svc.Subscribe(ctx, s.channel, s.agent, false); err != nil {
						t.Fatalf("Subscribe: %v", err)
					}
				}
				h.SetNotify(svc)
			}

			req := httptest.NewRequest(tt.method, "/api/stats/channels", nil)
			rr := httptest.NewRecorder()
			h.statsChannels(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				return
			}

			var got []notify.ChannelStat
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal response: %v (body: %s)", err, rr.Body.String())
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d channels, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				g := got[i]
				if g.Name != w.Name || g.MessageCount != w.MessageCount || g.MemberCount != w.MemberCount {
					t.Errorf("channel[%d] = {%s %d msgs %d members}, want {%s %d msgs %d members}",
						i, g.Name, g.MessageCount, g.MemberCount, w.Name, w.MessageCount, w.MemberCount)
				}
				if len(g.TopSenders) != len(w.TopSenders) {
					t.Errorf("channel[%d] top_senders = %+v, want %+v", i, g.TopSenders, w.TopSenders)
					continue
				}
				for j, h := range w.TopSenders {
					if g.TopSenders[j] != h {
						t.Errorf("channel[%d] top_senders[%d] = %+v, want %+v", i, j, g.TopSenders[j], h)
					}
				}
			}
		})
	}
}
