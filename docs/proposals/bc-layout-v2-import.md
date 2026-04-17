# bc Layout v2 — One-Shot Import Tool

**Status**: Design locked
**Author**: feature-dev (import tool)
**Related**: [`bc-layout-v2.md`](./bc-layout-v2.md) — authoritative target
layout and schema reference
**Date**: 2026-04-17

> **Design decisions locked 2026-04-17; this doc now reflects defaults, no
> further decisions required.** The target schema, directory layout, hash
> algorithm, and pointer-file contract are all defined in
> [`bc-layout-v2.md`](./bc-layout-v2.md). This doc covers only the import
> procedure.

## Goal

Produce a single `bc workspace import-v1` command that reads the *current chaotic
on-disk state* (nested `.bc/.bc/bc.db`, scattered sidecar DBs, partial
`~/.bc/workspaces/<id>/bc.db`, trashed pre-M11 snapshots) and emits one clean v2
workspace directory at `~/.bc/w/<name>-<hash>/` matching the target layout.

Explicitly **not** a continuous migration. The tool runs once, writes the target,
archives legacy state, and exits. Future bcd boots read only the v2 tree.

## 1. CLI Surface

```
bc workspace import-v1 [flags]
```

### Flags

| Flag | Default | Purpose |
|---|---|---|
| `--workspace PATH` | `$PWD` | Project dir (one whose legacy `.bc/` must be ported) |
| `--workspace-id ID` | auto-resolved from `workspaces.json` | Override when `workspaces.json` is missing/ambiguous |
| `--target DIR` | `~/.bc/w/<name>-<hash>` | Where the v2 workspace is written. Refuses to overwrite unless `--force` |
| `--dry-run` | false | Read-only. Prints plan + row-count diff table. Writes nothing. |
| `--force` | false | Allow non-empty target dir (implies `rm -rf target/*` with confirmation) |
| `--include-trash` | false | Also consult `~/.bc/workspaces.trash-*/` as a source. Default OFF — the old roles that live only in the trash snapshot (`api_lead`, `ui_lead`, …) are obsolete anyway. Even when ON, channels/messages/reactions/mentions are **never** re-introduced (those tables don't exist in v2). |
| `--archive DIR` | `~/.bc/archive/<ts>/` | Where legacy state is moved after success |
| `--keep-legacy` | false | Skip archive step; leave legacy in place (read-only chmod) |
| `--json` | false | Machine-readable report on stdout |
| `-v/--verbose` | false | Log every table, row count, skipped record |

### What it prints

1. **Preflight block** — bcd status, source inventory (file → row count), target resolution.
2. **Plan block** — per-domain: "source X → target table Y: N rows (M deduped)".
3. **Reconciliation warnings** — every conflict that required a rule (e.g., "zen-zebra worktree missing; stale agent row dropped").
4. **Apply block** (when not `--dry-run`) — progress bar for large tables (`cost_records`).
5. **Post-check table** — source rows vs target rows, per domain, with PASS/FAIL per rule.
6. **Archive summary** — paths moved, total bytes, roll-back command.

## 2. Algorithm

Runs for **one workspace at a time** (the current `$PWD`). Multi-workspace import is
a wrapper loop (out of scope).

### 2.0 Preflight — refuse to run on a live system

1. If `~/.bc/workspaces/<id>/bcd.pid` exists and the process is alive → abort.
2. Probe `http://127.0.0.1:9374/api/health` with 2 s timeout. If 200 → abort.
3. Probe `:8080/health` (legacy daemon port seen in some old configs) → abort.
4. Check `lsof -iTCP:9374 -sTCP:LISTEN` as final sanity.
5. If any `*.db-wal` files are non-zero and the owning db hasn't been checkpointed,
   run `PRAGMA wal_checkpoint(TRUNCATE);` on each source DB before reading.

Failure message: `bcd is running (pid 20522). Run 'bc down' before import.`

### 2.1 Discover legacy source

For the current workspace compute these candidates; each is optional:

| Role | Path | Expected |
|---|---|---|
| **Primary project DB** (nested) | `<ws>/.bc/.bc/bc.db` | costs, events, cron, mcp, notify, tools (legacy "everything" DB) |
| Project agents DB | `<ws>/.bc/bc.db` | roles + empty agents/agent_stats |
| Project agents state | `<ws>/.bc/agents/state.db` | often empty but schema exists |
| Project mcp sidecar | `<ws>/.bc/mcp.db` | `mcp_servers` |
| Project tools sidecar | `<ws>/.bc/tools.db` | `tools` |
| Project daemons sidecar | `<ws>/.bc/daemons.db` | `daemons` |
| Project secrets sidecar | `<ws>/.bc/secrets.db` | `secret_meta`, `secrets` |
| Project cost bak | `<ws>/.bc/.migrated/costs.db.bak` | older `cost_records` snapshot |
| Project agent dirs | `<ws>/.bc/agents/<agent>/` | worktrees + `claude/` + `claude.json` |
| **User-global costs** | `~/.bc/costs.db` (WHERE workspace_id=…) | the canonical post-M11 cost store |
| **User-global secrets** | `~/.bc/secrets.vault` + `~/.bc/secret-key` | encrypted user secrets |
| User-global workspaces registry | `~/.bc/workspaces.json` | workspace ID, name, path |
| User-global v2 workspace DB | `~/.bc/workspaces/<id>/bc.db` | currently *only roles*; small |
| User-global v2 agents | `~/.bc/workspaces/<id>/agents/<agent>/` | post-migration agents (e.g. `noble-tapir`) that must NOT be clobbered |
| Pre-M11 trash (opt-in) | `~/.bc/workspaces.trash-*/<6char>/` | last known full set of channels/messages/agents |

Absence of any of these is non-fatal — log and continue.

**Resolving the workspace ID**: prefer `workspaces.json` match on `path == --workspace`;
fall back to `--workspace-id`; else hash `--workspace` with the same algorithm v2 uses.

### 2.2 Compute target dir

```
name   = basename(workspace) | sanitize
hash   = hex(sha256(abs_workspace_path))[:8]
target = ~/.bc/w/<name>-<hash>/
```

The 8-char hex hash is locked — see
[`bc-layout-v2.md#21-hash-scheme`](./bc-layout-v2.md#21-hash-scheme). Shared
helper (`pkg/workspace.DataDirName`) is called from both bcd and the import
tool so the two always agree.

### 2.3 Create target skeleton

```
~/.bc/w/<name>-<hash8>/
├── bc.db                  # created with full v2 schema (via migrations.Up)
├── secrets.db             # per-workspace encrypted secrets (empty; populated in 2.6)
├── preferences.json       # seeded from user default_runtime (preferences.runtime)
├── bcd.pid                # not created here; bcd writes on first start
├── bcd.log                # not created here; bcd writes on first start
├── logs/                  # empty dir for per-agent logs
├── agents/                # empty, populated in step 2.5
└── import-report.json     # written at end; machine-readable audit
```

`~/.bc/settings.json` is created/merged once (not per-workspace). The tool
appends an entry to `settings.json.workspaces[]` for this import. If a
matching entry already exists, it is updated in place. The tool also writes
the project-root pointer file `<ws>/.bc/data-dir` with the absolute path to
the new data dir.

There is no global `~/.bc/secrets.db` or `~/.bc/secrets.vault` in v2 — secrets
live per-workspace in `<data_dir>/secrets.db`. See
[`bc-layout-v2.md#4-per-workspace-secretsdb-schema`](./bc-layout-v2.md#4-per-workspace-secretsdb-schema).

### 2.4 Data transfer via `ATTACH DATABASE`

For every source → target table pair, use:

```sql
ATTACH DATABASE '<source>' AS src;
INSERT INTO main.<target_table> (<cols>)
SELECT <cols_mapped> FROM src.<source_table>
WHERE <filter>
ON CONFLICT(<pk>) DO NOTHING;
DETACH DATABASE src;
```

Rationale: `ATTACH` keeps writes in a single transaction per table, dedup is free
via `ON CONFLICT DO NOTHING`, and 200k+ `cost_records` rows transfer in seconds
without going through Go memory.

Big-table tuning:

- `PRAGMA journal_mode = WAL;`
- `PRAGMA synchronous = NORMAL;`
- Wrap each domain in `BEGIN IMMEDIATE` / `COMMIT`.
- For `cost_records`, build a dedup key first:
  `CREATE UNIQUE INDEX idx_cost_dedup ON cost_records(agent_id, timestamp, cost_usd, session_id)`
  on the **target** before the bulk insert.

### 2.5 Filesystem copy — agent subtrees

For each agent discovered from the union of:

- `~/.bc/workspaces/<id>/agents/<agent>/` (v2, takes priority)
- `<ws>/.bc/agents/<agent>/` (legacy)

Per agent:

1. Resolve the worktree dir. If it's a symlink, `readlink -f` and verify the target
   exists. **Dangling symlink → log as "stale worktree" and drop the agent row
   from `agents` table** (unless `--rescue-stale` passed). Never silently recreate.
2. Use `rsync -a --delete-excluded --exclude 'node_modules' --exclude '.bc/.bc'
   --exclude '*.db-wal' --exclude '*.db-shm' <src>/ <target>/agents/<name>/`.
3. Copy `claude/` home and `claude.json` verbatim.
4. Rename any legacy path prefix `bc-<ws>-<agent>` → keep as-is (v2 uses same
   naming convention — no change needed).
5. If the agent has a live Claude session log at
   `~/.claude/projects/-Users-…-<agent>-…/*.jsonl`, leave it untouched; the v2
   agent dir references it by path.

### 2.6 Reconciliation rules — one-shot decisions

See §3 for the full authority table. The key decisions baked in:

- **`cost_records`**: user-global `~/.bc/costs.db` WHERE `workspace_id=<id>` is
  authority. Top up from project nested `bc.db` + `.migrated/costs.db.bak` using
  dedup key `(agent_id, timestamp, cost_usd, session_id)`. Overlapping windows
  (Mar 24–Apr 16) are silently deduped. Pre–Mar 24 rows from project DB are
  preserved (user-global didn't have them).
- **`roles`**: v2 workspace `bc.db` wins. The project `.bc/bc.db` role set
  (`base`, `designer`, `feature-dev`, `go-reviewer`, etc.) is **identical** to
  v2 for this workspace — confirmed at discovery. For other workspaces where
  sets truly diverge (e.g. legacy `api_lead`, `ui_lead` in the trash snapshot),
  the import **keeps v2** and logs a warning listing dropped legacy roles. User
  decides after the fact whether to re-add them.
- **`agents`**: union by agent name. If same name exists in both legacy `.bc/agents/`
  and `~/.bc/workspaces/<id>/agents/`, v2 wins (filesystem + DB row). `noble-tapir`
  case — never clobbered.
- **`secrets`**: `~/.bc/secrets.vault` (user-global, encrypted) is authority.
  Project `.bc/secrets.db` has 1 meta row and 0 actual secrets — copied as
  metadata only.

### 2.7 Post-checks

Run after every domain transfer. Emit a comparison table:

```
DOMAIN            SOURCE                   SRC ROWS   TGT ROWS   DELTA    STATUS
cost_records      nested bc.db + usr-glbl  +219050    220198     +1148    OK (dedup)
cost_imports      nested bc.db             2730       2730       0        OK
tools (provider)  tools.db                 10         10         0        OK
tools (mcp)       mcp.db                   4          4          0        OK (transformed)
agents            legacy+v2 union          4          3          -1       OK (zen-zebra stale, silent drop)
roles             v2 + project custom      1          1          0        OK (base seed only)
secrets           vault → per-ws db        N/A        N/A                 OK (re-encrypted)
```

`channels`, `messages`, `reactions`, `mentions`, `channel_members`,
`messages_fts`, `daemons` are intentionally absent — dropped in v2.

Any FAIL (e.g., target row count < source after dedup that shouldn't have
deduped) aborts the archive step and keeps legacy intact.

### 2.8 Dry-run vs apply

**Dry-run** performs steps 2.0–2.7 against a temp DB
(`/tmp/bc-import-<ts>/bc.db`) and temp agent dir (`/tmp/bc-import-<ts>/agents/`),
prints the plan and post-check table, then deletes the temp dir. No touch of
`~/.bc/` or `~/.bc/w/`.

**Apply** does the same but against the real target, then on post-check PASS
moves legacy to archive (§4).

## 3. Domain-by-Domain Authority Table

Ordered by complexity; each row: authority source, reason, conflict rule.

| Domain | Authority | Why | On conflict |
|---|---|---|---|
| `agents` | Union: `~/.bc/workspaces/<id>/agents/` ∪ project `.bc/agents/` | V2-partial holds post-migration agents (e.g. `noble-tapir`); project still has live worktrees for `curious-otter`, `sleek-osprey`, `swift-hawk` | v2-partial wins on same name. Dangling worktree → silent drop + log line. Agent dir basename preserved so the worktree's internal git config stays valid. |
| `agent_stats` | Recompute from events (not copied) | Legacy rows are stale; v2 recomputes on first scan | N/A |
| `channels`, `channel_members`, `messages`, `messages_fts`, `reactions`, `mentions` | **DROPPED** — `pkg/channel` deleted in PR #2946 (2026-04-10) | Replaced by `pkg/notify` gateway. No internal chat in v2. See [`bc-layout-v2.md#38-notify-replaces-channels`](./bc-layout-v2.md#38-notify-replaces-channels) | N/A. `--include-trash` is off by default and does **not** re-introduce these tables. |
| `cost_records` | **`~/.bc/costs.db` WHERE `workspace_id=<id>`** primary; top-up from project `.bc/.bc/bc.db` and `.bc/.migrated/costs.db.bak` | User-global has workspace_id column and is the post-M11 canonical store; project DBs have overlapping pre-M11 rows. Target has no `workspace_id` column (this DB *is* the workspace). | Dedup on `(agent_id, timestamp, cost_usd, session_id)`. Keep earliest `id`. `id` is renumbered on INSERT — accepted. |
| `cost_imports` | Project nested `bc.db` (2,730 rows) + bak (1,814) merged | Tracks ccusage sync watermarks; user-global has no workspace-scoped import log | Dedup on `source_path` (PK). Keep latest `imported_at`. |
| `cost_budgets` | Bak (3 rows) — nested bc.db has 0 | Budgets were configured pre-M11 and never re-saved | Copy all 3. |
| `cron_jobs`, `cron_logs` | Project nested `bc.db` | Zero rows currently; schema copy only | N/A |
| `mcp_servers` | **Transformed into `tools` rows with `type='mcp'`** | v2 schema absorbs MCP into a single `tools` table | Source: `.bc/mcp.db` (4 rows) over nested `bc.db.mcp_servers` (0 rows). Sidecar wins. |
| `tools` | Project `.bc/tools.db` (10 rows) over nested `bc.db.tools` (22 rows) | Nested has duplicate/stale entries; sidecar is the live registry the daemon reads | Sidecar wins. Log any tools present only in nested. Merge with MCP-transformed rows on `name` collision. |
| `daemons` | **DROPPED** — table removed in v2 | Process state lives in `bcd.pid` and `bcd.log`; UI `/logs` page reads the log file directly | N/A. `.bc/daemons.db` is archived, not imported. |
| `state.db` | **DROPPED** | Ephemeral state; recomputed on boot | N/A |
| `events` | Drop rows (schema remains) | Zero rows in nested DB; v2 starts fresh event log | N/A |
| `notify_*` (channels, gateways, messages, subscriptions, delivery_log) | Project nested `bc.db` | Zero rows today; schema must exist in target | N/A |
| `roles` | Union of `~/.bc/workspaces/<id>/bc.db` + project `.bc/bc.db` custom rows | V2 seed is only `base`. Any custom role rows in either source DB are preserved verbatim. | `base` is always overwritten by the v2 seed row. Any other name is imported as-is; on duplicate name, v2-partial wins. |
| `secrets` / `secret_meta` | **`~/.bc/secrets.vault`** (user-global, encrypted) → re-encrypted into **per-workspace `<data_dir>/secrets.db`** with a fresh per-workspace salt | Global secrets store is retired in v2 (see [`bc-layout-v2.md#4`](./bc-layout-v2.md#4-per-workspace-secretsdb-schema)). Project `.bc/secrets.db` had 1 meta row, 0 secrets — effectively empty. | Vault is decrypted using `~/.bc/secret-key`, re-encrypted per workspace, written to `<data_dir>/secrets.db`. Master key file kept. |
| `workspaces.json` | **MERGED** into `~/.bc/settings.json.workspaces[]` | v2 absorbs the registry; old file is archived, not kept | Upsert entry keyed on `path`; set `data_dir` to the new `~/.bc/w/<name>-<hash8>/`. |
| `.migrated-*` markers | **DROPPED** | v2 does no on-boot migration; markers are obsolete | Deleted after successful import. |

## 4. Safety

### 4.1 Pre-import backup

Before writing anything, copy the following to `--archive` dir
(default `~/.bc/archive/import-<YYYYMMDD-HHMMSS>/`):

```
archive/
├── project.bc.tar.gz        # entire <ws>/.bc/ tree
├── user.bc.tar.gz           # ~/.bc/costs.db, ~/.bc/secrets.vault,
│                            # ~/.bc/workspaces.json,
│                            # ~/.bc/workspaces/<id>/
├── sha256sums.txt
└── import-manifest.json     # CLI flags, versions, timestamp
```

Tarballs are made before the first `INSERT` runs; on any failure the tool
refuses to touch legacy and exits non-zero.

### 4.2 Roll back

```
bc workspace import-v1 --rollback ~/.bc/archive/import-20260417-021500/
```

Restores the two tarballs in place, removes the target `~/.bc/w/<name>-<hash>/`,
restores original `~/.bc/workspaces.json`. Idempotent: can be re-run.

### 4.3 Idempotent re-runs

The tool is re-runnable: if the target dir already exists and its
`import-report.json` reports PASS, the tool exits with "already imported, use
`--force` to re-run". With `--force`, target is wiped and re-built from the
*original* legacy (not from the current v2 dir). This guarantees a consistent
import regardless of how many times it's invoked — the legacy state is never
mutated until archive step.

### 4.4 Archive vs delete legacy

Default behaviour after PASS:

1. `mv <ws>/.bc/.bc/` → `<ws>/.bc/.archive-v1-<ts>/nested/`
2. `mv <ws>/.bc/*.db*` → `<ws>/.bc/.archive-v1-<ts>/sidecars/`
3. `mv <ws>/.bc/agents/` → `<ws>/.bc/.archive-v1-<ts>/agents/`
4. `chmod -R a-w <ws>/.bc/.archive-v1-<ts>/`
5. Leave `<ws>/.bc/settings.json` in place but rename to `settings.v1.json`.

The v2 tree is entirely in `~/.bc/w/<name>-<hash>/` — the project `<ws>/.bc/`
dir becomes a **pointer**: `<ws>/.bc/data-dir` (a text file with the absolute
path to the v2 tree). Existing tooling can read this pointer to find the v2
workspace.

**`--keep-legacy`** skips the move; everything stays in place, chmod'd
read-only. Useful when user wants to keep legacy live for a cooldown period
before final deletion.

## 5. Out of Scope

Explicitly **not** handled by this tool:

1. **Continuous sync.** This is one-shot. After apply, legacy is archived and
   the daemon reads only v2.
2. **Schema upgrades within v2.** v2-internal migrations are handled by the
   daemon's normal migration framework, not this tool.
3. **Reverse migration.** No v2 → v1. Only forward and rollback to pre-import
   archive.
4. **Multi-workspace batch.** This tool handles one workspace at a time. A
   shell wrapper (`for ws in $(jq -r '.workspaces[].path' ~/.bc/workspaces.json); do bc workspace import-v1 --workspace "$ws"; done`)
   is sufficient; embedding a batch mode bloats the tool.
5. **Secret re-encryption.** `~/.bc/secrets.vault` is copied byte-for-byte using
   the existing `~/.bc/secret-key`. If the user wants to rotate keys, they do
   it via `bc secrets rekey` after import.
6. **Cost re-import from Claude.** This tool moves existing `cost_records`; it
   does not re-query `ccusage` or Claude logs. The `cost_imports` watermark
   rows are preserved so subsequent `bc cost import` resumes cleanly.
7. **Trash recovery by default.** Pre-M11 snapshots in
   `~/.bc/workspaces.trash-*/` are ignored unless `--include-trash` is passed.
   Even then, only `channels`/`messages`/`channel_members` are copied (not
   agents — those worktrees no longer exist).
8. **Repair of broken agents.** A stale worktree (e.g. `zen-zebra`) is dropped
   with a log line. The tool does not recreate worktrees from branches.
9. **Claude session log linking.** Logs under `~/.claude/projects/…/*.jsonl`
   are left untouched. V2 picks them up by path on first run.
10. **Multi-user workspaces.** Assumes single-user `$HOME`.

## 6. Testing Plan

### 6.1 Unit tests (Go)

- `TestImportResolveTarget`: given `--workspace /Users/puneetrai/Projects/bc`
  and `workspaces.json` entry, resolves to `~/.bc/w/bc-<hash>/`.
- `TestImportDedupCostRecords`: seed in-memory DBs with overlapping rows,
  assert dedup key collapses them.
- `TestImportDanglingWorktree`: fake agent dir with symlink pointing nowhere;
  assert row dropped + warning emitted.
- `TestImportIdempotent`: run twice, assert second run no-ops.
- `TestImportPreflightBcdUp`: fake PID file + listening socket; assert abort.

### 6.2 Integration test

Fixture: a minimal `.bc/` tree with 3 agents, 100 cost records, 4 mcp servers.
Run `bc workspace import-v1 --dry-run` then apply. Assert:

- Post-check table all PASS.
- `~/.bc/w/testws-<hash>/bc.db` contains expected row counts.
- `~/.bc/w/testws-<hash>/agents/` contains 3 dirs.
- Archive tarball exists and round-trips via `--rollback`.

### 6.3 Live dogfood run on the bc workspace itself

Steps, in order:

1. `bc down` — ensure bcd is stopped.
2. `cp -a ~/.bc ~/.bc.before-import` (external backup — belt & braces).
3. `cp -a /Users/puneetrai/Projects/bc/.bc /tmp/bc-dot-bc-before-import` (external backup).
4. `bc workspace import-v1 --dry-run --verbose > /tmp/dry-run.log 2>&1`.
5. Inspect `/tmp/dry-run.log` for the post-check table. Specifically verify:
   - `cost_records`: ~219k (nested) + 23k (user-global this ws) + 266k (bak)
     → unique count after dedup. Expected net: ≤ 300k.
   - `tools` (type=provider): 10 from sidecar.
   - `tools` (type=mcp): 4 transformed from `mcp.db`.
   - `agents`: 4 (curious-otter, sleek-osprey, swift-hawk, noble-tapir) — no zen-zebra.
   - `roles`: 1 (`base`) plus any user-created custom roles — no `api_lead`, `ui_lead` etc.
   - `channels` / `messages`: tables absent in target schema regardless of
     `--include-trash`.
6. `bc workspace import-v1 --apply --verbose`.
7. `bc up` — start v2 daemon.
8. `bc agents` — confirm all 4 agents listed, zen-zebra absent.
9. `bc cost total` — confirm total matches expected (within ±1% of manual
   sum of three source DBs).
10. Confirm `<ws>/.bc/data-dir` pointer file exists and contains the absolute
    path to `~/.bc/w/bc-<hash8>/`.
11. Browse to `http://localhost:9374` — confirm TUI & web load, dark/light
    themes, `/logs` page renders `bcd.log`.
12. `bc workspace import-v1 --rollback ~/.bc/archive/import-*/` — test
    rollback on a sacrificial copy (do NOT rollback the real one if steps
    8–11 pass).

### 6.4 Acceptance criteria

- Dry-run on the live bc workspace completes in under 60 s.
- Apply completes in under 5 min for 300k cost rows + 3 agent worktrees.
- Post-check PASS in all domains.
- `bc up` boots cleanly on the new v2 tree.
- Rollback restores exact byte-equal state (verified via
  `sha256sum` comparison of archived tarballs vs restored files).

## 7. Locked Defaults

All previously open decisions are locked as of 2026-04-17. Summary of the
defaults baked into this tool (cross-reference
[`bc-layout-v2.md`](./bc-layout-v2.md) for full target schema context):

| # | Decision | Locked value |
|---|---|---|
| 1 | **Target directory** | `~/.bc/w/<name>-<hash8>/` — *not* the legacy `~/.bc/workspaces/<id>/`. The existing v2-partial at `~/.bc/workspaces/<id>/` is imported into the new location and then archived. |
| 2 | **Hash algorithm** | `hex(sha256(abs_project_path))[:8]`. 8 hex chars. Shared helper (`pkg/workspace.DataDirName`) invoked by both bcd and this tool; they will never disagree. |
| 3 | **Pointer file** | `<project>/.bc/data-dir` — one-line text file containing the absolute path to the data dir. This is the contract bcd uses to resolve the workspace from a project directory. |
| 4 | **Registry location** | Inside `~/.bc/settings.json.workspaces[]`. No separate `workspaces.json`. Old file is archived. |
| 5 | **Legacy fate** | Move to `<project>/.bc.archive-v1-<timestamp>/` on PASS. Kept for a week; user deletes manually. `--keep-legacy` leaves originals in place, chmod read-only. |
| 6 | **`--include-trash` default** | **OFF**. Trash-only roles (`api_lead`, `ui_lead`, …) are obsolete in v2's role model. Even when ON, no channels/messages/reactions/mentions are restored — those tables don't exist in v2. |
| 7 | **Dangling worktree** | Silent drop + log line. No `--drop-stale` flag; `--dry-run` surfaces the decision beforehand. |
| 8 | **Batch mode** | One workspace per invocation. Shell loop for many. Keeps failure blast radius small. |
| 9 | **Agent dir basename** | Preserved exactly across import so worktree internal git config stays valid. Source path `<ws>/.bc/agents/<name>/` → target `<data_dir>/agents/<name>/`. |
| 10 | **`cost_records.id` renumbering** | Allowed on dedup INSERT. Nothing references cost IDs externally. |
| 11 | **Channels / messages / reactions / mentions** | **Dropped entirely** — `pkg/channel` removed in PR #2946. `pkg/notify` gateway is the replacement (`notify_subscriptions`, `notify_delivery_log`, `notify_gateways`, `notify_messages`, `notify_channels`). |
| 12 | **Global secrets** | `~/.bc/secrets.vault` re-encrypted into per-workspace `<data_dir>/secrets.db` with a fresh salt. Master key at `~/.bc/secret-key` kept. No global secrets DB or vault in v2. |
| 13 | **Global costs** | `~/.bc/costs.db` rows WHERE `workspace_id=<id>` imported into per-workspace `cost_records`. No `workspace_id` column in target (the DB *is* the workspace). `~/.bc/costs.db` archived. |
| 14 | **`daemons` table** | Dropped. bcd state lives in `bcd.pid` + `bcd.log`; UI `/logs` page renders the log file. |
| 15 | **`.migrated-*` markers** | Deleted after successful import. v2 does no on-boot migration. |
| 16 | **Role seed** | Single row `base` after import. Custom role rows from either source DB are preserved; obsolete seeds (`root`, `feature-dev`, `designer`, `go-reviewer`, …) are not re-seeded. |
| 17 | **Per-agent runtime** | `agents.runtime_backend` is per-agent and nullable; NULL inherits `preferences.runtime`. Docker + tmux agents coexist in one workspace. Import preserves the column value from the source DB where present. |
