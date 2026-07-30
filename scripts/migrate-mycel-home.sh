#!/usr/bin/env bash
# migrate-mycel-home.sh — one-time deploy tool that reshapes an existing
# ~/.mycel into the entity-scoped layout:
#
#   ~/.mycel/
#     prefs.json           # from workspaces/<id>/preferences.json
#     mycel.db
#     secrets.vault
#     agents/<name>/       # worktree/ (from worktrees/<name>) + session/ + logs/ + tmp/
#     apps/<name>/         # from gateways/<name> and workspaces/<id>/apps/<name>
#     templates/           # merged with workspaces/<id>/templates
#     logs/                # daemon.log, events.jsonl, workspace logs
#     run/                 # daemon.pid, daemon.addr
#
# This is a deploy tool, NOT runtime code — the daemon has no fallbacks.
# Idempotent: re-running on an already-migrated home is a no-op.
#
# Usage:
#   scripts/migrate-mycel-home.sh [--dry-run] [--home <dir>]

set -euo pipefail

MYCEL_HOME="${MYCEL_HOME:-$HOME/.mycel}"
DRY_RUN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=1; shift ;;
    --home) MYCEL_HOME="$2"; shift 2 ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ ! -d "$MYCEL_HOME" ]]; then
  echo "nothing to migrate: $MYCEL_HOME does not exist"
  exit 0
fi

log() { echo "  $*"; }

run() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "  [dry-run] $*"
  else
    "$@"
  fi
}

# move <src> <dst> — move src to dst unless dst already exists.
move() {
  local src="$1" dst="$2"
  [[ -e "$src" ]] || return 0
  if [[ -e "$dst" ]]; then
    log "skip (exists): $dst"
    return 0
  fi
  run mkdir -p "$(dirname "$dst")"
  run mv "$src" "$dst"
  log "moved: $src -> $dst"
}

echo "migrating $MYCEL_HOME to the entity-scoped layout${DRY_RUN:+}"
[[ "$DRY_RUN" -eq 1 ]] && echo "(dry run — nothing will be changed)"

# ── 1. Base directories ─────────────────────────────────────────────────────
for d in agents apps templates logs run; do
  [[ -d "$MYCEL_HOME/$d" ]] || run mkdir -p "$MYCEL_HOME/$d"
done

# ── 2. prefs.json from workspaces/<id>/preferences.json ─────────────────────
if [[ ! -f "$MYCEL_HOME/prefs.json" ]]; then
  # Pick the most recently modified preferences.json (single existing
  # workspace in practice; warn when several are found).
  prefs_candidates=()
  while IFS= read -r p; do prefs_candidates+=("$p"); done < <(
    ls -t "$MYCEL_HOME"/workspaces/*/preferences.json 2>/dev/null || true
  )
  if [[ ${#prefs_candidates[@]} -gt 0 ]]; then
    if [[ ${#prefs_candidates[@]} -gt 1 ]]; then
      echo "WARNING: ${#prefs_candidates[@]} workspace preferences found; using newest: ${prefs_candidates[0]}" >&2
    fi
    run cp "${prefs_candidates[0]}" "$MYCEL_HOME/prefs.json"
    log "prefs: ${prefs_candidates[0]} -> $MYCEL_HOME/prefs.json"
  fi
else
  log "skip (exists): $MYCEL_HOME/prefs.json"
fi

# ── 3. Worktrees: worktrees/<name> -> agents/<name>/worktree ────────────────
if [[ -d "$MYCEL_HOME/worktrees" ]]; then
  for wt in "$MYCEL_HOME"/worktrees/*/; do
    [[ -d "$wt" ]] || continue
    name="$(basename "$wt")"
    move "$MYCEL_HOME/worktrees/$name" "$MYCEL_HOME/agents/$name/worktree"
  done
  run rmdir "$MYCEL_HOME/worktrees" 2>/dev/null || true
fi

# ── 4. Agent state: claude/, claude.json, session files -> session/ ─────────
for agent in "$MYCEL_HOME"/agents/*/; do
  [[ -d "$agent" ]] || continue
  name="$(basename "$agent")"
  move "$agent/claude"          "$agent/session/claude"
  move "$agent/claude.json"     "$agent/session/claude.json"
  move "$agent/session_id"      "$agent/session/session_id"
  move "$agent/session_history" "$agent/session/session_history"
  for d in session logs tmp; do
    [[ -d "$agent$d" ]] || run mkdir -p "$agent$d"
  done
done

# Nested per-workspace agent state (workspaces/<id>/agents/<name>/...)
for wsdir in "$MYCEL_HOME"/workspaces/*/; do
  [[ -d "$wsdir" ]] || continue
  for agent in "$wsdir"agents/*/; do
    [[ -d "$agent" ]] || continue
    name="$(basename "$agent")"
    move "$agent/claude"          "$MYCEL_HOME/agents/$name/session/claude"
    move "$agent/claude.json"     "$MYCEL_HOME/agents/$name/session/claude.json"
    move "$agent/session_id"      "$MYCEL_HOME/agents/$name/session/session_id"
    move "$agent/session_history" "$MYCEL_HOME/agents/$name/session/session_history"
    move "$agent/logs"            "$MYCEL_HOME/agents/$name/logs"
  done
done

# ── 5. Apps: gateways/<name> and workspaces/<id>/apps/<name> -> apps/ ───────
if [[ -d "$MYCEL_HOME/gateways" ]]; then
  for gw in "$MYCEL_HOME"/gateways/*/; do
    [[ -d "$gw" ]] || continue
    move "$MYCEL_HOME/gateways/$(basename "$gw")" "$MYCEL_HOME/apps/$(basename "$gw")"
  done
  run rmdir "$MYCEL_HOME/gateways" 2>/dev/null || true
fi
for wsdir in "$MYCEL_HOME"/workspaces/*/; do
  [[ -d "$wsdir" ]] || continue
  for appdir in "$wsdir"apps/*/; do
    [[ -d "$appdir" ]] || continue
    move "$wsdir"apps/"$(basename "$appdir")" "$MYCEL_HOME/apps/$(basename "$appdir")"
  done
done

# ── 6. Templates: workspaces/<id>/templates/* -> templates/ (no clobber) ────
for wsdir in "$MYCEL_HOME"/workspaces/*/; do
  [[ -d "$wsdir" ]] || continue
  if [[ -d "${wsdir}templates" ]]; then
    for tmpl in "${wsdir}templates"/*; do
      [[ -e "$tmpl" ]] || continue
      move "$tmpl" "$MYCEL_HOME/templates/$(basename "$tmpl")"
    done
  fi
done

# ── 7. Logs and daemon runtime files ────────────────────────────────────────
move "$MYCEL_HOME/daemon.log"  "$MYCEL_HOME/logs/daemon.log"
move "$MYCEL_HOME/daemon.pid"  "$MYCEL_HOME/run/daemon.pid"
move "$MYCEL_HOME/daemon.addr" "$MYCEL_HOME/run/daemon.addr"
for wsdir in "$MYCEL_HOME"/workspaces/*/; do
  [[ -d "$wsdir" ]] || continue
  move "${wsdir}events.jsonl" "$MYCEL_HOME/logs/events.jsonl"
  if [[ -d "${wsdir}logs" ]]; then
    for lf in "${wsdir}logs"/*; do
      [[ -e "$lf" ]] || continue
      move "$lf" "$MYCEL_HOME/logs/$(basename "$lf")"
    done
  fi
done

# ── 8. Retire the workspaces/ subtree ───────────────────────────────────────
if [[ -d "$MYCEL_HOME/workspaces" ]]; then
  leftovers="$(find "$MYCEL_HOME/workspaces" -type f 2>/dev/null | head -20 || true)"
  if [[ -z "$leftovers" ]]; then
    run rm -rf "$MYCEL_HOME/workspaces"
    log "removed empty workspaces/ subtree"
  else
    echo "NOTE: workspaces/ still holds files (left in place for manual review):" >&2
    echo "$leftovers" >&2
  fi
fi

# ── 9. Obsolete ledger ──────────────────────────────────────────────────────
if [[ -f "$MYCEL_HOME/costs.db" ]]; then
  echo "NOTE: $MYCEL_HOME/costs.db is obsolete (costs are computed from provider sessions);"
  echo "      it was left in place — remove it manually when you're confident."
fi

echo "done."
