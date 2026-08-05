# REST API Reference

The `mycel` server (started with `mycel up`) exposes an HTTP API used by the web UI, the CLI, and agents.

**Base URL:** `http://127.0.0.1:9374`
**Content-Type:** `application/json`

Source of truth: route registration in `server/server.go` and the handlers in `server/handlers/`.

## Conventions

### Authentication

When the server is started with an API key configured, every request must carry it as a Bearer token:

```
Authorization: Bearer <key>
```

The `X-API-Key: <key>` header is accepted as an alternative. Without a configured key (the zero-config localhost default), authentication is disabled. Exempt paths that never require auth: `/health`, `/api/health`, `/healthz`, and everything under `/_mcp/`.

### Rate limiting and body size

- Global token-bucket rate limit: **100 requests/second with a burst of 200**. Exceeding it returns `429` with a `Retry-After: 1` header.
- Request bodies are capped at **1 MB** (file uploads use their own multipart limit).

### Single-tenant server

Each daemon instance is single-tenant. All resources live at flat `/api/<resource>` paths — there is no per-request scoping. The former multi-tenant surfaces (`/api/workspaces...`, scoped headers and query parameters) are gone and return `404`.

### Pagination

List endpoints that support pagination accept `?limit=` (default 50, max 1000) and `?offset=` (default 0).

### Errors

Errors are JSON objects: `{"error": "<message>"}` with an appropriate status code (`400`, `404`, `405`, `409`, `429`, `500`, `503`). Internal errors return the generic message `internal server error`; details are only logged server-side.

---

## Health

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe. Returns `{"status":"ok","addr":...,"commit":...,"built_at":...}`. |
| GET | `/api/health` | Health with a live DB round-trip (`SELECT 1`). Returns `{"status":"ok","db":"ok","version":...,"commit":...}`; `503` with `"status":"unhealthy"` on DB failure. |
| GET | `/healthz` | Alias of `/api/health`. |
| GET | `/health/ready` | Readiness probe. Returns `{"status":"ok"\|"degraded","checks":{...}}`; `503` when degraded. |

---

## Agents

### Collection and fleet operations

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/agents` | List agents. Query: `includeArchived=1`, `onlyArchived=1`, `include=stats` (adds live resource metrics). |
| POST | `/api/agents` | Create an agent. Body: `{"name","role","tool","runtime_backend","parent","template","avatar":{"variant","color"}}`. Role defaults to `base` when only a template is given. Returns `201` with the agent. |
| GET | `/api/agents/generate-name` | Generate an unused agent name. Returns `{"name": "..."}`. |
| POST | `/api/agents/broadcast` | Send a message to all running agents. Body: `{"message"}`. Returns `{"sent": <n>}`. |
| POST | `/api/agents/send-role` | Send a message to all agents with a role. Body: `{"role","message"}`. |
| POST | `/api/agents/send-pattern` | Send a message to agents whose name matches a pattern. Body: `{"pattern","message"}`. |
| POST | `/api/agents/stop-all` | Stop all agents. Returns `{"stopped": <n>}`. |
| POST | `/api/agents/sync` | Reconcile in-memory agent state with live runtime sessions. Returns `{"synced": <n>, "stopped": <n>, "resumed": <n>}`. |
| GET | `/api/agents/health` | Per-agent health report (`healthy`/`degraded`/`unhealthy`, tmux liveness, state freshness). Query: `timeout=<duration>` (default `60s`), `agent=<name>`. |
| GET | `/api/agents/activity` | Recent activity events across all agents (Live page hydration). Query: `limit` (default 200, max 2000). |

Agent objects include `name`, `role`, `state`, `task`, `tool`, `runtime_backend`, `session`, `session_id`, `parent_id`, `children`, `created_at`/`started_at`/`updated_at`/`stopped_at`/`archived_at`, `repo_root`, `workspace` (the agent's repo path — the field name is a wire-compat holdover), `total_cost_usd`, `total_tokens`, and optional `stats`, `avatar`, `mcp_servers`.

### Bulk operations

All bulk endpoints are `POST` and return `{"results": [{"agent","status":"ok"|"error","error?"}]}`.

| Method | Path | Body |
|--------|------|------|
| POST | `/api/agents/bulk/start` | `{"agents": ["..."]}` |
| POST | `/api/agents/bulk/stop` | `{"agents": ["..."]}` |
| POST | `/api/agents/bulk/delete` | `{"agents": ["..."], "force": bool}` |
| POST | `/api/agents/bulk/message` | `{"agents": ["..."], "message": "..."}` |

### Per-agent

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/agents/{name}` | Get one agent. `404` if unknown. |
| DELETE | `/api/agents/{name}` | Delete an agent. Query: `force=true`. Returns `204`. |
| POST | `/api/agents/{name}/start` | Start the agent. Optional body: `{"runtime","resume_id"}`. |
| POST | `/api/agents/{name}/stop` | Stop the agent. Returns `{"status":"stopped"}`. |
| POST | `/api/agents/{name}/archive` | Archive the agent. Returns `{"status":"archived"}`. |
| POST | `/api/agents/{name}/unarchive` | Unarchive. Returns `{"status":"active"}`. |
| POST | `/api/agents/{name}/send` | Type a message into the agent's session. Body: strictly `{"message"}` — unknown fields and empty messages are rejected with `400`. |
| POST | `/api/agents/{name}/rename` | Body: `{"new_name"}`. Returns `{"status":"renamed","name"}`. |
| POST | `/api/agents/{name}/fork` | Fork the agent into a new one. Body: `{"name"}` (required). Returns `201` with the new agent. |
| POST | `/api/agents/{name}/report` | Agent self-reports state. Body: `{"state","message"}`. |
| POST | `/api/agents/{name}/hook` | Receive a Claude Code hook payload (state routing + event log + SSE fan-out). Body must include a known `event`; the raw body is persisted to the event log. |
| GET | `/api/agents/{name}/activity` | Activity timeline for one agent, derived from the event store. |
| GET | `/api/agents/{name}/events` | **SSE** stream of hook events (`event: hook` frames); replays the last 50 events, then tails. |
| GET | `/api/agents/{name}/stats` | Recent runtime stats samples. Query: `limit` (default 20, max 1000). |
| GET | `/api/agents/{name}/stats-computed` | Activity stats computed from hook events + live sampling (tool breakdown, tokens, cost, disk, CPU/mem). Works without TimescaleDB. |
| GET | `/api/agents/{name}/peek` | Capture recent terminal output. Query: `lines` (default 500, max 10000). Returns `{"output"}`. |
| GET | `/api/agents/{name}/last-terminal` | Last captured terminal output plus `state` and `stopped_at`. Query: `lines`. |
| GET | `/api/agents/{name}/sessions` | List recorded provider sessions for the agent. |
| GET | `/api/agents/{name}/output` | **SSE** stream of terminal output (initial snapshot, then `event: agent.output` frames with `{"output"}` deltas). |
| GET | `/api/agents/{name}/terminal` | **WebSocket** bridge to the agent's session via a PTY. `501` when the terminal handler is unavailable. |
| GET | `/api/agents/{name}/config` | Agent config: `{"worktree_path","system_prompt","runtime_backend","tool","session","mcp_servers"}`. The system prompt is read from the provider's prompt file (`CLAUDE.md`, `GEMINI.md`, `.cursorrules`, ...) in the worktree. |
| PATCH | `/api/agents/{name}/config` | Update the system prompt. Body: `{"system_prompt"}`. Writes the provider prompt file into the worktree. |
| GET | `/api/agents/{name}/mcps` | List MCP servers from the agent worktree's `.mcp.json`. |
| POST | `/api/agents/{name}/mcps` | Add an MCP server. Body: `{"name","url","command","type","env"}`. Returns `201`. |
| DELETE | `/api/agents/{name}/mcps/{mcp}` | Remove an MCP server from `.mcp.json`. Returns `204`. |
| GET | `/api/agents/{name}/env` | Persisted env vars: `[{"key","value"}]`. |
| PUT | `/api/agents/{name}/env` | Replace persisted env vars. Body: `[{"key","value"}]`. |
| GET | `/api/agents/{name}/loop` | Loop config: `{"prompt","enabled"}`. |
| PUT | `/api/agents/{name}/loop` | Set the loop config. When enabled, the server re-sends `prompt` each time the agent's turn ends (Stop / SessionEnd hooks). |

---

## Channels & Notify

Channels are inbound app sources (e.g. `slack:general`). Notification routing is subscription-based.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/channels` | List known app channels (discovered + subscribed) as `{name, description, members, member_count}`. Also served at `/api/apps/channels`. |
| GET | `/api/channels/{name}/history` | Message history from the notify store: `[{id, sender, content, created_at}]`. Query: `limit` (default 50, max 200), `before=<id>`. `/messages` suffix is accepted as an alias. |
| GET | `/api/notify/subscriptions` | List all subscriptions. |
| POST | `/api/notify/subscriptions` | Subscribe an agent to a channel. Body: `{"channel","agent","mention_only"}`. Returns `201`. |
| GET | `/api/notify/subscriptions/{channel}` | List subscribers of a channel. |
| PATCH | `/api/notify/subscriptions/{channel}` | Update a subscription. Body: `{"agent","mention_only"}`. |
| DELETE | `/api/notify/subscriptions/{channel}?agent=<name>` | Unsubscribe an agent. |
| GET | `/api/notify/activity/{channel}` | Delivery log entries for a channel. Query: `limit` (default 50, max 200). |

---

## Apps

Apps are plugin integrations that bridge external platforms (Slack, Telegram, Discord, WhatsApp, webhook adapters, ...) into channels. The catalog is descriptor-driven: 28 built-in plugins self-register from `pkg/app/builtin`.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/apps` | Descriptor catalog (every registered app with auth kind, fields, docs) plus configured instances with status. Secret fields report presence only, never values. |
| POST | `/api/apps/{instance}` | Create or update an app instance (`slack`, `telegram:alerts`). Body: `{"app","enabled","config":{...}}` — fields the descriptor marks secret are split out of `config` into the vault (`app:<instance>:<key>`); the rest lands in `prefs.json` `apps`. |
| DELETE | `/api/apps/{instance}` | Disconnect: removes the instance config, its vault keys, and its state dir. |
| POST | `/api/apps/{instance}/auth` | Start an auth flow — dispatches on the plugin's capability (QR pairing for WhatsApp, OAuth where supported). |
| GET | `/api/apps/{instance}/auth/status` | Poll auth/pairing completion. |
| GET | `/api/apps/{instance}/health` | Live adapter status: `{platform, connected, status, error, last_message_at}`. |
| GET | `/api/apps/{instance}/channels` | Channels discovered for this instance: `[{channel_key, name, platform}]`. |
| GET | `/api/apps/{instance}/channels/{channel}` | Channel detail with its subscriptions. |
| POST | `/api/apps/{instance}/react` | Send a reaction via the adapter (where supported). |
| ANY | `/api/apps/{instance}/api/*` | Proxy to the adapter's own HTTP handler (`501` if the adapter has none, `404` if the adapter isn't registered). |
| GET | `/api/apps/channels` | Unified channel list across instances (also at `/api/channels`). |
| POST | `/api/apps/channels/send` | Send a message into a channel. |
| GET | `/api/apps/channels/{name}` | Message history for a channel. |
| POST | `/api/apps/channels/refresh` | Re-resolve channel display metadata (names, kinds). |
| GET | `/api/notifications/overview` | Connected apps + channels with resolved identities. |

### Inbound webhooks

| Method | Path | Description |
|--------|------|-------------|
| POST | `/hooks/{name}` | Inbound webhook receiver for webhook-type app adapters (e.g. `/hooks/github`, `/hooks/webhook`). One route is mounted per adapter that exposes an HTTP handler. |

---

## Costs

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/costs` | Total cost summary, computed from provider session files. |
| GET | `/api/costs/agents` | Cost summaries grouped by agent. Paginated. |
| GET | `/api/costs/teams` | Cost summaries grouped by team. Paginated. |
| GET | `/api/costs/models` | Cost summaries grouped by model. Paginated. |
| GET | `/api/costs/daily` | Daily cost series. Query: `days` (default 30, max 365). |
| GET | `/api/costs/agent/{name}` | One agent's summary plus a 30-day daily breakdown: `{"summary", "daily"}`. |
| GET | `/api/costs/project` | Cost projection. Query: `lookback_days` (default 30), `project_days` (default 30). |
| POST | `/api/costs/sync` | Force a fresh scan of provider session files (drops the read cache). |
| GET | `/api/costs/budgets` | List all budgets. |
| POST | `/api/costs/budgets` | Set a budget. Body: `{"scope","period":"daily"\|"weekly"\|"monthly","limit_usd","alert_at","hard_stop"}`. |
| GET | `/api/costs/budgets/{scope}` | Budget check status for a scope; `404` when no budget is configured. |
| DELETE | `/api/costs/budgets/{scope}` | Delete a budget. Returns `204`. |
| GET | `/api/global/costs` | Cross-repo cost rollup computed from provider sources. Query: `start=<RFC3339 or YYYY-MM-DD>` (default 30 days ago), `groupBy=repo\|project`. |

---


## Secrets

Secret **values are never returned** — only metadata.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/secrets` | List secret metadata. Paginated. |
| POST | `/api/secrets` | Create/set a secret. Body: `{"name","value","description"}`. Returns `201` with metadata. |
| GET | `/api/secrets/{name}` | Get metadata for one secret. |
| PUT | `/api/secrets/{name}` | Update value/description. Body: `{"value","description"}`. |
| DELETE | `/api/secrets/{name}` | Delete. Returns `204`. |

---

## MCP Servers

MCP server registry (distinct from per-agent `.mcp.json`, see Agents).

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/mcp` | List MCP server configs. Paginated. |
| POST | `/api/mcp` | Add a server. Body is an MCP server config (`name`, `transport`, `command`/`url`, `args`, `env`, `enabled`). Returns `201`. |
| GET | `/api/mcp/{name}` | Get one server config. |
| PATCH | `/api/mcp/{name}` | Update env vars. Body: `{"env": {"KEY":"VALUE"}}` (required). |
| DELETE | `/api/mcp/{name}` | Remove. Returns `204`. |
| POST | `/api/mcp/{name}/enable` | Enable. Returns `{"enabled": true}`. |
| POST | `/api/mcp/{name}/disable` | Disable. Returns `{"enabled": false}`. |

---

## Tools

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/tools` | List tools. Query: repeatable `type=` filter (`cli`, `mcp`, ...). Paginated. |
| POST | `/api/tools` | Add a tool. Returns `201`. |
| GET | `/api/tools/{name}` | Get one tool. |
| PUT | `/api/tools/{name}` | Update a tool (name taken from the URL). |
| DELETE | `/api/tools/{name}` | Delete. Returns `204`. |
| POST | `/api/tools/{name}/enable` | Enable. |
| POST | `/api/tools/{name}/disable` | Disable. |
| GET | `/api/tools/unified` | Merged view of MCP servers + role CLI tools + tool-store entries with status/version: `[{name, type, status, transport?, command?, url?, version?, install_cmd?, upgrade_cmd?, required}]`. |
| POST | `/api/tools/unified/check` | Run health checks across the unified tool set. |

---

## Templates

Agent templates (system prompt + MCPs + policies). Stored under the global `~/.mycel/templates/`, with optional repo-scoped overrides.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/templates` | List templates. |
| POST | `/api/templates` | Create. Body: `{"name" (required), "description", "system_prompt", "system_prompt_file", "mcps", "secrets", "plugins", "context_files", "tool_policies", "max_cost_usd", "stuck_timeout_min"}`. `409` on duplicate name. |
| GET | `/api/templates/{name}` | Get a template including its rendered `system_prompt`. |
| PUT | `/api/templates/{name}` | Update. Omitting `system_prompt` preserves the existing prompt; sending `""` clears it. |
| DELETE | `/api/templates/{name}` | Delete. Returns `204`. |

---

## Providers

AI provider registry (claude, gemini, cursor, ...).

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/providers` | List providers with install status, agent counts, and cost totals. |
| GET | `/api/providers/{name}` | Provider detail: config, agent list, per-model cost breakdown. |
| GET | `/api/providers/{name}/commands` | CLI commands available for the provider. |
| GET | `/api/providers/{name}/mcps` | MCP servers configured for the provider (claude/cursor; empty for others). |
| POST | `/api/providers/{name}/mcps` | Add an MCP server via the provider's config adapter. Body: `{"name","transport","url","command"}`. Returns `201`. |
| POST | `/api/providers/{name}/install` | Returns the install hint command (does not execute it). |
| POST | `/api/providers/{name}/update` | Returns the update hint command. |
| POST | `/api/providers/{name}/check-update` | Version check. Returns `{current_version, latest_version, update_available, update_command}`. |
| PATCH | `/api/providers/{name}/config` | Override the provider's launch command in the global settings. Body: `{"command"}`. |

---

## Roles

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/roles` | All roles, resolved through inheritance (deduplicated by normalized name). |
| POST | `/api/roles` | Create a role. Body includes `name` (required), `description`, `prompt`, `parent_roles`, `mcp_servers`, `secrets`, `plugins`, `cli_tools`, `rules`, `commands`, `skills`, `agents`, lifecycle prompts, `review`. `409` if it exists. |
| GET | `/api/roles/{name}` | Resolved role. |
| PUT | `/api/roles/{name}` | Create/update the role (URL name wins over body name). |
| DELETE | `/api/roles/{name}` | Delete. Returns `204`. |

---

## Settings

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/settings` | Full global configuration document (`prefs.json`). |
| PATCH | `/api/settings` | Partial update. Body is an object whose top-level keys are config sections: `user`, `server`, `runtime`, `providers`, `apps`, `storage`, `logs`, `ui`, `injected_instructions` (`version` is ignored; unknown sections are rejected with `400`). Each provided section **replaces** that section, except `apps`, which is merged per instance key and validated against each app's descriptor (secret-typed fields are rejected — they go through `/api/apps`). The merged config is validated before saving; the response echoes the saved config. |

See [Settings API](api-settings.md) for the configuration schema.

---

## Events, Logs & SSE

### Live event stream

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/events` | **SSE** hub stream for the web UI. Frames are `data: {"type": "<event>", "data": {...}}`. |

Event types published on the hub:

- `connected` — sent once per connection
- `agent.created`, `agent.started`, `agent.stopped`, `agent.deleted`, `agent.renamed`, `agent.forked`, `agent.state_changed` — agent lifecycle
- `agent.hook` — raw hook payloads relayed from `POST /api/agents/{name}/hook`
- `channel.message` — inbound gateway message (`{"channel","message":{"sender","content","type"}}`)
- `gateway.message` — notify-service delivery event

### Event log and history

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/logs` | Read the persistent event log. Query: `tail` (default 100, max 10000). |
| POST | `/api/logs` | Append an event to the log. |
| GET | `/api/logs/{agent}` | Events for one agent. |
| GET | `/api/events/history` | Paginated SSE-event history from the JSONL persistence file. Query: `limit` (default 100), `offset`. Returns `{"events", "total"}`. |
| GET | `/api/tasks/current` | Current task list derived from TaskCreate/TaskUpdate events. |

---

## Stats

All timeseries endpoints accept `from`/`to` (RFC3339, default: last hour) and `interval` (default `5m`).

### Summaries

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/stats/summary` | Fleet overview: agent counts, channel/message totals, cost, roles, tools, uptime. |
| GET | `/api/stats/system` | Host snapshot: hostname, OS/arch, CPU, memory, disk, Go version, goroutines, uptime. |
| GET | `/api/system/info` | Minimal host info: `{hostname, os, arch}`. |

### Agent timeseries

Filters: `agent` (comma-separated), `role`, `tool`, `runtime`.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/agents/stats/latest` | Most recent metric sample per agent. |
| GET | `/api/agents/stats/cpu` | CPU timeseries. |
| GET | `/api/agents/stats/mem` | Memory timeseries. |
| GET | `/api/agents/stats/disk` | Disk I/O timeseries. |
| GET | `/api/agents/stats/net` | Network timeseries. |
| GET | `/api/agents/stats/tokens` | Token usage timeseries. |
| GET | `/api/agents/stats/cost` | Cost timeseries. |
| GET | `/api/agents/stats/summary/{name}` | Combined resource + token + cost summary for one agent. |

### System timeseries

Filter: `system` (comma-separated).

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/system/stats/cpu` | Host CPU timeseries. |
| GET | `/api/system/stats/mem` | Host memory timeseries. |
| GET | `/api/system/stats/disk` | Host disk timeseries. |
| GET | `/api/system/stats/net` | Host network timeseries. |

### Channel timeseries

Filter: `channel` (comma-separated).

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/channels/stats/messages` | Message counts. |
| GET | `/api/channels/stats/members` | Member counts. |
| GET | `/api/channels/stats/reactions` | Reaction counts. |

---

## Repos

Repos the agents can be bound to. The server itself is anchored at one repo.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/repos` | List known repos: `{"repos": [...], "default": "<path>"}`. |

### Discovery & GitHub auth

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/repos/discover/local` | Scan a filesystem root for git repos. Body: `{"root" (required), "depth?"}`. Returns `{"candidates": [...]}` annotated with `already_registered`. |
| POST | `/api/repos/discover/github` | List the authenticated user's GitHub repos. Body: `{"query?"}`. `401` when not authenticated. |
| POST | `/api/repos/clone` | Clone a repo. Body: `{"url","target","name?"}`. |
| GET | `/api/auth/github` | GitHub auth status: `{"connected": bool, "login": "..."}`. |
| POST | `/api/auth/github` | Store a token. Body: `{"token"}` (validated before saving). |
| DELETE | `/api/auth/github` | Remove the stored token. |

---

## Doctor

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/doctor` | Run all health checks. |
| GET | `/api/doctor/{category}` | Run one category; `404` for unknown categories. |

---

## Dependencies

Optional service dependencies managed by the server (e.g. database, code server, browser containers). Mutating endpoints are **loopback-only** unless `MYCEL_REMOTE=1` is set.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deps` | List dependencies with status: `{"deps": [{id, name, description, state, error?, deprecated}]}`. |
| GET | `/api/deps/{id}` | One dependency's detail. |
| GET | `/api/deps/{id}/status` | Same as detail (compatibility route). |
| POST | `/api/deps/{id}/start` | Start (loopback-only; `409` if deprecated). Returns `202`. |
| POST | `/api/deps/{id}/stop` | Stop (loopback-only). Returns `202`. |
| GET | `/api/deps/{id}/logs` | **SSE** stream of log lines (`event: log`). Query: `tail` (default 200, max 2000). |

---

## Files

Attachment upload/download (channel attachments and shared screenshots).

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/files/upload` | Multipart upload. Fields: `file` (required), `channel` (required), `sender` (optional, default `web`). Returns `201` with file metadata. Used by gateways/agents; the web UI currently downloads only. |
| GET | `/api/files/{id}` | Download/serve a stored file (inline, cached for a day). |

---

## Code

Read-only code browsing for agent worktrees. All endpoints take `worktree=<agent name>` to target an agent's worktree.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/code/tree` | Directory listing. Query: `path`, `worktree`, `show_hidden=1`. Returns `[{name, path, is_dir, size?}]` (`.git` and `.mycel` hidden at top level by default). |
| GET | `/api/code/file` | File contents (max 2 MiB). Query: `path` (required), `worktree`. |
| GET | `/api/code/diff` | `git diff main...HEAD` for the worktree as `text/plain`. Query: `worktree`, `path` (optional narrow). Empty body for the `main` worktree or missing `main` ref. |
| GET | `/api/code/search` | ripgrep-backed search (API-only; no web UI caller yet). Query: `q` (required, ≤1024 chars), `worktree`, `path` (subdir), `max` (default 500, max 2000), `case=1` (case-insensitive), `regex=1` (regex instead of literal). Returns `{"matches":[{path, line, col, text, before, after}], "truncated", "elapsed_ms"}`. |

`GET /api/code` (no sub-path) returns `404` with a hint listing the sub-routes.

---

## MCP Protocol (agent-facing)

The MCP server agents connect to for `send_message` / `query_costs` and friends. These paths bypass API-key auth.

| Method | Path | Description |
|--------|------|-------------|
| ANY | `/_mcp/{agent}` | Streamable HTTP MCP endpoint (stateless, JSON responses). The path segment is the trusted agent identity. Unknown paths return a JSON `404`. |

See [MCP](../explanation/mcp.md) for the tool list and transport details.

---

## Endpoint Index

| Resource | Endpoints |
|----------|-----------|
| Health | 4 |
| Agents (collection + bulk + per-agent) | 43 |
| Agent stats timeseries | 8 |
| Channels & Notify | 8 |
| Apps (+ webhooks) | 16 |
| Costs (incl. global) | 13 |
| Secrets | 5 |
| MCP servers | 7 |
| Tools (incl. unified) | 10 |
| Templates | 5 |
| Providers | 9 |
| Roles | 5 |
| Settings | 2 |
| Events, Logs & SSE | 6 |
| Stats (summaries + system + channels) | 10 |
| Repos (+ discovery/clone) | 12 |
| Doctor | 2 |
| Dependencies | 6 |
| Files | 2 |
| Code | 4 |
| MCP protocol | 1 |
