package notify

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// Store is the SQLite/Postgres-backed persistence layer for subscriptions
// and the delivery log. It borrows the global database handle passed
// to OpenStore and never closes it.
type Store struct {
	db     *db.DB
	driver string // "sqlite" or "timescale"
}

// OpenStore opens the notify store on the given database.
// The handle is borrowed: callers (typically the global db
// registry) own its lifecycle.
func OpenStore(d *db.DB, driver string) (*Store, error) {
	if d == nil {
		return nil, fmt.Errorf("notify store requires a database (nil handle)")
	}
	s := &Store{db: d, driver: driver}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("init notify schema: %w", err)
	}
	return s, nil
}

// Close is a no-op — the global DB is owned by the caller.
func (s *Store) Close() error { return nil }

// q converts ? placeholders to $1, $2, ... for Postgres. No-op for SQLite.
func (s *Store) q(query string) string {
	if s.driver != "timescale" {
		return query
	}
	var b strings.Builder
	n := 0
	for i := range len(query) {
		if query[i] == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
		} else {
			b.WriteByte(query[i])
		}
	}
	return b.String()
}

func (s *Store) initSchema() error {
	var schema string
	if s.driver == "timescale" {
		schema = schemaPostgres
	} else {
		schema = schemaSQLite
	}
	// context.TODO() retained: initSchema runs synchronously during OpenStore at
	// startup before any request context exists; threading ctx through would
	// force a public API change on OpenStore and dozens of call sites across
	// tests/services for no operational benefit (schema DDL is fire-and-forget).
	if _, err := s.db.ExecContext(context.TODO(), schema); err != nil {
		return err
	}
	// Additive column migrations for databases created before real-identity
	// avatars landed. Both are idempotent — the error is ignored when the
	// column already exists (mirrors pkg/home.RoleStore's ALTER pattern).
	for _, alter := range []string{
		`ALTER TABLE notify_channels ADD COLUMN avatar_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notify_messages ADD COLUMN sender_avatar TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notify_subscriptions ADD COLUMN muted INTEGER NOT NULL DEFAULT 0`,
	} {
		_, _ = s.db.ExecContext(context.TODO(), alter) //nolint:errcheck // ignore if column already exists
	}
	if err := s.migrateLegacyCatchAll(context.TODO()); err != nil {
		return fmt.Errorf("migrate legacy catch-all: %w", err)
	}
	return nil
}

// migrateLegacyCatchAll rewrites "{platform}:general" subscription rows to
// "{platform}:*" (#3467) so Slack/Discord "#general" is no longer overloaded
// as the catch-all key. notify_channels / message history for ":general" are
// left alone — those are real channel traffic.
func (s *Store) migrateLegacyCatchAll(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, s.q(
		`SELECT channel, agent, mention_only, muted FROM notify_subscriptions`))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type row struct {
		channel, agent string
		mentionOnly    bool
		muted          bool
	}
	var legacy []row
	for rows.Next() {
		var r row
		var mentionInt, mutedInt int
		if err := rows.Scan(&r.channel, &r.agent, &mentionInt, &mutedInt); err != nil {
			return err
		}
		r.mentionOnly = mentionInt != 0
		r.muted = mutedInt != 0
		if IsLegacyCatchAll(r.channel) {
			legacy = append(legacy, r)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range legacy {
		platform := PlatformOf(r.channel)
		canonical := CatchAllChannel(platform)
		if canonical == "" {
			continue
		}
		// Upsert onto the canonical key, then drop the legacy row.
		if err := s.Subscribe(ctx, canonical, r.agent, r.mentionOnly); err != nil {
			return err
		}
		if r.muted {
			if err := s.SetMuted(ctx, canonical, r.agent, true); err != nil {
				return err
			}
		}
		if err := s.Unsubscribe(ctx, r.channel, r.agent); err != nil {
			return err
		}
	}
	return nil
}

const schemaSQLite = `
CREATE TABLE IF NOT EXISTS notify_subscriptions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    channel      TEXT NOT NULL,
    agent        TEXT NOT NULL,
    mention_only INTEGER NOT NULL DEFAULT 0,
    muted        INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(channel, agent)
);

CREATE TABLE IF NOT EXISTS notify_delivery_log (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    logged_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    channel   TEXT NOT NULL,
    agent     TEXT NOT NULL,
    status    TEXT NOT NULL CHECK(status IN ('delivered', 'failed', 'pending')),
    error     TEXT,
    preview   TEXT
);

CREATE TABLE IF NOT EXISTS notify_gateways (
    name         TEXT PRIMARY KEY,
    enabled      INTEGER NOT NULL DEFAULT 0,
    connected    INTEGER NOT NULL DEFAULT 0,
    last_seen_at TEXT,
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS notify_messages (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    channel   TEXT NOT NULL,
    sender    TEXT NOT NULL,
    sender_avatar TEXT NOT NULL DEFAULT '',
    content   TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS notify_channels (
    channel   TEXT PRIMARY KEY,
    platform     TEXT NOT NULL,
    platform_id  TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    kind         TEXT NOT NULL DEFAULT '',
    avatar_url   TEXT NOT NULL DEFAULT '',
    participant_count INTEGER NOT NULL DEFAULT 0,
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_notify_subs_channel ON notify_subscriptions(channel);
CREATE INDEX IF NOT EXISTS idx_notify_subs_agent ON notify_subscriptions(agent);
CREATE INDEX IF NOT EXISTS idx_notify_delivery_channel ON notify_delivery_log(channel, id DESC);
CREATE INDEX IF NOT EXISTS idx_notify_messages_channel ON notify_messages(channel, id DESC);
-- Covering index for ChannelStats' per-(channel,sender) aggregation: turns the
-- GROUP BY channel, sender scan into a covering-index walk with no temp b-tree.
CREATE INDEX IF NOT EXISTS idx_notify_messages_chan_sender ON notify_messages(channel, sender);
`

const schemaPostgres = `
CREATE TABLE IF NOT EXISTS notify_subscriptions (
    id           BIGSERIAL PRIMARY KEY,
    channel      TEXT NOT NULL,
    agent        TEXT NOT NULL,
    mention_only INTEGER NOT NULL DEFAULT 0,
    muted        INTEGER NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(channel, agent)
);

CREATE TABLE IF NOT EXISTS notify_delivery_log (
    id        BIGSERIAL PRIMARY KEY,
    logged_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    channel   TEXT NOT NULL,
    agent     TEXT NOT NULL,
    status    TEXT NOT NULL CHECK(status IN ('delivered', 'failed', 'pending')),
    error     TEXT,
    preview   TEXT
);

CREATE TABLE IF NOT EXISTS notify_gateways (
    name         TEXT PRIMARY KEY,
    enabled      INTEGER NOT NULL DEFAULT 0,
    connected    INTEGER NOT NULL DEFAULT 0,
    last_seen_at TIMESTAMPTZ,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS notify_messages (
    id        BIGSERIAL PRIMARY KEY,
    channel   TEXT NOT NULL,
    sender    TEXT NOT NULL,
    sender_avatar TEXT NOT NULL DEFAULT '',
    content   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS notify_channels (
    channel   TEXT PRIMARY KEY,
    platform     TEXT NOT NULL,
    platform_id  TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    kind         TEXT NOT NULL DEFAULT '',
    avatar_url   TEXT NOT NULL DEFAULT '',
    participant_count INTEGER NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notify_subs_channel ON notify_subscriptions(channel);
CREATE INDEX IF NOT EXISTS idx_notify_subs_agent ON notify_subscriptions(agent);
CREATE INDEX IF NOT EXISTS idx_notify_delivery_channel ON notify_delivery_log(channel, id DESC);
CREATE INDEX IF NOT EXISTS idx_notify_messages_channel ON notify_messages(channel, id DESC);
CREATE INDEX IF NOT EXISTS idx_notify_messages_chan_sender ON notify_messages(channel, sender);
`

// Subscribe adds an agent to a channel. If already subscribed, this is a no-op
// for the row and clears muted (an explicit subscribe means deliver).
func (s *Store) Subscribe(ctx context.Context, channel, agent string, mentionOnly bool) error {
	mentionInt := 0
	if mentionOnly {
		mentionInt = 1
	}
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO notify_subscriptions (channel, agent, mention_only, muted)
		 VALUES (?, ?, ?, 0)
		 ON CONFLICT(channel, agent) DO UPDATE SET mention_only = excluded.mention_only, muted = 0`),
		channel, agent, mentionInt)
	return err
}

// Unsubscribe removes an agent from a channel.
func (s *Store) Unsubscribe(ctx context.Context, channel, agent string) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`DELETE FROM notify_subscriptions WHERE channel = ? AND agent = ?`),
		channel, agent)
	return err
}

// SetMentionOnly updates the mention_only flag for a subscription.
func (s *Store) SetMentionOnly(ctx context.Context, channel, agent string, mentionOnly bool) error {
	mentionInt := 0
	if mentionOnly {
		mentionInt = 1
	}
	_, err := s.db.ExecContext(ctx, s.q(
		`UPDATE notify_subscriptions SET mention_only = ? WHERE channel = ? AND agent = ?`),
		mentionInt, channel, agent)
	return err
}

// SetMuted upserts a mute row for (channel, agent). muted=true suppresses
// catch-all delivery; muted=false removes the mute row so catch-all applies
// again (#3466).
func (s *Store) SetMuted(ctx context.Context, channel, agent string, muted bool) error {
	if !muted {
		return s.Unsubscribe(ctx, channel, agent)
	}
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO notify_subscriptions (channel, agent, mention_only, muted)
		 VALUES (?, ?, 0, 1)
		 ON CONFLICT(channel, agent) DO UPDATE SET muted = 1`),
		channel, agent)
	return err
}

// Subscribers returns all subscriptions for a channel (including muted rows).
func (s *Store) Subscribers(ctx context.Context, channel string) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, s.q(
		`SELECT id, channel, agent, mention_only, muted, created_at FROM notify_subscriptions WHERE channel = ?`),
		channel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var subs []Subscription
	for rows.Next() {
		var sub Subscription
		var mentionInt, mutedInt int
		var createdStr string
		if err := rows.Scan(&sub.ID, &sub.Channel, &sub.Agent, &mentionInt, &mutedInt, &createdStr); err != nil {
			return nil, err
		}
		sub.MentionOnly = mentionInt != 0
		sub.Muted = mutedInt != 0
		sub.CreatedAt, _ = time.Parse(time.RFC3339, createdStr) //nolint:errcheck // DB-written timestamp
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// AllSubscriptions returns all subscriptions across all channels.
func (s *Store) AllSubscriptions(ctx context.Context) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, channel, agent, mention_only, muted, created_at FROM notify_subscriptions ORDER BY channel, agent`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var subs []Subscription
	for rows.Next() {
		var sub Subscription
		var mentionInt, mutedInt int
		var createdStr string
		if err := rows.Scan(&sub.ID, &sub.Channel, &sub.Agent, &mentionInt, &mutedInt, &createdStr); err != nil {
			return nil, err
		}
		sub.MentionOnly = mentionInt != 0
		sub.Muted = mutedInt != 0
		sub.CreatedAt, _ = time.Parse(time.RFC3339, createdStr) //nolint:errcheck // DB-written timestamp
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// ChannelsForAgent returns the channel keys an agent is subscribed to
// (excluding muted rows). Used to populate the mycel-managed prompt (#3648).
func (s *Store) ChannelsForAgent(ctx context.Context, agent string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, s.q(
		`SELECT channel FROM notify_subscriptions WHERE agent = ? AND muted = 0 ORDER BY channel`), agent)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var ch string
		if err := rows.Scan(&ch); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// LogDelivery records a delivery attempt.
func (s *Store) LogDelivery(ctx context.Context, e DeliveryEntry) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO notify_delivery_log (channel, agent, status, error, preview)
		 VALUES (?, ?, ?, ?, ?)`),
		e.Channel, e.Agent, string(e.Status), e.Error, e.Preview)
	return err
}

// RecentActivity returns the most recent delivery log entries for a channel,
// newest first. When before > 0, only entries with id < before are returned,
// enabling cursor pagination for older pages (the id column is indexed).
func (s *Store) RecentActivity(ctx context.Context, channel string, limit int, before int64) ([]DeliveryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if before > 0 {
		rows, err = s.db.QueryContext(ctx, s.q(
			`SELECT id, logged_at, channel, agent, status, COALESCE(error, ''), COALESCE(preview, '')
			 FROM notify_delivery_log
			 WHERE channel = ? AND id < ?
			 ORDER BY id DESC
			 LIMIT ?`),
			channel, before, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, s.q(
			`SELECT id, logged_at, channel, agent, status, COALESCE(error, ''), COALESCE(preview, '')
			 FROM notify_delivery_log
			 WHERE channel = ?
			 ORDER BY id DESC
			 LIMIT ?`),
			channel, limit)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var entries []DeliveryEntry
	for rows.Next() {
		var e DeliveryEntry
		var loggedStr string
		if err := rows.Scan(&e.ID, &loggedStr, &e.Channel, &e.Agent, &e.Status, &e.Error, &e.Preview); err != nil {
			return nil, err
		}
		e.LoggedAt, _ = time.Parse(time.RFC3339, loggedStr) //nolint:errcheck // DB-written timestamp
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// PruneActivity deletes old delivery log entries, keeping the most recent keepLast per channel.
func (s *Store) PruneActivity(ctx context.Context, channel string, keepLast int) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`DELETE FROM notify_delivery_log
		 WHERE channel = ? AND id NOT IN (
		     SELECT id FROM notify_delivery_log WHERE channel = ? ORDER BY id DESC LIMIT ?
		 )`),
		channel, channel, keepLast)
	return err
}

// DeliveryChannels returns the distinct channel names in the delivery log.
func (s *Store) DeliveryChannels(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT channel FROM notify_delivery_log`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var channels []string
	for rows.Next() {
		var ch string
		if err := rows.Scan(&ch); err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

// TotalMessageCount returns the total number of stored messages across all channels.
func (s *Store) TotalMessageCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notify_messages`).Scan(&count)
	return count, err
}

// TopSender is a sender ranked by message count within a channel.
type TopSender struct {
	Sender string `json:"sender"`
	Count  int    `json:"count"`
}

// ChannelStat aggregates per-channel notification activity for the
// /api/stats/channels endpoint: message volume, subscriber count, last
// activity, and the most active senders.
type ChannelStat struct {
	LastActivity time.Time   `json:"last_activity"`
	Name         string      `json:"name"`
	TopSenders   []TopSender `json:"top_senders"`
	MessageCount int         `json:"message_count"`
	MemberCount  int         `json:"member_count"`
}

// channelStatsTopSenders caps the per-channel top_senders list.
const channelStatsTopSenders = 5

// ChannelStats returns aggregated per-channel activity: message counts and
// last activity from notify_messages, subscriber counts from
// notify_subscriptions, and the top senders per channel. Channels with
// subscriptions but no messages are included with a zero message count.
// Results are sorted by message count (descending), then by name.
func (s *Store) ChannelStats(ctx context.Context) ([]ChannelStat, error) {
	byName := make(map[string]*ChannelStat)
	get := func(name string) *ChannelStat {
		cs, ok := byName[name]
		if !ok {
			cs = &ChannelStat{Name: name}
			byName[name] = cs
		}
		return cs
	}

	// Message counts + last activity per channel. created_at is TEXT on
	// SQLite but TIMESTAMPTZ on Postgres, where pgx's binary mode can't
	// scan it into a string — cast to text so both drivers agree.
	msgRows, err := s.db.QueryContext(ctx,
		`SELECT channel, COUNT(*), CAST(MAX(created_at) AS TEXT) FROM notify_messages GROUP BY channel`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = msgRows.Close() }()
	for msgRows.Next() {
		var name, last string
		var count int
		if err = msgRows.Scan(&name, &count, &last); err != nil {
			return nil, err
		}
		cs := get(name)
		cs.MessageCount = count
		if ts, parseErr := parseStoredTime(last); parseErr == nil {
			cs.LastActivity = ts
		}
	}
	if err = msgRows.Err(); err != nil {
		return nil, err
	}

	// Subscriber counts per channel.
	subRows, err := s.db.QueryContext(ctx,
		`SELECT channel, COUNT(*) FROM notify_subscriptions GROUP BY channel`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = subRows.Close() }()
	for subRows.Next() {
		var name string
		var count int
		if err = subRows.Scan(&name, &count); err != nil {
			return nil, err
		}
		get(name).MemberCount = count
	}
	if err = subRows.Err(); err != nil {
		return nil, err
	}

	// Top senders per channel: rows arrive ordered by count, so the first
	// channelStatsTopSenders rows per channel are its top senders.
	senderRows, err := s.db.QueryContext(ctx,
		`SELECT channel, sender, COUNT(*) AS c FROM notify_messages
		 GROUP BY channel, sender ORDER BY c DESC, sender ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = senderRows.Close() }()
	for senderRows.Next() {
		var name, sender string
		var count int
		if err = senderRows.Scan(&name, &sender, &count); err != nil {
			return nil, err
		}
		cs := get(name)
		if len(cs.TopSenders) < channelStatsTopSenders {
			cs.TopSenders = append(cs.TopSenders, TopSender{Sender: sender, Count: count})
		}
	}
	if err = senderRows.Err(); err != nil {
		return nil, err
	}

	out := make([]ChannelStat, 0, len(byName))
	for _, cs := range byName {
		out = append(out, *cs)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MessageCount != out[j].MessageCount {
			return out[i].MessageCount > out[j].MessageCount
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// MessageRecord is a stored inbound gateway message for the activity feed.
type MessageRecord struct {
	CreatedAt time.Time `json:"created_at"`
	Channel   string    `json:"channel"`
	Sender    string    `json:"sender"`
	AvatarURL string    `json:"avatar_url,omitempty"` // raw platform avatar URL for the sender, "" when none
	Content   string    `json:"content"`
	ID        int64     `json:"id"`
}

// SaveMessage stores an inbound gateway message for the activity feed.
// senderAvatar is the raw platform avatar URL for the sender ("" when the
// adapter could not resolve one).
func (s *Store) SaveMessage(ctx context.Context, channel, sender, senderAvatar, content string) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO notify_messages (channel, sender, sender_avatar, content) VALUES (?, ?, ?, ?)`),
		channel, sender, senderAvatar, content)
	return err
}

// GetMessages returns recent messages for a channel (newest first).
func (s *Store) GetMessages(ctx context.Context, channel string, limit int, before int64) ([]MessageRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if before > 0 {
		rows, err = s.db.QueryContext(ctx, s.q(
			`SELECT id, channel, sender, sender_avatar, content, created_at FROM notify_messages
			 WHERE channel = ? AND id < ? ORDER BY id DESC LIMIT ?`),
			channel, before, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, s.q(
			`SELECT id, channel, sender, sender_avatar, content, created_at FROM notify_messages
			 WHERE channel = ? ORDER BY id DESC LIMIT ?`),
			channel, limit)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var msgs []MessageRecord
	for rows.Next() {
		var m MessageRecord
		var createdStr string
		if err := rows.Scan(&m.ID, &m.Channel, &m.Sender, &m.AvatarURL, &m.Content, &createdStr); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdStr) //nolint:errcheck // DB-written timestamp
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// PersistedChannel is a saved channel → platform_id mapping with
// resolved display metadata.
type PersistedChannel struct {
	Channel          string
	Platform         string
	PlatformID       string
	DisplayName      string
	Kind             string // group | person | channel | feed | other
	AvatarURL        string // raw platform avatar URL (person photo / group icon), "" when none
	ParticipantCount int
}

// LoadChannels returns all persisted channel mappings.
func (s *Store) LoadChannels(ctx context.Context) ([]PersistedChannel, error) {
	rows, err := s.db.QueryContext(ctx, s.q(
		`SELECT channel, platform, platform_id, display_name, kind, avatar_url, participant_count FROM notify_channels`))
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // read-only query

	var channels []PersistedChannel
	for rows.Next() {
		var c PersistedChannel
		if err := rows.Scan(&c.Channel, &c.Platform, &c.PlatformID, &c.DisplayName, &c.Kind, &c.AvatarURL, &c.ParticipantCount); err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

// SaveChannel persists a channel mapping so it survives server restarts.
//
// On conflict, platform_id is preserved if the existing row already has a
// non-empty value. This prevents a later event with a fallback platform_id
// (e.g., the channel name when a numeric chat_id couldn't be extracted from
// the raw payload) from clobbering a previously-stored real platform_id.
// UpdateChannelPlatformID force-overwrites a channel's platform id.
// SaveChannel deliberately preserves existing non-empty ids; this is the
// explicit upgrade path for fallback→native id promotions.
func (s *Store) UpdateChannelPlatformID(ctx context.Context, channel, platformID string) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`UPDATE notify_channels SET platform_id = ?, updated_at = ? WHERE channel = ?`),
		platformID, time.Now().UTC().Format(time.RFC3339), channel)
	return err
}

func (s *Store) SaveChannel(ctx context.Context, channel, platform, platformID string) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO notify_channels (channel, platform, platform_id, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(channel) DO UPDATE SET
		   platform = excluded.platform,
		   platform_id = CASE
		     WHEN notify_channels.platform_id IS NULL OR notify_channels.platform_id = ''
		       THEN excluded.platform_id
		     ELSE notify_channels.platform_id
		   END,
		   updated_at = excluded.updated_at`),
		channel, platform, platformID, time.Now().UTC().Format(time.RFC3339))
	return err
}

// UpsertChannelMeta stores resolved display metadata for a channel. The row
// is created if the mapping does not exist yet (platform derived from the
// channel prefix); existing platform/platform_id values are never touched.
// Empty display_name/kind/avatar_url and a zero participant_count never
// clobber previously-resolved values.
func (s *Store) UpsertChannelMeta(ctx context.Context, channel, displayName, kind, avatarURL string, participantCount int) error {
	platform := channel
	if i := strings.Index(channel, ":"); i > 0 {
		platform = channel[:i]
	}
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO notify_channels (channel, platform, platform_id, display_name, kind, avatar_url, participant_count, updated_at)
		 VALUES (?, ?, '', ?, ?, ?, ?, ?)
		 ON CONFLICT(channel) DO UPDATE SET
		   display_name = CASE
		     WHEN excluded.display_name = '' THEN notify_channels.display_name
		     ELSE excluded.display_name
		   END,
		   kind = CASE
		     WHEN excluded.kind = '' THEN notify_channels.kind
		     ELSE excluded.kind
		   END,
		   avatar_url = CASE
		     WHEN excluded.avatar_url = '' THEN notify_channels.avatar_url
		     ELSE excluded.avatar_url
		   END,
		   participant_count = CASE
		     WHEN excluded.participant_count = 0 THEN notify_channels.participant_count
		     ELSE excluded.participant_count
		   END,
		   updated_at = excluded.updated_at`),
		channel, platform, displayName, kind, avatarURL, participantCount, time.Now().UTC().Format(time.RFC3339))
	return err
}

// parseStoredTime parses a created_at value as either driver returns it:
// RFC3339 text (SQLite writes) or Postgres timestamp text
// ("2006-01-02 15:04:05.999999+00") after the CAST(... AS TEXT).
func parseStoredTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999-07",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized timestamp %q", s)
}
