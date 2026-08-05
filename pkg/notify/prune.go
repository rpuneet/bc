package notify

import (
	"sort"
	"strings"
)

// CatchAllChannel returns the catch-all subscription key for a platform
// ("{platform}:general"). Empty platform yields an empty string.
func CatchAllChannel(platform string) string {
	if platform == "" {
		return ""
	}
	return platform + catchAllSuffix
}

// PlatformOf returns the platform prefix of a channel key ("slack:eng" → "slack").
// Empty when the key has no ":" separator.
func PlatformOf(channel string) string {
	i := strings.IndexByte(channel, ':')
	if i <= 0 {
		return ""
	}
	return channel[:i]
}

// IsCatchAll reports whether channel is exactly "{platform}:general".
func IsCatchAll(channel string) bool {
	platform := PlatformOf(channel)
	return platform != "" && channel == CatchAllChannel(platform)
}

// FindPruneCandidates returns non-catch-all subscriptions that look like
// leftovers from the old catch-all copy behavior (#3463/#3465): same agent
// and mention_only as an existing "{platform}:general" row.
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
		if !IsCatchAll(sub.Channel) || sub.Muted {
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
		if IsCatchAll(sub.Channel) || sub.Muted {
			continue
		}
		platform := PlatformOf(sub.Channel)
		if platform == "" {
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
