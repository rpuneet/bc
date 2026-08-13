#!/usr/bin/env bash
# previous-semver-tag.sh — resolve the prior vMAJOR.MINOR.PATCH release tag.
#
# GoReleaser's default "previous" tag is the newest reachable tag by date.
# QA/evidence tags like qa-3624-*-evidence sit between releases and shrink
# the published changelog (v0.4.7 only listed 2 commits: qa-…evidence…v0.4.7).
#
# Usage:
#   previous-semver-tag.sh                 # previous of HEAD's exact tag, or newest
#   previous-semver-tag.sh v0.4.7
#   previous-semver-tag.sh --current v0.4.7
#
# Prints the previous plain semver tag to stdout, or nothing if first release.
# Exits 0 when empty; exits 2 on bad args.
set -euo pipefail

current=""
if [[ "${1:-}" == "--current" ]]; then
  current="${2:-}"
  [[ -n "$current" ]] || { echo "usage: $0 [--current] vX.Y.Z" >&2; exit 2; }
elif [[ -n "${1:-}" ]]; then
  current="$1"
fi

plain_re='^v[0-9]+\.[0-9]+\.[0-9]+$'
# Allow optional pre-release suffix on the *current* cut tag.
any_re='^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$'

if [[ -z "$current" ]]; then
  if exact=$(git describe --tags --exact-match HEAD 2>/dev/null); then
    if [[ "$exact" =~ $any_re ]]; then
      current="$exact"
    fi
  fi
  if [[ -z "$current" ]]; then
    current=$(git tag -l 'v*' --sort=-version:refname | grep -E "$plain_re" | head -n1 || true)
  fi
fi

if [[ -z "$current" ]]; then
  exit 0
fi
if [[ ! "$current" =~ $any_re ]]; then
  echo "refusing non-semver current tag: $current (expected vMAJOR.MINOR.PATCH)" >&2
  exit 2
fi

# Compare using the plain core (v0.4.8-rc.1 → v0.4.8) so RCs still span
# from the last shipped patch, never from qa-* evidence tags.
core="${current%%-*}"

prev=""
found=0
while IFS= read -r tag; do
  if [[ $found -eq 1 ]]; then
    prev="$tag"
    break
  fi
  if [[ "$tag" == "$core" ]]; then
    found=1
  fi
done < <(git tag -l 'v*' --sort=-version:refname | grep -E "$plain_re" || true)

# Cutting an RC before the plain tag exists: previous is the newest plain tag.
if [[ $found -eq 0 ]]; then
  prev=$(git tag -l 'v*' --sort=-version:refname | grep -E "$plain_re" | head -n1 || true)
  # If that newest is somehow newer than core (shouldn't happen), still OK —
  # callers only use this for changelog span on a release cut.
fi

printf '%s' "$prev"
