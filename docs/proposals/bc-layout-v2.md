# bc Layout v2 — Target Directory and Schema

**Status**: Locked design (2026-04-17)
**Author**: Root
**Related**: [`bc-layout-v2-import.md`](./bc-layout-v2-import.md) — one-shot import tool that produces this layout
**Date**: 2026-04-17
**Scope**: Defines the canonical on-disk layout and schemas that bcd v2 reads and
writes. All other proposals that touch persistence must reference this document
rather than redefine tables locally.

Design decisions locked 2026-04-17; this is the authoritative reference for v2
on-disk state. No further structural decisions are required.

---

## 1. Problem Statement

The current on-disk layout has accreted across M7–M11 and is now chaotic. A
single live workspace (the `bc` repo itself) holds state in **eleven** SQLite
files plus three side JSON files, with meaningful rows in four of them.
Concrete inventory at the time of writing:

| File | Tables with rows | Rows |
|---|---|---|
| `<ws>/.bc/.bc/bc.db` (nested) | `cost_records` | 219,050 |
| `<ws>/.bc/.bc/bc.db` | `cost_imports` | 2,730 |
| `<ws>/.bc/.bc/bc.db` | `tools` | 22 |
| `<ws>/.bc/bc.db` | `roles` | 8 |
| `<ws>/.bc/mcp.db` | `mcp_servers` | 4 |
| `<ws>/.bc/tools.db` | `tools` | 10 |
| `<ws>/.bc/secrets.db` | `secret_meta` | 1 |
| `<ws>/.bc/daemons.db` | `daemons` | 1 |
| `~/.bc/costs.db` | `cost_records` (all workspaces) | 205,783 |
| `~/.bc/secrets.vault` | encrypted user secrets | n/a |
| `~/.bc/workspaces.json` | registry | 7 |
| `~/.bc/workspaces/<id>/bc.db` (v2 partial) | `roles`, `agents` | ~10 |

Additional debris: `~/.bc/workspaces/` contains **1,100** id-directories (most
obsolete stubs), `.migrated-*` marker files, per-project `state.db`, and three
user-global `workspaces.trash-*/` dumps.

Consequences:

- Every read path has to consult two or three DBs and pick a winner.
- The cost store lives in both a per-project *and* a user-global DB with
  overlapping rows.
- MCP servers live in **three** places (`mcp.db`, `tools.db`, `bc.db`) with no
  canonical source.
- Channels/messages/reactions code was deleted in PR #2946 but the tables and
  the old trash snapshots still exist on disk.
- Migration markers (`.migrated-*`) gate startup behaviour and are fragile.

Layout v2 collapses this to one workspace directory containing one primary
database, one secrets database, one preferences file, and a flat log/pid pair.

---

## 2. Target Layout

```
~/.bc/
├── settings.json            # user prefs + workspace registry (no workspaces.json)
├── templates/               # shared markdown agent-prompt templates
├── tui-unread.json          # small JSON consumed by the TUI
└── w/
    └── <name>-<hash8>/      # e.g. bc-a3f7c291/
        ├── bc.db            # ONE sqlite — all workspace state
        ├── secrets.db       # per-workspace encrypted secrets
        ├── preferences.json # workspace config incl. runtime = tmux | docker
        ├── bcd.pid          # flat pid file
        ├── bcd.log          # flat log file (served by /logs UI page)
        ├── logs/
        │   └── <agent>.log  # per-agent log (rotated)
        └── agents/
            └── <name>/
                ├── worktree/       # real git worktree dir (NOT a symlink)
                ├── claude/         # copy of ~/.claude for this agent
                └── claude.json     # per-agent claude config
```

### 2.1 Hash Scheme

```
hash = hex(sha256(abs_project_path))[:8]
```

First 8 hex characters of the SHA-256 over the absolute project path. Example:
`/Users/puneetrai/Projects/bc` → `a3f7c291` → directory `bc-a3f7c291`.

The same helper (`pkg/workspace.DataDirName`) is called from both bcd and the
import tool. Hash length is 8; collisions across ~65k workspaces are ignored —
far beyond any realistic user.

### 2.2 Pointer File

Each project root keeps a one-line text file:

```
<project>/.bc/data-dir
```

Contents: the absolute path to `~/.bc/w/<name>-<hash8>/`, with a trailing
newline. bcd resolves the workspace by reading this file. No search-up logic,
no registry lookup, no symlinks. `bc init` writes it; `bc workspace import-v1`
writes it.

If the file is missing, bcd refuses to start and prints:

```
no .bc/data-dir pointer in <project>; run 'bc init' or 'bc workspace import-v1'
```

### 2.3 `~/.bc/settings.json`

Absorbs everything that used to live in `~/.bc/workspaces.json`. Shape:

```json
{
  "version": 2,
  "user": {
    "default_runtime": "docker",
    "editor": "code"
  },
  "workspaces": [
    {
      "name": "bc",
      "path": "/Users/puneetrai/Projects/bc",
      "data_dir": "/Users/puneetrai/.bc/w/bc-a3f7c291",
      "hash": "a3f7c291",
      "created_at": "2026-04-17T02:00:00Z"
    }
  ]
}
```

No separate `workspaces.json`. No `.migrated-*` markers.

### 2.4 `preferences.json`

Per-workspace configuration. Example:

```json
{
  "runtime": "docker",
  "default_provider": "claude",
  "web_port": 9374,
  "log_level": "info",
  "features": {
    "notify_gateway": true
  }
}
```

`runtime` is the **default** for new agents. Individual agents can override it
via the `agents.runtime_backend` column (§3.1), so tmux and docker agents can
coexist in the same workspace.

---

## 3. Per-Workspace `bc.db` Schema

`~/.bc/w/<name>-<hash8>/bc.db` is the single source of truth for this
workspace. All tables live here. No `workspace_id` column anywhere — the DB
*is* the workspace.

### 3.1 Agents

```sql
CREATE TABLE agents (
    name            TEXT PRIMARY KEY,
    template        TEXT,                    -- template name at creation time (historical marker)
    prompt          TEXT NOT NULL DEFAULT '', -- rendered prompt (copied from template or another agent)
    mcp_servers     TEXT NOT NULL DEFAULT '[]',
    plugins         TEXT NOT NULL DEFAULT '[]',
    settings        TEXT NOT NULL DEFAULT '{}',
    state           TEXT NOT NULL DEFAULT 'idle',
    tool            TEXT,
    parent_id       TEXT,
    team            TEXT,
    task            TEXT,
    session         TEXT,
    session_id      TEXT,
    worktree_dir    TEXT,
    log_file        TEXT,
    hooked_work     TEXT,
    children        TEXT,
    is_root         INTEGER NOT NULL DEFAULT 0,
    crash_count     INTEGER NOT NULL DEFAULT 0,
    last_crash_time TEXT,
    recovered_from  TEXT,
    runtime_backend TEXT,                    -- 'tmux' | 'docker'; NULL = inherit preferences.runtime
    ttl             INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT,
    started_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    stopped_at      TEXT,
    deleted_at      TEXT
);
CREATE INDEX idx_agents_state    ON agents(state);
CREATE INDEX idx_agents_template ON agents(template);
CREATE INDEX idx_agents_parent   ON agents(parent_id);
```

**No `role` column.** Agents are created from a **template** (file at
`~/.bc/templates/<name>.md` or `~/.bc/w/<ws>/templates/<name>.md`) or
**copied** from another agent. The `template` column is a historical marker
only — after creation the agent owns its `prompt`, `mcp_servers`, `plugins`,
`settings` outright. Editing the template file later does NOT retroactively
mutate existing agents.

`runtime_backend` is per-agent. Default comes from `preferences.json`; an agent
created with `bc agent create <name> --runtime tmux` overrides it for that agent
alone. The old `workspace` column is dropped — this DB *is* the workspace.

**CLI:**

```
bc agent create <name> [--template <tmpl>] [--copy <existing-agent>] [--runtime tmux|docker] [--tool claude|gemini|...]
```

`--template` and `--copy` are mutually exclusive. If neither is passed, the
default template `base` is used.

### 3.2 Agent Stats

```sql
CREATE TABLE agent_stats (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_name     TEXT    NOT NULL,
    collected_at   TEXT    NOT NULL,
    cpu_pct        REAL    NOT NULL DEFAULT 0,
    mem_used_mb    REAL    NOT NULL DEFAULT 0,
    mem_limit_mb   REAL    NOT NULL DEFAULT 0,
    net_rx_mb      REAL    NOT NULL DEFAULT 0,
    net_tx_mb      REAL    NOT NULL DEFAULT 0,
    block_read_mb  REAL    NOT NULL DEFAULT 0,
    block_write_mb REAL    NOT NULL DEFAULT 0
);
CREATE INDEX idx_agent_stats_agent ON agent_stats(agent_name);
CREATE INDEX idx_agent_stats_time  ON agent_stats(collected_at);
```

### 3.3 Templates (no DB table)

**There is no `roles` table and no `templates` table.** Templates are plain
markdown files on disk:

```
~/.bc/templates/<name>.md             # global / user-level templates
~/.bc/w/<ws>/templates/<name>.md      # workspace-scoped overrides
```

Workspace template with the same name wins over the global one.

A template file carries the prompt body plus optional YAML frontmatter for
MCP servers, plugins, settings, and tool default:

```markdown
---
mcp_servers: [github, playwright]
plugins: []
settings: { max_turns: 50 }
tool: claude
---

You are a base agent. Edit this prompt to describe what this agent does.
```

At agent creation time, `bc agent create <name> --template <tmpl>` reads the
file, resolves the template, and writes the rendered values into the
`agents` row (`prompt`, `mcp_servers`, `plugins`, `settings`, `tool`).

**Seeded template set:** exactly one file, `~/.bc/templates/base.md`, with a
plain prompt and no MCPs. Every previously seeded role (`root`, `feature-dev`,
`designer`, `go-reviewer`, `engineer`, `manager`, `api_lead`, `ui_lead`, etc.)
is **deleted**. No equivalent templates ship by default — users author their
own as markdown files.

`--copy <agent-name>` clones another agent's current `prompt`, `mcp_servers`,
`plugins`, `settings`, and `tool` instead of reading a template file.

### 3.4 Events

```sql
CREATE TABLE events (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    type      TEXT NOT NULL,
    agent     TEXT,
    message   TEXT,
    data      TEXT,
    timestamp TEXT NOT NULL
);
CREATE INDEX idx_events_agent     ON events(agent);
CREATE INDEX idx_events_timestamp ON events(timestamp DESC);
```

### 3.5 Cost

```sql
CREATE TABLE cost_records (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id              TEXT NOT NULL,
    model                 TEXT NOT NULL,
    input_tokens          INTEGER NOT NULL DEFAULT 0,
    output_tokens         INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
    total_tokens          INTEGER NOT NULL DEFAULT 0,
    cost_usd              REAL NOT NULL DEFAULT 0,
    session_id            TEXT,
    timestamp             TEXT NOT NULL
);
CREATE INDEX idx_cost_records_agent     ON cost_records(agent_id);
CREATE INDEX idx_cost_records_model     ON cost_records(model);
CREATE INDEX idx_cost_records_timestamp ON cost_records(timestamp DESC);
CREATE INDEX idx_cost_records_session   ON cost_records(session_id);

CREATE TABLE cost_imports (
    source_path  TEXT NOT NULL,
    watermark    TEXT NOT NULL,
    record_count INTEGER NOT NULL DEFAULT 0,
    imported_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (source_path)
);

CREATE TABLE cost_budgets (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    scope      TEXT NOT NULL UNIQUE,
    period     TEXT NOT NULL DEFAULT 'monthly'
                    CHECK (period IN ('daily', 'weekly', 'monthly')),
    limit_usd  REAL NOT NULL DEFAULT 0,
    alert_at   REAL NOT NULL DEFAULT 0.8,
    hard_stop  INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX idx_cost_budgets_scope ON cost_budgets(scope);
```

No `workspace_id` column. No `team_id`. The DB is the workspace; team
membership is resolved via `agents.team`.

### 3.6 Cron

```sql
CREATE TABLE cron_jobs (
    name        TEXT PRIMARY KEY,
    schedule    TEXT NOT NULL,
    agent_name  TEXT,
    prompt      TEXT,
    command     TEXT,
    enabled     INTEGER NOT NULL DEFAULT 1,
    last_run    DATETIME,
    next_run    DATETIME,
    run_count   INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE cron_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    job_name    TEXT NOT NULL REFERENCES cron_jobs(name) ON DELETE CASCADE,
    status      TEXT NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    cost_usd    REAL NOT NULL DEFAULT 0,
    output      TEXT,
    run_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_cron_logs_job ON cron_logs(job_name, run_at DESC);
```

### 3.7 Tools (absorbs MCP servers)

```sql
CREATE TABLE tools (
    name          TEXT PRIMARY KEY,
    type          TEXT NOT NULL DEFAULT 'provider',   -- 'provider' | 'mcp' | 'cli'
    command       TEXT NOT NULL DEFAULT '',
    install_cmd   TEXT,
    upgrade_cmd   TEXT,
    version_cmd   TEXT,
    transport     TEXT DEFAULT '',                    -- for type='mcp': 'stdio' | 'sse'
    url           TEXT,                               -- for type='mcp' + transport='sse'
    args          TEXT DEFAULT '[]',                  -- JSON array
    env           TEXT DEFAULT '{}',                  -- JSON object
    slash_cmds    TEXT,
    mcp_servers   TEXT,
    config        TEXT,
    health_status TEXT DEFAULT 'unknown',
    last_checked  TEXT,
    builtin       BOOLEAN DEFAULT FALSE,
    enabled       BOOLEAN DEFAULT TRUE,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_tools_type    ON tools(type);
CREATE INDEX idx_tools_enabled ON tools(enabled);
```

Old `mcp_servers` rows are imported as `tools` rows with `type='mcp'`. The
`mcp_servers` table is dropped.

### 3.8 Notify (replaces channels)

The old channels/messages/reactions/mentions/FTS system was removed in
PR #2946 (2026-04-10). Its replacement is `pkg/notify`: gateway adapters
(Slack, Telegram, Discord, GitHub) ingest external events; those events are
dispatched to subscribed agents. There is no internal chat.

```sql
CREATE TABLE notify_subscriptions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    channel      TEXT NOT NULL,
    agent        TEXT NOT NULL,
    mention_only INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(channel, agent)
);
CREATE INDEX idx_notify_subs_channel ON notify_subscriptions(channel);
CREATE INDEX idx_notify_subs_agent   ON notify_subscriptions(agent);

CREATE TABLE notify_delivery_log (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    logged_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    channel   TEXT NOT NULL,
    agent     TEXT NOT NULL,
    status    TEXT NOT NULL CHECK(status IN ('delivered', 'failed', 'pending')),
    error     TEXT,
    preview   TEXT
);
CREATE INDEX idx_notify_delivery_channel ON notify_delivery_log(channel, id DESC);

CREATE TABLE notify_gateways (
    name         TEXT PRIMARY KEY,
    enabled      INTEGER NOT NULL DEFAULT 0,
    connected    INTEGER NOT NULL DEFAULT 0,
    last_seen_at TEXT,
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE notify_messages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    channel    TEXT NOT NULL,
    sender     TEXT NOT NULL,
    content    TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX idx_notify_messages_channel ON notify_messages(channel, id DESC);

CREATE TABLE notify_channels (
    bc_channel  TEXT PRIMARY KEY,
    platform    TEXT NOT NULL,
    platform_id TEXT NOT NULL,
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
```

---

## 4. Per-Workspace `secrets.db` Schema

`~/.bc/w/<name>-<hash8>/secrets.db`. Same shape as the existing per-project
secrets DB, but now per-workspace only (no global `~/.bc/secrets.db`, no
`~/.bc/secrets.vault`). Encryption key derivation uses `secret_meta.salt`.

```sql
CREATE TABLE secrets (
    name        TEXT PRIMARY KEY,
    value       TEXT NOT NULL,                 -- AES-GCM ciphertext, base64
    description TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE secret_meta (
    key   TEXT PRIMARY KEY,                    -- 'salt', 'kdf', 'version'
    value TEXT NOT NULL
);
```

Key material: still `~/.bc/secret-key` (single user-wide master key). Salt is
per-workspace so ciphertext from workspace A cannot be decrypted in B even
with a stolen master key.

---

## 5. What Lives Where

| Domain | Storage | Rationale |
|---|---|---|
| User preferences | `~/.bc/settings.json` | Single file, JSON, easily hand-edited |
| Workspace registry | `~/.bc/settings.json.workspaces[]` | Absorbed from old `workspaces.json`; one file to back up |
| Shared templates | `~/.bc/templates/*.md` | Markdown, version-controlled by user |
| TUI unread counts | `~/.bc/tui-unread.json` | Small, high-churn, not worth a DB |
| Workspace config | `<data_dir>/preferences.json` | Plain JSON, hand-editable; default runtime lives here |
| Agents, events, cost, cron, tools, notify | `<data_dir>/bc.db` | One DB per workspace; atomic backup |
| Secrets | `<data_dir>/secrets.db` | Encrypted with per-workspace salt |
| PID file | `<data_dir>/bcd.pid` | Flat file; readable by any tool |
| Daemon log | `<data_dir>/bcd.log` | Flat file; served by UI `/logs` page |
| Agent logs | `<data_dir>/logs/<agent>.log` | Flat files, rotatable, one per agent |
| Agent worktrees | `<data_dir>/agents/<name>/worktree/` | Real directory; git worktree add points here |
| Agent claude home | `<data_dir>/agents/<name>/claude/` | Isolated `~/.claude` copy per agent |
| Agent claude config | `<data_dir>/agents/<name>/claude.json` | Per-agent provider config |
| Project → data_dir link | `<project>/.bc/data-dir` | Single-line text file with absolute path |

---

## 6. Migration Boundaries

One sentence per legacy source:

- `<ws>/.bc/.bc/bc.db` — **imported** into `<data_dir>/bc.db` (`cost_records`,
  `cost_imports`, `events`, `cron_*`, `tools`, `notify_*`).
- `<ws>/.bc/bc.db` — **imported** (agents table only; legacy `roles` table is
  dropped entirely — roles no longer exist as a concept, replaced by
  `~/.bc/templates/*.md` files).
- `<ws>/.bc/mcp.db` — **transformed**: rows become `tools` rows with
  `type='mcp'`.
- `<ws>/.bc/tools.db` — **imported** into `tools`.
- `<ws>/.bc/secrets.db` — **dropped** (was effectively empty: 1 meta row, 0
  secrets).
- `<ws>/.bc/daemons.db` — **dropped** entirely; `daemons` table removed.
- `<ws>/.bc/agents/state.db` — **dropped**; recomputed from events.
- `<ws>/.bc/agents/<name>/` — **moved** into `<data_dir>/agents/<name>/`
  preserving basename; worktree git config remains valid.
- `~/.bc/costs.db` — **imported** WHERE `workspace_id=<id>` into per-workspace
  `cost_records`; global file retired.
- `~/.bc/secrets.vault` + `~/.bc/secret-key` — **transformed**: re-encrypted
  into per-workspace `secrets.db` with a per-workspace salt; master key file
  kept as `~/.bc/secret-key`.
- `~/.bc/workspaces.json` — **absorbed** into `~/.bc/settings.json`; file
  removed.
- `~/.bc/workspaces/<id>/bc.db` — **imported** into new `<data_dir>/bc.db`
  (v2-partial agents not clobbered; roles table dropped).
- `.migrated-*` markers — **deleted** after import; no migration markers in
  v2.
- `~/.bc/workspaces.trash-*/` — **ignored** by default; `--include-trash`
  opt-in.

Legacy project `.bc/` tree is renamed to `.bc.archive-v1-<timestamp>/` on
successful import and kept for one week. User deletes manually after cooldown.

---

## 7. Removed Decisions (now locked)

These were open questions in earlier drafts; all are now locked:

1. **Hash scheme.** `sha256(abs_path)[:8]` — 8 hex chars. Shared helper
   between bcd and the import tool.
2. **Pointer file.** `<project>/.bc/data-dir` — one-line text file containing
   the absolute path to the data dir. No registry search, no symlink.
3. **Workspace registry location.** Inside `~/.bc/settings.json`; no separate
   `workspaces.json`.
4. **Secrets.** Per-workspace `secrets.db` only. No global `~/.bc/secrets.db`,
   no `~/.bc/secrets.vault`. Master key at `~/.bc/secret-key`; per-workspace
   salt in `secret_meta`.
5. **Cost store.** Per-workspace `cost_records` table inside `bc.db`. No
   `workspace_id` column. No global `~/.bc/costs.db`.
6. **Daemons.** Dropped entirely. Process state is `bcd.pid` + `bcd.log`;
   health/logs exposed via the UI `/logs` page. No `daemons` table, no
   `daemons.db`.
7. **Channels.** The `channels`, `channel_members`, `messages`,
   `messages_fts`, `reactions`, `mentions` tables are dropped entirely —
   `pkg/channel` was removed in PR #2946. The replacement is `pkg/notify`
   with the five `notify_*` tables above: gateway adapters ingest from
   external platforms and dispatch to subscribed agents. No internal chat
   system.
8. **Per-agent runtime.** `agents.runtime_backend` is per-agent and nullable.
   NULL means inherit `preferences.runtime`. Docker and tmux agents coexist
   in the same workspace.
9. **Migration markers.** `.migrated-*` files are deleted after import. V2
   does no on-boot migration; fresh install is always clean, import-tool is
   the only transformer.
10. **Roles deleted.** No `roles` table, no `role` column on agents. Agents
    are created from **templates** (markdown files at `~/.bc/templates/*.md`)
    or by **copying** an existing agent (`bc agent create <name> --copy
    <existing>`). CLI flag is `--template`/`--copy`, never `--role`. Default
    template is `base` (one file: `~/.bc/templates/base.md`, plain prompt, no
    MCPs).

---

## 8. Remaining Open Items

Small enough that v2 work can start without them:

1. **Log rotation policy.** `bcd.log` and `logs/<agent>.log` need a size/age
   cap. Proposal: 100 MB ring per file, last 5 rotations kept. Not blocking
   layout — can be added in a follow-up.
2. **`preferences.json` feature flag surface.** The `features` block is
   intentionally free-form for now; as features stabilize, promote individual
   flags to first-class keys.
3. **Backup tool ergonomics.** A single `bc workspace backup` command that
   tars `<data_dir>/` is desirable but out of scope for the layout itself —
   the layout enables it; the command is a follow-up.

Everything else is locked. See
[`bc-layout-v2-import.md`](./bc-layout-v2-import.md) for the one-shot import
tool that produces this layout from the current on-disk chaos.
