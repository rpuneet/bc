package notify

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// Store is the SQLite/Postgres-backed persistence layer for subscriptions
// and the delivery log. Uses the shared workspace database.
type Store struct {
	db     *db.DB
	driver string // "sqlite" or "timescale"
}

// OpenStore opens the notify store using the shared workspace database.
func OpenStore(workspacePath string) (*Store, error) {
	shared := db.SharedWrapped()
	if shared == nil {
		return nil, fmt.Errorf("notify store requires shared database (none available for workspace %s)", workspacePath)
	}
	driver := db.SharedDriver()
	s := &Store{db: shared, driver: driver}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("init notify schema: %w", err)
	}
	// One-time normalization of legacy discord channel keys (see
	// migrate_discord.go). Same ctx rationale as initSchema below.
	if err := s.normalizeDiscordChannels(context.TODO()); err != nil {
		return nil, fmt.Errorf("normalize discord channels: %w", err)
	}
	return s, nil
}

// Close is a no-op — the shared DB is owned by the caller.
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
	_, err := s.db.ExecContext(context.TODO(), schema)
	return err
}

const schemaSQLite = `
CREATE TABLE IF NOT EXISTS notify_subscriptions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    channel      TEXT NOT NULL,
    agent        TEXT NOT NULL,
    mention_only INTEGER NOT NULL DEFAULT 0,
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
    content   TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS notify_channels (
    bc_channel   TEXT PRIMARY KEY,
    platform     TEXT NOT NULL,
    platform_id  TEXT NOT NULL,
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_notify_subs_channel ON notify_subscriptions(channel);
CREATE INDEX IF NOT EXISTS idx_notify_subs_agent ON notify_subscriptions(agent);
CREATE INDEX IF NOT EXISTS idx_notify_delivery_channel ON notify_delivery_log(channel, id DESC);
CREATE INDEX IF NOT EXISTS idx_notify_messages_channel ON notify_messages(channel, id DESC);
`

const schemaPostgres = `
CREATE TABLE IF NOT EXISTS notify_subscriptions (
    id           BIGSERIAL PRIMARY KEY,
    channel      TEXT NOT NULL,
    agent        TEXT NOT NULL,
    mention_only INTEGER NOT NULL DEFAULT 0,
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
    content   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS notify_channels (
    bc_channel   TEXT PRIMARY KEY,
    platform     TEXT NOT NULL,
    platform_id  TEXT NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notify_subs_channel ON notify_subscriptions(channel);
CREATE INDEX IF NOT EXISTS idx_notify_subs_agent ON notify_subscriptions(agent);
CREATE INDEX IF NOT EXISTS idx_notify_delivery_channel ON notify_delivery_log(channel, id DESC);
CREATE INDEX IF NOT EXISTS idx_notify_messages_channel ON notify_messages(channel, id DESC);
`

// Subscribe adds an agent to a channel. If already subscribed, this is a no-op.
func (s *Store) Subscribe(ctx context.Context, channel, agent string, mentionOnly bool) error {
	mentionInt := 0
	if mentionOnly {
		mentionInt = 1
	}
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO notify_subscriptions (channel, agent, mention_only)
		 VALUES (?, ?, ?)
		 ON CONFLICT(channel, agent) DO UPDATE SET mention_only = excluded.mention_only`),
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

// Subscribers returns all subscriptions for a channel.
func (s *Store) Subscribers(ctx context.Context, channel string) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, s.q(
		`SELECT id, channel, agent, mention_only, created_at FROM notify_subscriptions WHERE channel = ?`),
		channel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var subs []Subscription
	for rows.Next() {
		var sub Subscription
		var mentionInt int
		var createdStr string
		if err := rows.Scan(&sub.ID, &sub.Channel, &sub.Agent, &mentionInt, &createdStr); err != nil {
			return nil, err
		}
		sub.MentionOnly = mentionInt != 0
		sub.CreatedAt, _ = time.Parse(time.RFC3339, createdStr) //nolint:errcheck // DB-written timestamp
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// AllSubscriptions returns all subscriptions across all channels.
func (s *Store) AllSubscriptions(ctx context.Context) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, channel, agent, mention_only, created_at FROM notify_subscriptions ORDER BY channel, agent`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var subs []Subscription
	for rows.Next() {
		var sub Subscription
		var mentionInt int
		var createdStr string
		if err := rows.Scan(&sub.ID, &sub.Channel, &sub.Agent, &mentionInt, &createdStr); err != nil {
			return nil, err
		}
		sub.MentionOnly = mentionInt != 0
		sub.CreatedAt, _ = time.Parse(time.RFC3339, createdStr) //nolint:errcheck // DB-written timestamp
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// LogDelivery records a delivery attempt.
func (s *Store) LogDelivery(ctx context.Context, e DeliveryEntry) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO notify_delivery_log (channel, agent, status, error, preview)
		 VALUES (?, ?, ?, ?, ?)`),
		e.Channel, e.Agent, string(e.Status), e.Error, e.Preview)
	return err
}

// RecentActivity returns the most recent delivery log entries for a channel.
func (s *Store) RecentActivity(ctx context.Context, channel string, limit int) ([]DeliveryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, s.q(
		`SELECT id, logged_at, channel, agent, status, COALESCE(error, ''), COALESCE(preview, '')
		 FROM notify_delivery_log
		 WHERE channel = ?
		 ORDER BY id DESC
		 LIMIT ?`),
		channel, limit)
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

// MessageRecord is a stored inbound gateway message for the activity feed.
type MessageRecord struct {
	CreatedAt time.Time `json:"created_at"`
	Channel   string    `json:"channel"`
	Sender    string    `json:"sender"`
	Content   string    `json:"content"`
	ID        int64     `json:"id"`
}

// SaveMessage stores an inbound gateway message for the activity feed.
func (s *Store) SaveMessage(ctx context.Context, channel, sender, content string) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO notify_messages (channel, sender, content) VALUES (?, ?, ?)`),
		channel, sender, content)
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
			`SELECT id, channel, sender, content, created_at FROM notify_messages
			 WHERE channel = ? AND id < ? ORDER BY id DESC LIMIT ?`),
			channel, before, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, s.q(
			`SELECT id, channel, sender, content, created_at FROM notify_messages
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
		if err := rows.Scan(&m.ID, &m.Channel, &m.Sender, &m.Content, &createdStr); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdStr) //nolint:errcheck // DB-written timestamp
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// PersistedChannel is a saved bc_channel → platform_id mapping.
type PersistedChannel struct {
	BCChannel  string
	Platform   string
	PlatformID string
}

// LoadChannels returns all persisted channel mappings.
func (s *Store) LoadChannels(ctx context.Context) ([]PersistedChannel, error) {
	rows, err := s.db.QueryContext(ctx, s.q(
		`SELECT bc_channel, platform, platform_id FROM notify_channels`))
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // read-only query

	var channels []PersistedChannel
	for rows.Next() {
		var c PersistedChannel
		if err := rows.Scan(&c.BCChannel, &c.Platform, &c.PlatformID); err != nil {
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
func (s *Store) SaveChannel(ctx context.Context, bcChannel, platform, platformID string) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO notify_channels (bc_channel, platform, platform_id, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(bc_channel) DO UPDATE SET
		   platform = excluded.platform,
		   platform_id = CASE
		     WHEN notify_channels.platform_id IS NULL OR notify_channels.platform_id = ''
		       THEN excluded.platform_id
		     ELSE notify_channels.platform_id
		   END,
		   updated_at = excluded.updated_at`),
		bcChannel, platform, platformID, time.Now().UTC().Format(time.RFC3339))
	return err
}
