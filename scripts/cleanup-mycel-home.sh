#!/usr/bin/env bash
# cleanup-mycel-home.sh — one-time removal of accumulated ~/.mycel debris.
#
# What it removes:
#   1. Dead ~/.mycel/workspaces/<id>/ state dirs. Historic bugs hashed every
#      candidate path walked (worktrees, parents, tmp dirs), leaving ~10k
#      orphaned dirs. Only the ids in the keep-list survive (default:
#      13c6e9ca6322 — the /Users/puneetrai/Projects/bc repo, the only state
#      dir whose repo exists and has live agents).
#   2. Registry leftovers at the root: workspaces.json.bak,
#      workspaces.json.polluted-*, workspaces.json.pre-prune-*,
#      workspaces.bak-*, workspaces.trash-* (the registry itself is gone).
#   3. Migration snapshots at the root: costs.db.pre-*, costs.db.bak-*.
#   4. Migration snapshots inside KEPT state dirs: bc.db.pre-* (plus their
#      -shm/-wal sidecars) and preferences.json.bak-*.
#   5. Migration marker files: .migrated-* at the root and in kept dirs
#      (no current code reads them).
#   6. Truncates the unrotated ~/.mycel/daemon.log (~30 MB).
#
# Modes:
#   Default        dry run — prints what WOULD happen, touches nothing.
#   --apply        actually delete (after a tar backup of every path slated
#                  for removal, unless --no-backup).
#
# Usage:
#   scripts/cleanup-mycel-home.sh                # dry run
#   scripts/cleanup-mycel-home.sh --apply        # backup, then delete
#   scripts/cleanup-mycel-home.sh --apply --keep 13c6e9ca6322,deadbeef1234
#
# Flags:
#   --apply              perform deletions (default is dry run)
#   --keep <ids>         comma-separated workspace ids to keep
#                        (default: 13c6e9ca6322)
#   --home <dir>         mycel home (default: $HOME/.mycel)
#   --backup-file <path> tar.gz destination
#                        (default: $HOME/mycel-cleanup-backup-<ts>.tar.gz)
#   --no-backup          skip the tar backup step (not recommended)

set -euo pipefail

MYCEL_HOME="${HOME}/.mycel"
KEEP_IDS="13c6e9ca6322"
APPLY=0
DO_BACKUP=1
BACKUP_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply) APPLY=1; shift ;;
    --keep) KEEP_IDS="$2"; shift 2 ;;
    --home) MYCEL_HOME="$2"; shift 2 ;;
    --backup-file) BACKUP_FILE="$2"; shift 2 ;;
    --no-backup) DO_BACKUP=0; shift ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown flag: $1 (see --help)" >&2; exit 2 ;;
  esac
done

WORKSPACES_DIR="${MYCEL_HOME}/workspaces"
[[ -z "$BACKUP_FILE" ]] && BACKUP_FILE="${HOME}/mycel-cleanup-backup-$(date +%Y%m%d-%H%M%S).tar.gz"

if [[ ! -d "$MYCEL_HOME" ]]; then
  echo "error: $MYCEL_HOME does not exist — nothing to clean" >&2
  exit 1
fi

# Sanity: every keep id must exist, otherwise we are probably on the wrong
# machine (or the id list is stale) and mass deletion would be catastrophic.
IFS=',' read -r -a KEEP_ARR <<<"$KEEP_IDS"
for id in "${KEEP_ARR[@]}"; do
  if [[ ! -d "${WORKSPACES_DIR}/${id}" ]]; then
    echo "error: keep id ${id} not found at ${WORKSPACES_DIR}/${id} — refusing to continue" >&2
    exit 1
  fi
done

is_kept() {
  local id="$1"
  for k in "${KEEP_ARR[@]}"; do [[ "$id" == "$k" ]] && return 0; done
  return 1
}

# ---------------------------------------------------------------- collect
# Every path slated for removal goes into this list (newline-separated).
TARGETS_FILE="$(mktemp)"
trap 'rm -f "$TARGETS_FILE"' EXIT

# 1. Dead workspace state dirs
DEAD_WS_COUNT=0
KEPT_WS_COUNT=0
if [[ -d "$WORKSPACES_DIR" ]]; then
  while IFS= read -r dir; do
    id="$(basename "$dir")"
    if is_kept "$id"; then
      KEPT_WS_COUNT=$((KEPT_WS_COUNT + 1))
    else
      echo "$dir" >>"$TARGETS_FILE"
      DEAD_WS_COUNT=$((DEAD_WS_COUNT + 1))
    fi
  done < <(find "$WORKSPACES_DIR" -mindepth 1 -maxdepth 1 -type d | sort)
fi

# 2–5. File debris (root leftovers, snapshots, markers)
collect_glob() {
  local pattern f
  for pattern in "$@"; do
    # shellcheck disable=SC2086
    for f in $pattern; do
      if [[ -e "$f" ]]; then
        echo "$f" >>"$TARGETS_FILE"
      fi
    done
  done
  return 0
}

collect_glob \
  "${MYCEL_HOME}/workspaces.json.bak" \
  "${MYCEL_HOME}/workspaces.json.polluted-*" \
  "${MYCEL_HOME}/workspaces.json.pre-prune-*" \
  "${MYCEL_HOME}/workspaces.bak-*" \
  "${MYCEL_HOME}/workspaces.trash-*" \
  "${MYCEL_HOME}/costs.db.pre-*" \
  "${MYCEL_HOME}/costs.db.bak-*" \
  "${MYCEL_HOME}/.migrated-*"

for id in "${KEEP_ARR[@]}"; do
  collect_glob \
    "${WORKSPACES_DIR}/${id}/bc.db.pre-*" \
    "${WORKSPACES_DIR}/${id}/preferences.json.bak-*" \
    "${WORKSPACES_DIR}/${id}/.migrated-*"
done

FILE_DEBRIS_COUNT=$(( $(wc -l <"$TARGETS_FILE") - DEAD_WS_COUNT ))

# 6. daemon.log (truncated, not deleted — the daemon may hold it open)
DAEMON_LOG="${MYCEL_HOME}/daemon.log"
DAEMON_LOG_SIZE=0
[[ -f "$DAEMON_LOG" ]] && DAEMON_LOG_SIZE=$(wc -c <"$DAEMON_LOG" | tr -d ' ')

# ---------------------------------------------------------------- report
human() { # bytes → human readable
  awk -v b="$1" 'BEGIN{ split("B KB MB GB TB", u); i=1; while (b>=1024 && i<5) { b/=1024; i++ } printf "%.1f%s", b, u[i] }'
}

TOTAL_BEFORE=$(du -sk "$MYCEL_HOME" | awk '{print $1 * 1024}')
echo "== mycel home cleanup =="
echo "home:               $MYCEL_HOME ($(human "$TOTAL_BEFORE"))"
echo "keep ids:           $KEEP_IDS"
echo "workspace dirs:     $((DEAD_WS_COUNT + KEPT_WS_COUNT)) total → keep $KEPT_WS_COUNT, remove $DEAD_WS_COUNT"
echo "file debris:        $FILE_DEBRIS_COUNT (registry leftovers, db snapshots, markers)"
echo "daemon.log:         $(human "$DAEMON_LOG_SIZE") (will be truncated)"
echo

if [[ "$APPLY" -eq 0 ]]; then
  echo "-- DRY RUN (pass --apply to delete) --"
  echo "file debris slated for removal:"
  grep -v "^${WORKSPACES_DIR}/[0-9a-f]\{12\}$" "$TARGETS_FILE" | sed 's/^/  rm /' || true
  echo "  (+ ${DEAD_WS_COUNT} dead workspace dirs under ${WORKSPACES_DIR}/)"
  echo "backup would be written to: ${BACKUP_FILE}"
  exit 0
fi

# ---------------------------------------------------------------- backup
if [[ "$DO_BACKUP" -eq 1 ]]; then
  echo "backing up $(wc -l <"$TARGETS_FILE" | tr -d ' ') paths + daemon.log to ${BACKUP_FILE} ..."
  BACKUP_LIST="$(mktemp)"
  cat "$TARGETS_FILE" >"$BACKUP_LIST"
  [[ -f "$DAEMON_LOG" ]] && echo "$DAEMON_LOG" >>"$BACKUP_LIST"
  # -T reads the file list; paths are absolute, tar stores them relative to /.
  tar -czf "$BACKUP_FILE" -T "$BACKUP_LIST"
  rm -f "$BACKUP_LIST"
  echo "backup done: $(du -h "$BACKUP_FILE" | awk '{print $1}')"
else
  echo "!! backup skipped (--no-backup)"
fi

# ---------------------------------------------------------------- delete
echo "removing ${DEAD_WS_COUNT} dead workspace dirs and ${FILE_DEBRIS_COUNT} debris files ..."
REMOVED=0
while IFS= read -r target; do
  # Belt-and-braces: only ever delete inside $MYCEL_HOME.
  case "$target" in
    "${MYCEL_HOME}"/*) rm -rf "$target"; REMOVED=$((REMOVED + 1)) ;;
    *) echo "  skip (outside home): $target" >&2 ;;
  esac
done <"$TARGETS_FILE"

if [[ -f "$DAEMON_LOG" ]]; then
  : >"$DAEMON_LOG"
  echo "daemon.log truncated"
fi

# ---------------------------------------------------------------- summary
TOTAL_AFTER=$(du -sk "$MYCEL_HOME" | awk '{print $1 * 1024}')
REMAINING_WS=$(find "$WORKSPACES_DIR" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l | tr -d ' ')
echo
echo "== done =="
echo "removed paths:      $REMOVED"
echo "workspace dirs:     $REMAINING_WS remaining"
echo "home size:          $(human "$TOTAL_BEFORE") → $(human "$TOTAL_AFTER")"
[[ "$DO_BACKUP" -eq 1 ]] && echo "backup:             $BACKUP_FILE"
