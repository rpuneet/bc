# PR #3003 follow-ups

Tracker for everything PR #3003 doesn't ship that we know we need. Not a commitment — every item should be filed as its own GitHub issue before work starts.

Split into three buckets: **awaits decision** (needs Puneet to choose), **deferred to follow-up PRs** (scope set, just not in this PR), and **candidate improvements** (nice-to-have, no owner).

## 1. Awaits decision

### 1.1 Channels archive (PR #2946 data)

The old `pkg/channel` tables (`channels`, `messages`, `messages_fts`, `reactions`, `mentions`) were deleted in PR #2946 in favor of `pkg/notify` (gateway dispatch). 725 historical messages survive in `workspaces.trash-1776351218/13c6e9/bc.db` but are unreadable under the new schema.

Options:

| | Option | Cost | Downside |
|---|---|---|---|
| a | Accept loss as designed | 0 | Conversations pre-channels-revamp are gone |
| b | Build read-only `/api/archive/channels` viewer backed by the trash DB | ~1 PR | Adds a second data path; has to stay maintained |
| c | Revert PR #2946 | Big — pkg/channel restoration | Undoes a month of work |

**Default if no decision:** (a). 725 rows is small; conversations tend to be time-bounded.

### 1.2 16 historical agent rows

During the 2026-04-17 cost recovery, curious-otter inserted 16 historical agent rows into `bc/.bc/agents` table (state=stopped, tool=claude, runtime_backend=tmux) reconstructed from the cost ledger. Puneet flagged that the initial re-attribution was wrong by agent-name; prefix-based fix landed. Open question: should the row **insertion itself** be reverted?

Trade-off: keeping them means the UI shows historical agents that can't be interacted with. Reverting means the `/costs` view still shows those names (they exist in cost_records), just with no corresponding entry in `agents`.

**Default if no decision:** keep. They're harmless listing-only; no daemon code path spawns them.

### 1.3 Legacy `.bc/roles/*.md` on v2 import

Bc-layout-v2 locked "single `base` template; users author their own markdown." Legacy workspaces have role-specific markdown files in `<project>/.bc/roles/` (root.md, engineer.md, etc.). The v2 import tool has three options:

| | Option | Downside |
|---|---|---|
| a | Copy each legacy role to `~/.bc/templates/<role>.md` | Proliferates templates across users |
| b | Always reset to the shipped `base` template; back up legacy to `<project>/.bc/roles.backup/` | Loses any user-edited prompts |
| c | Prompt the user interactively during import | Import stops being a one-shot `--dry-run` default |

**Default if no decision:** (b) with back-up. Matches the locked "single base template" decision.

### 1.4 Global 204k cost rows on v2 drop

Bc-layout-v2 drops the global `~/.bc/costs.db`. On the bc machine there are ~208k rows in it (169k bc + 30k trade + 8k kognivida). Where do they go?

| | Option | Cost |
|---|---|---|
| a | Fold into each workspace's `bc.db` during import, dedupe against existing per-ws rows | Moderate import complexity |
| b | Accept loss (per-ws DBs have their own cost history, ~140k rows between all three) | 0 |
| c | Export to a `~/.bc/legacy-costs.sqlite` archive separate from the new layout | Low; keeps data accessible for offline analysis without re-import |

**Default if no decision:** (c). Cheapest, preserves data, clean separation.

## 2. Deferred to follow-up PRs

All of these are already scoped in proposals/ADRs; they just don't ship with PR #3003.

### 2.1 Bc-layout-v2 physical migration

`docs/proposals/bc-layout-v2.md` + `docs/proposals/bc-layout-v2-import.md`. Covers:

- `~/.bc/w/<name>-<hash8>/` directory scheme (replaces `~/.bc/workspaces/<12-char-id>/`)
- `~/.bc/settings.json` absorbs the registry
- Single `bc.db` per workspace (drops sidecar `state.db`, `cron.db`, etc.)
- `<project>/.bc/data-dir` pointer file
- `bc workspace import-v1` transformer

Filed as its own PR after #3003 lands and its migration-removal has had a couple of weeks to prove quiet.

### 2.2 Agent-create API + CLI rework

Issue #2999 includes "remove `--role`, introduce `--template <tmpl>` and `--copy <agent>`." Roles table gets deleted; single `base` template seeds. Affects:

- `pkg/agent.Service.Create` signature
- `POST /api/agents` body shape
- `bc agent create` flags
- `web/src/components/CreateAgentModal` (already has template + fork — just needs the `--role` field removed)
- Roles table migration down (or just `DROP TABLE`)

Depends on 1.3 above being decided.

### 2.3 `bc tunnel` MVP

`docs/proposals/bc-tunnel.md`. Phased implementation — can ship phase 1 (CLI + WSS client against a local test relay) as its own PR without phase 2 (actual relay deployment).

### 2.4 CLI migration phases

ADR 0001 §Migration plan. Each "family" of subcommands (agent, cost, template, secret, mcp, workspace, code, stats) is its own PR that replaces direct-`pkg/` calls with HTTP client calls.

### 2.5 `bc up/down/restart` → `~/.bc/settings.json`

Phase 1 of ADR 0001. Small, isolated. Makes the rest of the CLI work meaningful.

### 2.6 React TUI bundle retirement

After bubbletea parity per ADR 0001 §3. Probably 1-2 months after the ADR itself lands.

## 3. Candidate improvements (no owner)

### 3.1 Code search: context-line pagination

`GET /api/code/search` returns one line of context before/after each match. Clients currently need to fetch the full file to show a bigger peek. Consider `&context=<n>` param.

### 3.2 MCP compat: 308 variant with opt-in header

The current `/_mcp/<agent>/*` shim rewrites internally. For clients that set `X-Prefer-Redirect: 308`, emit an actual 308 so they learn the canonical URL and stop hitting the legacy path.

### 3.3 Daemon.addr: PID in the file

`~/.bc/daemon.addr` currently holds just `http://host:port`. Adding `pid=<n>\n` on a second line would let `bc down` match the file's expectations without reading `daemon.pid` separately.

### 3.4 LegacyUIScope: warn on no-active-ws

When a legacy path is hit but no active workspace is registered, the middleware silently passes through. Log a warn with the path so operators notice stale bookmarks from a machine that just went through a registry wipe.

### 3.5 Code search: binary-file detection

`rg --max-filesize 10M` bounds by size but binary files within that budget still get scanned. Add `--binary --with-filename` or `rg --text=false` to skip them cleanly.

## 4. Meta

- **Any of §3's items becoming blocking?** Promote to §1 or §2 with justification.
- **Anything in §1 that never gets decided?** Default listed after each option takes over.
- **New follow-ups discovered during review?** Append to §3 with a file:line reference.

Filed as `docs/proposals/pr3003-followups.md` rather than GitHub issues to keep the body of this PR small. File individual issues when work starts.
