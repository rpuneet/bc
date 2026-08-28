package notify

import (
	"sort"
	"strings"
)

// catchAllSuffix is the reserved channel leaf for platform-wide fallback
// subscriptions (#3467). It cannot collide with a real Slack/Discord
// "#general" channel the way the legacy ":general" leaf did.
const catchAllSuffix = ":*"

// legacyCatchAllSuffix is the pre-#3467 catch-all leaf. Still read (and
// migrated to catchAllSuffix) so existing workspaces keep delivering.
const legacyCatchAllSuffix = ":general"

// CatchAllChannel returns the catch-all subscription key for a platform
// ("{platform}:*"). Empty platform yields an empty string.
func CatchAllChannel(platform string) string {
	if platform == "" {
		return ""
	}
	return platform + catchAllSuffix
}

// LegacyCatchAllChannel returns the pre-#3467 catch-all key ("{platform}:general").
func LegacyCatchAllChannel(platform string) string {
	if platform == "" {
		return ""
	}
	return platform + legacyCatchAllSuffix
}

// PlatformOf returns the adapter/instance prefix of a channel key.
// Channel keys are "{adapter.Name()}:{leaf}", and adapter names may themselves
// contain ":" (labeled multi-instance, e.g. "github:mycel:issue-42" → "github:mycel").
// Empty when the key has no ":" separator.
func PlatformOf(channel string) string {
	i := strings.LastIndexByte(channel, ':')
	if i <= 0 {
		return ""
	}
	return channel[:i]
}

// IsCatchAll reports whether channel is the canonical catch-all ("{platform}:*").
func IsCatchAll(channel string) bool {
	platform := PlatformOf(channel)
	return platform != "" && channel == CatchAllChannel(platform)
}

// hasRealGeneralRoom reports platforms where "{adapter}:general" is a real
// chat room (#general), not a catch-all placeholder. Migrating those rows to
// ":*" re-subscribes agents to every channel — the loop users hit when they
// pick Slack #general and a daemon restart rewrites them onto slack:*.
func hasRealGeneralRoom(platform string) bool {
	adapter := platform
	if i := strings.IndexByte(adapter, ':'); i >= 0 {
		adapter = adapter[:i]
	}
	switch adapter {
	case "slack", "discord":
		return true
	default:
		return false
	}
}

// IsLegacyCatchAll reports whether channel is the pre-#3467 catch-all key
// ("{platform}:general"). Slack/Discord #general use the same key for a real
// room, so those are never treated as legacy catch-all (#3467 follow-up).
func IsLegacyCatchAll(channel string) bool {
	platform := PlatformOf(channel)
	if platform == "" || channel != LegacyCatchAllChannel(platform) {
		return false
	}
	if hasRealGeneralRoom(platform) {
		return false
	}
	return true
}

// IsAnyCatchAll reports canonical or legacy catch-all keys. Used by prune
// heuristics while both forms may exist in a database.
func IsAnyCatchAll(channel string) bool {
	return IsCatchAll(channel) || IsLegacyCatchAll(channel)
}

// FindPruneCandidates returns non-catch-all subscriptions that look like
// leftovers from the old catch-all copy behavior (#3463/#3465): same agent
// and mention_only as an existing catch-all row ("{platform}:*" or legacy
// "{platform}:general").
//
// Muted rows are skipped — they are intentional mute markers (#3466), not
// auto-copied delivery subscriptions. There is no provenance column, so this
// heuristic can also match deliberate per-channel subscriptions that happen
// to mirror the catch-all; callers must confirm before deleting.
func FindPruneCandidates(subs []Subscription) []Subscription {
	type key struct {
		agent       string
		mentionOnly bool
	}
	catchAll := map[string]map[key]struct{}{} // platform → catch-all agent settings
	for _, sub := range subs {
		if !IsAnyCatchAll(sub.Channel) || sub.Muted {
			continue
		}
		platform := PlatformOf(sub.Channel)
		if catchAll[platform] == nil {
			catchAll[platform] = map[key]struct{}{}
		}
		catchAll[platform][key{agent: sub.Agent, mentionOnly: sub.MentionOnly}] = struct{}{}
	}

	var out []Subscription
	for _, sub := range subs {
		if IsAnyCatchAll(sub.Channel) || sub.Muted {
			continue
		}
		platform := PlatformOf(sub.Channel)
		if platform == "" {
			continue
		}
		// Slack/Discord #general is a real room — never a catch-all copy leftover.
		if hasRealGeneralRoom(platform) && sub.Channel == LegacyCatchAllChannel(platform) {
			continue
		}
		agents, ok := catchAll[platform]
		if !ok {
			continue
		}
		if _, match := agents[key{agent: sub.Agent, mentionOnly: sub.MentionOnly}]; !match {
			continue
		}
		out = append(out, sub)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Channel != out[j].Channel {
			return out[i].Channel < out[j].Channel
		}
		return out[i].Agent < out[j].Agent
	})
	return out
}

// FilterPruneByPlatform keeps candidates whose channel platform matches
// platform. Empty platform returns all candidates unchanged.
func FilterPruneByPlatform(candidates []Subscription, platform string) []Subscription {
	if platform == "" {
		return candidates
	}
	var out []Subscription
	for _, sub := range candidates {
		if PlatformOf(sub.Channel) == platform {
			out = append(out, sub)
		}
	}
	return out
}
