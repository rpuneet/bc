#!/usr/bin/env bash
# migrate-worktree-layout.sh — move agents from the nested per-workspace
# layout to the flat mycel-home layout.
#
# Old layout (per agent):
#   ~/.mycel/workspaces/<id>/agents/<name>/bc-<repo>-<name>/   worktree
#   ~/.mycel/workspaces/<id>/agents/<name>/claude/             Claude state
#   ~/.mycel/workspaces/<id>/agents/<name>/claude.json         Claude config
#   ~/.mycel/workspaces/<id>/logs/<name>.log                   session log
#
# New layout:
#   ~/.mycel/worktrees/<name>/                                 worktree
#   ~/.mycel/agents/<name>/claude/                             Claude state
#   ~/.mycel/agents/<name>/claude.json                         Claude config
#   ~/.mycel/agents/<name>/logs/<name>.log                     session log
#
# For each agents row in ~/.mycel/mycel.db whose worktree_dir is under
# */workspaces/*/agents/*:
#   - skip if a tmux session mycel-*-<name> is running ("restart required")
#   - move the worktree dir to ~/.mycel/worktrees/<name>
#   - move sibling state (claude/, claude.json, logs) to ~/.mycel/agents/<name>/
#   - git -C <repo> worktree repair <new-path> (repo from the agents row)
#   - UPDATE agents SET worktree_dir/log_file accordingly
#
# Modes:
#   Default        dry run — prints what WOULD happen, touches nothing.
#   --apply        actually migrate.
#
# Usage:
#   scripts/migrate-worktree-layout.sh              # dry run
#   scripts/migrate-worktree-layout.sh --apply      # migrate
#
# Flags:
#   --apply        perform the migration (default is dry run)
#   --home <dir>   mycel home (default: $HOME/.mycel)

set -euo pipefail

MYCEL_HOME="${HOME}/.mycel"
APPLY=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply) APPLY=1; shift ;;
    --home) MYCEL_HOME="$2"; shift 2 ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown flag: $1 (see --help)" >&2; exit 2 ;;
  esac
done

DB="${MYCEL_HOME}/mycel.db"
NEW_WORKTREES="${MYCEL_HOME}/worktrees"
NEW_AGENTS="${MYCEL_HOME}/agents"

if ! command -v sqlite3 >/dev/null 2>&1; then
  echo "error: sqlite3 is required" >&2
  exit 1
fi
if [[ ! -f "$DB" ]]; then
  echo "error: $DB does not exist — nothing to migrate" >&2
  exit 1
fi

run() { # print in dry-run, execute in apply
  if [[ "$APPLY" -eq 1 ]]; then
    "$@"
  else
    echo "  would: $*"
  fi
}

MIGRATED=0
SKIPPED_RUNNING=0
SKIPPED_MISSING=0
FAILED=0

# Rows: name|worktree_dir|repo|log_file for agents still on the nested layout.
while IFS='|' read -r name wt_dir repo log_file; do
  [[ -z "$name" ]] && continue

  echo "== agent: $name"
  echo "  old worktree: $wt_dir"

  # Skip agents with a live tmux session (mycel-*-<name>) — moving a
  # directory out from under a running agent would corrupt its session.
  if command -v tmux >/dev/null 2>&1 && \
     tmux list-sessions -F '#{session_name}' 2>/dev/null | grep -q "^mycel-.*-${name}$"; then
    echo "  SKIP: tmux session running — restart required (stop the agent, then re-run)"
    SKIPPED_RUNNING=$((SKIPPED_RUNNING + 1))
    continue
  fi

  if [[ ! -d "$wt_dir" ]]; then
    echo "  SKIP: worktree dir missing on disk (will be recreated on next start)"
    SKIPPED_MISSING=$((SKIPPED_MISSING + 1))
    continue
  fi

  old_agent_dir="$(dirname "$wt_dir")"           # .../workspaces/<id>/agents/<name>
  old_state_dir="$(dirname "$(dirname "$old_agent_dir")")"  # .../workspaces/<id>
  new_wt="${NEW_WORKTREES}/${name}"
  new_agent_dir="${NEW_AGENTS}/${name}"

  if [[ -e "$new_wt" ]]; then
    echo "  SKIP: $new_wt already exists — resolve manually"
    FAILED=$((FAILED + 1))
    continue
  fi

  # 1. Move the worktree.
  run mkdir -p "$NEW_WORKTREES"
  run mv "$wt_dir" "$new_wt"

  # 2. Move sibling state: claude/, claude.json, anything else in the
  #    old per-agent dir except the (already moved) worktree.
  run mkdir -p "$new_agent_dir"
  if [[ -d "$old_agent_dir" ]]; then
    while IFS= read -r entry; do
      base="$(basename "$entry")"
      [[ "$entry" == "$wt_dir" ]] && continue
      run mv "$entry" "${new_agent_dir}/${base}"
    done < <(find "$old_agent_dir" -mindepth 1 -maxdepth 1 ! -path "$wt_dir" 2>/dev/null)
    run rmdir "$old_agent_dir" 2>/dev/null || true
  fi

  # 3. Move the session log into the agent's logs dir.
  new_log_file=""
  if [[ -n "$log_file" && -f "$log_file" ]]; then
    new_log_file="${new_agent_dir}/logs/${name}.log"
    run mkdir -p "${new_agent_dir}/logs"
    run mv "$log_file" "$new_log_file"
  fi

  # 4. Repair git worktree metadata from the agent's repo.
  if [[ -n "$repo" && -e "$repo/.git" ]]; then
    run git -C "$repo" worktree repair "$new_wt"
  else
    echo "  warn: repo '$repo' not found — skipping git worktree repair"
  fi

  # 5. Update the DB row.
  esc_name="${name//\'/\'\'}"
  sql="UPDATE agents SET worktree_dir = '${new_wt//\'/\'\'}'"
  if [[ -n "$new_log_file" ]]; then
    sql+=", log_file = '${new_log_file//\'/\'\'}'"
  fi
  sql+=" WHERE name = '${esc_name}';"
  if [[ "$APPLY" -eq 1 ]]; then
    sqlite3 "$DB" "$sql"
  else
    echo "  would: sqlite3 $DB \"$sql\""
  fi

  echo "  new worktree: $new_wt"
  echo "  new state:    $new_agent_dir"
  MIGRATED=$((MIGRATED + 1))
done < <(sqlite3 -separator '|' "$DB" \
  "SELECT name, worktree_dir, COALESCE(repo, ''), COALESCE(log_file, '')
     FROM agents
    WHERE deleted_at IS NULL
      AND worktree_dir LIKE '%/workspaces/%/agents/%';")

echo
echo "== summary =="
[[ "$APPLY" -eq 0 ]] && echo "-- DRY RUN (pass --apply to migrate) --"
echo "migrated:          $MIGRATED"
echo "skipped (running): $SKIPPED_RUNNING"
echo "skipped (missing): $SKIPPED_MISSING"
echo "needs attention:   $FAILED"
