#!/usr/bin/env bash
# previous-semver-tag_test.sh — assert QA/evidence tags never become "previous".
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$ROOT/scripts/previous-semver-tag.sh"
chmod +x "$SCRIPT"

# Requires a real git checkout with release tags.
cd "$ROOT"

got=$("$SCRIPT" --current v0.4.7)
want=v0.4.6
if [[ "$got" != "$want" ]]; then
  echo "FAIL: previous of v0.4.7 = '$got' (want $want)" >&2
  exit 1
fi

got=$("$SCRIPT" --current v0.4.6)
want=v0.4.5
if [[ "$got" != "$want" ]]; then
  echo "FAIL: previous of v0.4.6 = '$got' (want $want)" >&2
  exit 1
fi

# Non-semver must fail closed.
if "$SCRIPT" --current qa-3624-127bead-evidence >/dev/null 2>&1; then
  echo "FAIL: non-semver current should exit 2" >&2
  exit 1
fi

echo "ok: previous-semver-tag"
