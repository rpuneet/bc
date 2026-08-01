package whatsapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// metaCacheTTL bounds how long resolved channel metadata is cached in the
// adapter. WhatsApp rate-limits info queries, so resolution is lazy and
// cached; a manual refresh after the TTL re-fetches from the platform.
const metaCacheTTL = 15 * time.Minute

// identityClient is the subset of whatsmeow.Client used for identity
// resolution, extracted so tests can inject a fake without a live session.
type identityClient interface {
	GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error)
	GetContact(ctx context.Context, jid types.JID) (types.ContactInfo, error)
	// ProfilePictureURL returns the downloadable URL of the profile picture
	// for a user or group JID, or "" when none exists / is not visible.
	ProfilePictureURL(ctx context.Context, jid types.JID) string
}

// waIdentityClient adapts *whatsmeow.Client to identityClient.
type waIdentityClient struct {
	c *whatsmeow.Client
}

func (w waIdentityClient) GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
	return w.c.GetGroupInfo(ctx, jid)
}

func (w waIdentityClient) GetContact(ctx context.Context, jid types.JID) (types.ContactInfo, error) {
	return w.c.Store.Contacts.GetContact(ctx, jid)
}

// ProfilePictureURL asks WhatsApp for the (thumbnail) profile-picture URL.
// A missing picture or a privacy setting returns nil,nil or an error from
// whatsmeow; either way we degrade to "" and the UI keeps the initials chip.
func (w waIdentityClient) ProfilePictureURL(ctx context.Context, jid types.JID) string {
	info, err := w.c.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: true})
	if err != nil || info == nil {
		return ""
	}
	return info.URL
}

// cachedMeta is a resolved ChannelMeta with its resolution time.
type cachedMeta struct {
	at   time.Time
	meta gateway.ChannelMeta
}

var _ gateway.ChannelIdentity = (*Adapter)(nil)

// identity returns the identity client: an injected fake in tests, otherwise
// the live whatsmeow client (nil until connected/paired).
func (a *Adapter) identity() identityClient {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.idClient != nil {
		return a.idClient
	}
	if a.client != nil {
		return waIdentityClient{a.client}
	}
	return nil
}

// ResolveChannel implements gateway.ChannelIdentity. platformID must be a
// WhatsApp JID: group JIDs (@g.us) resolve to the group subject and
// participant count; user JIDs (@s.whatsapp.net, @lid) resolve to the
// contact's name, falling back to a formatted phone number. Results are
// cached in-adapter because WhatsApp rate-limits info queries.
func (a *Adapter) ResolveChannel(ctx context.Context, platformID string) (gateway.ChannelMeta, error) {
	if !strings.Contains(platformID, "@") {
		return gateway.ChannelMeta{}, fmt.Errorf("whatsapp: %q is not a JID", platformID)
	}
	jid, err := types.ParseJID(platformID)
	if err != nil {
		return gateway.ChannelMeta{}, fmt.Errorf("whatsapp: parse JID %q: %w", platformID, err)
	}

	a.metaMu.Lock()
	if c, ok := a.metaCache[platformID]; ok && time.Since(c.at) < metaCacheTTL {
		a.metaMu.Unlock()
		return c.meta, nil
	}
	a.metaMu.Unlock()

	client := a.identity()
	if client == nil {
		return gateway.ChannelMeta{}, fmt.Errorf("whatsapp: not connected")
	}

	meta, err := resolveJID(ctx, client, jid)
	if err != nil {
		return gateway.ChannelMeta{}, err
	}

	a.metaMu.Lock()
	if a.metaCache == nil {
		a.metaCache = make(map[string]cachedMeta)
	}
	a.metaCache[platformID] = cachedMeta{at: time.Now(), meta: meta}
	a.metaMu.Unlock()
	return meta, nil
}

// resolveJID resolves a single JID to display metadata.
func resolveJID(ctx context.Context, client identityClient, jid types.JID) (gateway.ChannelMeta, error) {
	switch jid.Server {
	case types.GroupServer:
		info, err := client.GetGroupInfo(ctx, jid)
		if err != nil {
			return gateway.ChannelMeta{}, fmt.Errorf("whatsapp: group info %s: %w", jid, err)
		}
		count := len(info.Participants)
		if count == 0 {
			count = info.ParticipantCount
		}
		name := info.Name
		if name == "" {
			name = jid.User
		}
		return gateway.ChannelMeta{
			DisplayName:      name,
			Kind:             gateway.ChannelKindGroup,
			AvatarURL:        client.ProfilePictureURL(ctx, jid),
			ParticipantCount: count,
		}, nil

	case types.DefaultUserServer, types.HiddenUserServer:
		meta := gateway.ChannelMeta{Kind: gateway.ChannelKindPerson}
		if info, err := client.GetContact(ctx, jid); err == nil {
			meta.DisplayName = firstNonEmpty(info.FullName, info.BusinessName, info.PushName)
		}
		if meta.DisplayName == "" {
			meta.DisplayName = formatPhone(jid.User)
		}
		meta.AvatarURL = client.ProfilePictureURL(ctx, jid)
		return meta, nil

	default:
		return gateway.ChannelMeta{
			DisplayName: jid.User,
			Kind:        gateway.ChannelKindOther,
		}, nil
	}
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// formatPhone renders the user part of a JID as an international phone
// number ("+14155551234"). Non-numeric ids are returned unchanged.
func formatPhone(user string) string {
	if user == "" {
		return ""
	}
	for _, r := range user {
		if r < '0' || r > '9' {
			return user
		}
	}
	return "+" + user
}
