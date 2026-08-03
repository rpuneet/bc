#!/usr/bin/env sh
# version.sh — print the build version for this checkout.
#
# There is one version format: semver, derived from git tags. A release build
# and a source build of the same tree therefore report the same shape, and
# anything that compares versions (the About page's "update available" check,
# npm, the Homebrew tap) can compare them without special cases.
#
#   on a tag, clean      0.4.4
#   after a tag          0.4.5-dev.12.g1a2b3c4
#   uncommitted changes   ...-dev.12.g1a2b3c4.dirty
#   no tags reachable    dev
#
# Off a tag the *patch is incremented* and the distance recorded as a
# prerelease. That ordering is the point: semver ranks a prerelease below the
# release it names, so a literal `git describe` string like "0.4.4-12-g1a2b3c4"
# would sort *below* 0.4.4 and make a build that is twelve commits newer look
# older. Incrementing first makes it sort above 0.4.4 and below 0.4.5, which is
# where it actually belongs. This is the same convention goreleaser uses for
# snapshots (`version_template: {{ incpatch .Version }}-next`).
set -eu

# Not a git checkout (release tarball, vendored source): nothing to derive from.
if ! git rev-parse --git-dir >/dev/null 2>&1; then
	echo dev
	exit 0
fi

# Tracked modifications only, matching `git describe --dirty`. Untracked files
# are not a different build of the tree, so they must not change the version.
dirty=''
if ! git diff --quiet HEAD 2>/dev/null; then
	dirty='.dirty'
fi

# Exactly on a tag with nothing modified: this *is* that release.
if [ -z "$dirty" ]; then
	if tag=$(git describe --tags --exact-match HEAD 2>/dev/null); then
		echo "${tag#v}"
		exit 0
	fi
fi

# --long forces the "<tag>-<distance>-g<sha>" form even when HEAD is the tag,
# so a dirty tagged build still gets a distinguishable prerelease version.
if ! desc=$(git describe --tags --long --abbrev=7 HEAD 2>/dev/null); then
	# Shallow clone or a fork with no tags fetched — CI checks out at depth 1.
	echo dev
	exit 0
fi

sha=${desc##*-}          # g1a2b3c4
rest=${desc%-*}          # v0.4.4-12
distance=${rest##*-}     # 12
base=${rest%-*}          # v0.4.4
base=${base#v}
core=${base%%-*}         # drop any prerelease on the tag itself (v0.1.0-alpha)

# An unparseable tag still has to yield valid semver, or the consumers this
# script exists to unify would need the special case back.
next=$(printf '%s' "$core" | awk -F. 'NF==3 && $1 ~ /^[0-9]+$/ && $2 ~ /^[0-9]+$/ && $3 ~ /^[0-9]+$/ { printf "%d.%d.%d", $1, $2, $3 + 1 }')
if [ -z "$next" ]; then
	next=0.0.0
fi

echo "${next}-dev.${distance}.${sha}${dirty}"
