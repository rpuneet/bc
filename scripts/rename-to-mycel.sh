#!/usr/bin/env bash
# One-shot ops rename: ~/Projects/bc -> ~/Projects/mycel.
#
# Coordinates everything that holds the old absolute path:
#   1. stops the daemon
#   2. renames the directory
#   3. repairs git worktree links (both directions)
#   4. rewrites agent repo paths in ~/.mycel/mycel.db
#   5. restarts the daemon from the moved checkout
#
# Any agent session (tmux) that was started from the old path keeps its
# inode-based cwd but should be restarted to pick up the new path.
set -euo pipefail

OLD="${1:-$HOME/Projects/bc}"
NEW="${2:-$HOME/Projects/mycel}"
MYCEL_DB="$HOME/.mycel/mycel.db"

[[ -d "$OLD" ]] || { echo "missing: $OLD" >&2; exit 1; }
[[ -e "$NEW" ]] && { echo "exists: $NEW — refusing to clobber" >&2; exit 1; }

BIN="$NEW/.bc/agents/zen-zebra/bc-bc-zen-zebra/bin/mycel"

echo "== stopping daemon"
"$OLD/.bc/agents/zen-zebra/bc-bc-zen-zebra/bin/mycel" down || true
sleep 2

echo "== renaming $OLD -> $NEW"
mv "$OLD" "$NEW"

echo "== repairing git worktree links"
# Main-repo side: fix gitdir pointers for every linked worktree that
# lived under the renamed tree; worktree side fixes itself via repair.
git -C "$NEW" worktree repair || true
while IFS= read -r wt; do
  git -C "$NEW" worktree repair "$wt" 2>/dev/null || true
done < <(git -C "$NEW" worktree list --porcelain | awk '/^worktree /{print $2}')

echo "== rewriting agent repo paths in mycel.db"
sqlite3 "$MYCEL_DB" "UPDATE agents SET workspace = REPLACE(workspace, '$OLD', '$NEW') WHERE workspace LIKE '$OLD%';"
sqlite3 "$MYCEL_DB" "SELECT DISTINCT workspace FROM agents WHERE deleted_at IS NULL OR deleted_at='';"

echo "== restarting daemon"
"$BIN" up -d
sleep 3
curl -s -m 5 http://127.0.0.1:8080/api/health || true
echo
echo "done. Restart agent sessions started from the old path."
