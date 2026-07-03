package notify

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// This file contains a one-time (but idempotent) normalization of legacy
// Discord channel keys. Three naming generations exist in old databases for
// the same Discord channel:
//
//  1. "discord:<channel>"                       — bare channel name, no guild
//  2. "discord:<guild><channel>"                — guild and channel
//     concatenated without a separator (old sanitizer stripped the ':')
//  3. "discord:<guild:with:colons>:<channel>"   — guild names containing
//     spaces/punctuation got colon-mangled into extra segments
//
// The canonical scheme is "discord:<guild-slug>:<channel-slug>". The
// normalization runs at store open, only touches rows matching the legacy
// patterns, and is a no-op once all keys are canonical.

const discordPrefix = "discord:"

// discordSlug mirrors the discord adapter's slugifier: lowercase, runs of
// spaces/colons/hyphens collapse to a single '-', other punctuation dropped,
// no leading/trailing separators. It is intentionally duplicated rather than
// imported so this data migration stays frozen even if the adapter's live
// scheme evolves.
func discordSlug(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	pendingSep := false
	for _, r := range name {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_':
			if pendingSep && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingSep = false
			b.WriteRune(r)
		case r == ' ' || r == ':' || r == '-':
			pendingSep = true
		}
		// All other runes are dropped.
	}
	return b.String()
}

// isCanonicalDiscordKey reports whether key already has the canonical form
// "discord:<guild-slug>:<channel-slug>".
func isCanonicalDiscordKey(key string) bool {
	rest := strings.TrimPrefix(key, discordPrefix)
	parts := strings.Split(rest, ":")
	if len(parts) != 2 {
		return false
	}
	guild, channel := parts[0], parts[1]
	return guild != "" && channel != "" &&
		guild == discordSlug(guild) && channel == discordSlug(channel)
}

// buildDiscordChannelMapping computes legacy key → canonical key for the
// given set of discord channel keys.
//
// Colon-mangled keys (extra ':' segments) normalize deterministically: the
// segment after the last colon is the channel, everything before it is the
// guild. Bare keys (no ':' after the prefix) cannot be split syntactically,
// so they are only rewritten when they unambiguously match exactly one known
// canonical key — either its channel slug, or its guild+channel
// concatenation (with or without a joining dash). Anything ambiguous or
// unresolvable is left untouched.
func buildDiscordChannelMapping(keys []string) map[string]string {
	canonical := make(map[string]struct{})
	var mangled, bare []string
	for _, k := range keys {
		rest := strings.TrimPrefix(k, discordPrefix)
		switch {
		case isCanonicalDiscordKey(k):
			canonical[k] = struct{}{}
		case strings.Contains(rest, ":"):
			mangled = append(mangled, k)
		default:
			bare = append(bare, k)
		}
	}

	mapping := make(map[string]string)

	for _, k := range mangled {
		rest := strings.TrimPrefix(k, discordPrefix)
		i := strings.LastIndex(rest, ":")
		guild, channel := discordSlug(rest[:i]), discordSlug(rest[i+1:])
		if guild == "" || channel == "" {
			continue // cannot normalize safely; leave untouched
		}
		canon := discordPrefix + guild + ":" + channel
		canonical[canon] = struct{}{}
		if canon != k {
			mapping[k] = canon
		}
	}

	for _, k := range bare {
		rest := discordSlug(strings.TrimPrefix(k, discordPrefix))
		if rest == "" {
			continue
		}
		var matches []string
		for canon := range canonical {
			seg := strings.SplitN(strings.TrimPrefix(canon, discordPrefix), ":", 2)
			guild, channel := seg[0], seg[1]
			if rest == channel || rest == guild+channel || rest == guild+"-"+channel {
				matches = append(matches, canon)
			}
		}
		if len(matches) == 1 {
			mapping[k] = matches[0]
		}
	}
	return mapping
}

// normalizeDiscordChannels rewrites legacy discord channel keys in
// notify_subscriptions, notify_messages, notify_delivery_log, and
// notify_channels to the canonical "discord:<guild>:<channel>" form, merging
// duplicates. For subscriptions, the newest row per (channel, agent) wins.
// Idempotent: once all keys are canonical this is a no-op.
func (s *Store) normalizeDiscordChannels(ctx context.Context) error {
	keys, err := s.discordChannelKeys(ctx)
	if err != nil {
		return fmt.Errorf("collect discord channels: %w", err)
	}
	mapping := buildDiscordChannelMapping(keys)
	if len(mapping) == 0 {
		return nil
	}

	legacy := make([]string, 0, len(mapping))
	for k := range mapping {
		legacy = append(legacy, k)
	}
	sort.Strings(legacy)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin discord normalization: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after successful commit

	for _, old := range legacy {
		canon := mapping[old]

		// Subscriptions: keep only the newest row per agent across the legacy
		// and canonical keys, then fold the survivors into the canonical key.
		if _, err := tx.ExecContext(ctx, s.q(
			`DELETE FROM notify_subscriptions
			 WHERE channel IN (?, ?)
			   AND id NOT IN (
			       SELECT MAX(id) FROM notify_subscriptions
			       WHERE channel IN (?, ?) GROUP BY agent
			   )`), old, canon, old, canon); err != nil {
			return fmt.Errorf("merge subscriptions %s -> %s: %w", old, canon, err)
		}
		if _, err := tx.ExecContext(ctx, s.q(
			`UPDATE notify_subscriptions SET channel = ? WHERE channel = ?`), canon, old); err != nil {
			return fmt.Errorf("rename subscriptions %s -> %s: %w", old, canon, err)
		}

		if _, err := tx.ExecContext(ctx, s.q(
			`UPDATE notify_messages SET channel = ? WHERE channel = ?`), canon, old); err != nil {
			return fmt.Errorf("rename messages %s -> %s: %w", old, canon, err)
		}

		if _, err := tx.ExecContext(ctx, s.q(
			`UPDATE notify_delivery_log SET channel = ? WHERE channel = ?`), canon, old); err != nil {
			return fmt.Errorf("rename delivery log %s -> %s: %w", old, canon, err)
		}

		// Channel mappings: bc_channel is the primary key, so keep the
		// canonical row when it exists and drop the legacy one; otherwise
		// rename the legacy row.
		if _, err := tx.ExecContext(ctx, s.q(
			`DELETE FROM notify_channels
			 WHERE bc_channel = ?
			   AND EXISTS (SELECT 1 FROM notify_channels WHERE bc_channel = ?)`), old, canon); err != nil {
			return fmt.Errorf("dedupe channels %s -> %s: %w", old, canon, err)
		}
		if _, err := tx.ExecContext(ctx, s.q(
			`UPDATE notify_channels SET bc_channel = ? WHERE bc_channel = ?`), canon, old); err != nil {
			return fmt.Errorf("rename channels %s -> %s: %w", old, canon, err)
		}
	}
	return tx.Commit()
}

// discordChannelKeys returns the distinct discord channel keys present in
// any of the notify tables.
func (s *Store) discordChannelKeys(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT channel FROM notify_subscriptions WHERE channel LIKE 'discord:%'
		 UNION SELECT channel FROM notify_messages WHERE channel LIKE 'discord:%'
		 UNION SELECT channel FROM notify_delivery_log WHERE channel LIKE 'discord:%'
		 UNION SELECT bc_channel FROM notify_channels WHERE bc_channel LIKE 'discord:%'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
