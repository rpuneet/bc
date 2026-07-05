package whatsapp

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"go.mau.fi/whatsmeow/types"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// fakeIdentityClient implements identityClient for tests — no live session.
type fakeIdentityClient struct {
	groups       map[string]*types.GroupInfo
	contacts     map[string]types.ContactInfo
	mu           sync.Mutex
	groupCalls   int
	contactCalls int
}

func (f *fakeIdentityClient) GetGroupInfo(_ context.Context, jid types.JID) (*types.GroupInfo, error) {
	f.mu.Lock()
	f.groupCalls++
	f.mu.Unlock()
	gi, ok := f.groups[jid.String()]
	if !ok {
		return nil, fmt.Errorf("group not found: %s", jid)
	}
	return gi, nil
}

func (f *fakeIdentityClient) GetContact(_ context.Context, jid types.JID) (types.ContactInfo, error) {
	f.mu.Lock()
	f.contactCalls++
	f.mu.Unlock()
	info, ok := f.contacts[jid.String()]
	if !ok {
		return types.ContactInfo{}, nil // whatsmeow returns Found=false, not an error
	}
	return info, nil
}

func newTestAdapter(t *testing.T, fake *fakeIdentityClient) *Adapter {
	t.Helper()
	a := New(t.TempDir())
	a.idClient = fake
	return a
}

func groupInfo(name string, participants int) *types.GroupInfo {
	gi := &types.GroupInfo{}
	gi.Name = name
	gi.Participants = make([]types.GroupParticipant, participants)
	return gi
}

func TestResolveChannel_Group(t *testing.T) {
	fake := &fakeIdentityClient{
		groups: map[string]*types.GroupInfo{
			"1234@g.us": groupInfo("Family Group", 12),
		},
	}
	a := newTestAdapter(t, fake)

	meta, err := a.ResolveChannel(context.Background(), "1234@g.us")
	if err != nil {
		t.Fatalf("ResolveChannel: %v", err)
	}
	want := gateway.ChannelMeta{DisplayName: "Family Group", Kind: "group", ParticipantCount: 12}
	if meta != want {
		t.Fatalf("got %+v, want %+v", meta, want)
	}
}

func TestResolveChannel_PersonContactName(t *testing.T) {
	fake := &fakeIdentityClient{
		contacts: map[string]types.ContactInfo{
			"14155551234@s.whatsapp.net": {Found: true, FullName: "Alice Smith", PushName: "alice"},
		},
	}
	a := newTestAdapter(t, fake)

	meta, err := a.ResolveChannel(context.Background(), "14155551234@s.whatsapp.net")
	if err != nil {
		t.Fatalf("ResolveChannel: %v", err)
	}
	if meta.DisplayName != "Alice Smith" || meta.Kind != "person" {
		t.Fatalf("got %+v, want Alice Smith/person", meta)
	}
}

func TestResolveChannel_PersonNameFallbacks(t *testing.T) {
	tests := []struct {
		name    string
		contact types.ContactInfo
		want    string
	}{
		{"business name", types.ContactInfo{Found: true, BusinessName: "Acme Corp"}, "Acme Corp"},
		{"push name", types.ContactInfo{Found: true, PushName: "bob"}, "bob"},
		{"phone number", types.ContactInfo{}, "+14155551234"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeIdentityClient{
				contacts: map[string]types.ContactInfo{
					"14155551234@s.whatsapp.net": tt.contact,
				},
			}
			a := newTestAdapter(t, fake)
			meta, err := a.ResolveChannel(context.Background(), "14155551234@s.whatsapp.net")
			if err != nil {
				t.Fatalf("ResolveChannel: %v", err)
			}
			if meta.DisplayName != tt.want {
				t.Fatalf("display_name = %q, want %q", meta.DisplayName, tt.want)
			}
			if meta.Kind != "person" {
				t.Fatalf("kind = %q, want person", meta.Kind)
			}
		})
	}
}

func TestResolveChannel_NotAJID(t *testing.T) {
	a := newTestAdapter(t, &fakeIdentityClient{})
	if _, err := a.ResolveChannel(context.Background(), "family-group"); err == nil {
		t.Fatal("expected error for non-JID platform id")
	}
}

func TestResolveChannel_CachesResults(t *testing.T) {
	fake := &fakeIdentityClient{
		groups: map[string]*types.GroupInfo{
			"1234@g.us": groupInfo("Family Group", 3),
		},
	}
	a := newTestAdapter(t, fake)

	for range 3 {
		if _, err := a.ResolveChannel(context.Background(), "1234@g.us"); err != nil {
			t.Fatalf("ResolveChannel: %v", err)
		}
	}
	if fake.groupCalls != 1 {
		t.Fatalf("expected 1 platform lookup (cached), got %d", fake.groupCalls)
	}
}

func TestResolveChannel_GroupLookupError(t *testing.T) {
	a := newTestAdapter(t, &fakeIdentityClient{})
	if _, err := a.ResolveChannel(context.Background(), "999@g.us"); err == nil {
		t.Fatal("expected error when group info lookup fails")
	}
}

func TestFormatPhone(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"14155551234", "+14155551234"},
		{"", ""},
		{"not-a-number", "not-a-number"},
	}
	for _, tt := range tests {
		if got := formatPhone(tt.in); got != tt.want {
			t.Errorf("formatPhone(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
