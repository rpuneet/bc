package notify

import (
	"sort"
	"strings"
)

// catchAllSuffix is the reserved channel leaf for platform-wide fallback
// subscriptions (#3467). It cannot collide with a real named-room "#general"
// the way the legacy ":general" leaf did.
const catchAllSuffix = ":*"

// legacyCatchAllSuffix is the pre-#3467 catch-all leaf. Still read (and
// migrated to catchAllSuffix) on platforms that used it as a synthetic
// placeholder (gmail/telegram/…). Named-room adapters never used it that
// way after #3467 — see namedRoomAdapter / IsLegacyCatchAll (#3730).
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

// adapterRoot returns the unlabeled adapter name ("slack", "discord", …)
// from a platform/instance prefix that may itself contain ":"
// ("discord:my-server", "github:mycel").
func adapterRoot(platform string) string {
	if i := strings.IndexByte(platform, ':'); i >= 0 {
		return platform[:i]
	}
	return platform
}

// namedRoomAdapter reports gateways whose channel keys are human-visible
// room/channel names. On those adapters the leaf "general" is a real room
// (#general, guild:general, …), never the pre-#3467 catch-all placeholder.
//
// Catch-all is exclusively "{platform}:*" (#3467). Treating named-room
// ":general" as legacy catch-all rewrote deliberate #general subscriptions
// to ":*" on every daemon restart and delivered #social (etc.) to agents
// that only opted into #general (#3730).
func namedRoomAdapter(platform string) bool {
	switch adapterRoot(platform) {
	case "slack", "discord", "mattermost", "irc", "matrix":
		return true
	default:
		return false
	}
}

// IsCatchAll reports whether channel is the canonical catch-all ("{platform}:*").
func IsCatchAll(channel string) bool {
	platform := PlatformOf(channel)
	return platform != "" && channel == CatchAllChannel(platform)
}

// IsLegacyCatchAll reports whether channel is a pre-#3467 *synthetic*
// catch-all key ("{platform}:general"). Named-room adapters (Slack, Discord,
// …) use that same key for a real #general room, so those are never legacy
// catch-all (#3467 / #3730).
func IsLegacyCatchAll(channel string) bool {
	platform := PlatformOf(channel)
	if platform == "" || channel != LegacyCatchAllChannel(platform) {
		return false
	}
	return !namedRoomAdapter(platform)
}

// IsAnyCatchAll reports canonical or legacy (synthetic) catch-all keys.
// Used by prune heuristics while both forms may exist in a database.
func IsAnyCatchAll(channel string) bool {
	return IsCatchAll(channel) || IsLegacyCatchAll(channel)
}

// FindPruneCandidates returns non-catch-all subscriptions that look like
// leftovers from the old catch-all copy behavior (#3463/#3465): same agent
// and mention_only as an existing catch-all row ("{platform}:*" or legacy
// synthetic "{platform}:general").
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
		// Real #general on named-room adapters shares the ":general" leaf with
		// the old catch-all key but is not a copy leftover (#3730).
		if sub.Channel == LegacyCatchAllChannel(platform) && !IsLegacyCatchAll(sub.Channel) {
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
