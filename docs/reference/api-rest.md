# REST API Reference

**Base URL:** `http://127.0.0.1:9374`
**Content-Type:** `application/json`
**Authentication:** None (localhost-only)
**Total endpoints:** 41 across 14 resource groups

---

## Health

### `GET /health`
Liveness probe.

**Response:** `200 OK`
```json
{
  "status": "ok",
  "addr": "127.0.0.1:9374"
}
```

### `GET /health/ready`
Readiness probe. Checks DB connectivity and agent runtime.

**Response:** `200 OK`
```json
{
  "status": "ok",
  "checks": {
    "db": "ok",
    "agents": "23 total"
  }
}
```

> Returns `503 Service Unavailable` if status is `"degraded"`.

---

## Agents

### `GET /api/agents`
List all agents. Reconciles with live sessions before returning.

**Query params:**
| Param | Type | Description |
|-------|------|-------------|
| role | string | Filter by role |
| state | string | Filter: running, stopped, error, starting |
| team | string | Filter by team ID |

**Response:** `200 OK` — `AgentDTO[]`

### `POST /api/agents`
Create and start a new agent.

**Body:**
```json
{
  "name": "eng-01",
  "role": "engineer",
  "workspace": "~/repos/my-project",
  "tool": "claude",
  "runtime": "docker",
  "team": "backend-team"
}
```
- `name` — required, alphanumeric + hyphens/underscores
- `role` — required, must exist in `roles` table
- `workspace` — required if no team (inherits team workspace if omitted)
- `tool` — optional, default from settings
- `runtime` — optional, "tmux" or "docker"
- `team` — optional, adds agent to team

**Response:** `201 Created` — `AgentDTO`

### `GET /api/agents/{name}`
**Response:** `200 OK` — `AgentDTO` | `404`

### `DELETE /api/agents/{name}?force=true`
Delete agent. Cleans up Docker container, git worktree, and branch.

**Response:** `204 No Content`

### `POST /api/agents/{name}/start`
Start a stopped agent. Resumes Claude session if valid UUID exists.

**Response:** `200 OK` — `AgentDTO`

### `POST /api/agents/{name}/stop`
**Response:** `200 OK` — `{"status": "stopped"}`

### `POST /api/agents/{name}/send`
Send text to agent's tmux/Docker session.

**Body:** `{"message": "string"}`
**Response:** `200 OK` — `{"status": "sent"}`

### `POST /api/agents/{name}/hook`
Receive Claude Code hook event. Updates agent state.

**Body:** `{"event": "tool_use_start | tool_use_end | user_input_required | stop"}`
**Response:** `200 OK` — `{"ok": true}`

### `GET /api/agents/{name}/peek?lines=500`
Read recent terminal output via `tmux capture-pane`. Returns readable formatted output.

**Query params:** `lines` (int, default 500, max 10000)
**Response:** `200 OK` — `{"output": "string"}`

### `GET /api/agents/{name}/stats?limit=20`
Docker resource stats (CPU, memory, network).

**Response:** `200 OK` — `AgentStatsRecord[]`

### `POST /api/agents/{name}/rename`
**Body:** `{"new_name": "string"}`
**Response:** `200 OK`

### `GET /api/agents/{name}/sessions`
Session history (current + archived UUIDs with timestamps).

**Response:** `200 OK` — `SessionEntry[]`

### `GET /api/agents/generate-name`
**Response:** `200 OK` — `{"name": "witty-parrot"}`

### `POST /api/agents/broadcast`
Send to all running agents.

**Body:** `{"message": "string", "team": "optional-team-id"}`
- If `team` specified, sends only to agents in that team
**Response:** `200 OK` — `{"sent": 3}`

### `POST /api/agents/send-role`
Send to all agents with a specific role.

**Body:** `{"role": "engineer", "message": "string"}`
**Response:** `200 OK` — `SendResult`

### `POST /api/agents/stop-all`
**Response:** `200 OK` — `{"stopped": 5}`

---

## Teams

### `GET /api/teams`
List all teams as a flat list. Use `parent_id` to build tree client-side.

**Response:** `200 OK` — `TeamDTO[]`
```json
[{
  "id": "backend-team",
  "name": "Backend",
  "parent_id": "root-team",
  "workspace": "~/repos/api",
  "agents": ["eng-01", "eng-02"],
  "children": ["db-team"],
  "created_at": 1711000000000
}]
```

### `POST /api/teams`
**Body:** `{"id": "backend", "name": "Backend Team", "parent_id": "root", "workspace": "~/repos/api"}`
**Response:** `201 Created` — `TeamDTO`

### `GET /api/teams/{id}`
### `PUT /api/teams/{id}`
### `DELETE /api/teams/{id}`
Deleting a team does NOT delete its agents.

### `POST /api/teams/{id}/members`
Add agent to team.

**Body:** `{"agent_id": "eng-01"}`
**Response:** `204 No Content`

### `DELETE /api/teams/{id}/members?agent_id=eng-01`
Remove agent from team.

---

## Roles

Roles are stored in the database. No markdown files on disk.

### `GET /api/roles`
List all roles (metadata only, no prompt bodies).

### `POST /api/roles`
Create role.

**Body:**
```json
{
  "name": "engineer",
  "description": "Implements features and fixes bugs",
  "prompt": "You are a senior engineer...",
  "settings": {"model": "opus"},
  "commands": {"lint": "Run linting on the codebase"},
  "mcp_servers": ["playwright", "github"],
  "secrets": ["GITHUB_TOKEN"]
}
```
**Response:** `201 Created`

### `GET /api/roles/{id}`
Full role including prompt body and settings.

### `PUT /api/roles/{id}`
Update role.

### `DELETE /api/roles/{id}`
Delete role. Agents keep their current config.

---

## Gateways & Channels

Notification gateways bridge external platforms to bc agents. See [Channel Architecture](../architecture-notifications.md) for full design.

### `GET /api/gateways`

List all gateways with connection status.

### `POST /api/gateways`

Connect a new gateway.

**Body:** `{"platform": "slack", "tokens": {"bot_token": "xoxb-...", "app_token": "xapp-..."}}`

### `PATCH /api/gateways/{gateway}`

Update gateway tokens/settings.

### `DELETE /api/gateways/{gateway}`

Disconnect and remove gateway.

### `GET /api/gateways/{gateway}/health`

Live connection probe.

### `GET /api/gateways/{gateway}/channels`

List discovered channels for a gateway.

### `POST /api/gateways/{gateway}/channels/{channel}/agents`

Subscribe an agent to a channel.

**Body:** `{"agent": "eng-01", "mention_only": false}`

### `DELETE /api/gateways/{gateway}/channels/{channel}/agents/{agent}`

Unsubscribe agent from channel.

### `PATCH /api/gateways/{gateway}/channels/{channel}/agents/{agent}`

Update subscription settings (e.g., toggle mention_only).

**Body:** `{"mention_only": true}`

### `GET /api/gateways/{gateway}/channels/{channel}/activity`

Recent delivery log entries for a channel.

---

## Costs

### `GET /api/costs`
Workspace cost summary with token breakdown.

**Response:**
```json
{
  "total_cost_usd": 12.50,
  "input_tokens": 500000,
  "output_tokens": 150000,
  "cache_read_tokens": 300000,
  "cache_creation_tokens": 50000,
  "request_count": 250,
  "period": "all_time"
}
```

### `GET /api/costs/agents`
Per-agent cost breakdown with token details.

### `GET /api/costs/teams`
Per-team cost aggregation.

### `GET /api/costs/models`
Per-model cost breakdown.

### `GET /api/costs/daily?days=30`
Daily cost time series (for graphs).

**Response:** `200 OK`
```json
[
  {"date": "2026-03-20", "cost_usd": 2.50, "input_tokens": 100000, "output_tokens": 30000, "requests": 45},
  {"date": "2026-03-21", "cost_usd": 3.10, "input_tokens": 120000, "output_tokens": 35000, "requests": 52}
]
```

### `GET /api/costs/agent/{name}?days=7`
Single agent cost time series.

### `POST /api/costs/sync`
Trigger JSONL cost import from Claude session files.

---

## Secrets

Values are AES-256-GCM encrypted. API never returns values.

### `GET /api/secrets`
### `POST /api/secrets`
**Body:** `{"name": "GITHUB_TOKEN", "value": "ghp_...", "description": "GitHub PAT"}`

### `GET /api/secrets/{name}`
Metadata only (no value).

### `PUT /api/secrets/{name}`
### `DELETE /api/secrets/{name}`

---

## Cron

Scheduled bash commands that run on a timer. To prompt an agent, use a cron job that curls the agent send API.

### `GET /api/cron`
### `POST /api/cron`
**Body:**
```json
{
  "name": "nightly-lint",
  "schedule": "0 2 * * *",
  "command": "cd ~/repos/api && make lint",
  "enabled": true
}
```

### `GET /api/cron/{name}`
### `DELETE /api/cron/{name}`
### `POST /api/cron/{name}/enable`
### `POST /api/cron/{name}/disable`
### `POST /api/cron/{name}/run`
Manual trigger.

### `GET /api/cron/{name}/logs?last=20`

---

## Daemons

Long-running processes managed by bcd. Support tmux and Docker runtimes with restart policies.

### `GET /api/daemons`
List all daemons with pagination.

**Query params:** `limit` (int, default 50), `offset` (int, default 0)

**Response:** `200 OK` -- `Daemon[]`

### `POST /api/daemons`
Create and start a daemon.

**Body:**
```json
{
  "name": "db",
  "cmd": "postgres -D /data",
  "image": "postgres:17",
  "runtime": "docker",
  "restart": "always",
  "env": ["POSTGRES_PASSWORD=secret"],
  "ports": ["5432:5432"]
}
```
- `name` -- required, unique identifier
- `cmd` -- required, command to run
- `image` -- Docker image (required if runtime is docker)
- `runtime` -- "tmux" or "docker"
- `restart` -- restart policy: "no", "always", "on-failure"
- `env` -- environment variables as `KEY=VALUE` strings
- `ports` -- port mappings as `HOST:CONTAINER` strings

**Response:** `201 Created` -- `Daemon`

### `GET /api/daemons/{name}`
**Response:** `200 OK` -- `Daemon` | `404`

### `POST /api/daemons/{name}/stop`
**Response:** `200 OK` -- `{"status": "stopped"}`

### `POST /api/daemons/{name}/restart`
**Response:** `200 OK` -- `Daemon`

### `DELETE /api/daemons/{name}`
**Response:** `204 No Content`

---

## Tools

AI tool provider configurations.

### `GET /api/tools`
### `GET /api/tools/{name}`
### `PUT /api/tools/{name}`
### `DELETE /api/tools/{name}`
### `POST /api/tools/{name}/enable`
### `POST /api/tools/{name}/disable`

---

## MCP Servers

External MCP server configurations for agents.

### `GET /api/mcp`
### `POST /api/mcp`
**Body:**
```json
{
  "name": "playwright",
  "transport": "sse",
  "url": "http://localhost:3100/sse",
  "env": {"BROWSER": "chromium"},
  "enabled": true
}
```

### `GET /api/mcp/{name}`
### `DELETE /api/mcp/{name}`
### `POST /api/mcp/{name}/enable`
### `POST /api/mcp/{name}/disable`

---

## Event Log

### `GET /api/logs?tail=100`
Recent events. Default: last 100.

**Query params:** `tail` (int, default 100, max 10000), `type` (filter by event type)

### `GET /api/logs/{agent}`
Events for specific agent. Same params.

---

## Doctor

### `GET /api/doctor`
Run all health checks.

### `GET /api/doctor/{category}`

---

## SSE Events

### `GET /api/events`
Server-Sent Events stream.

**Event types:**

| Type | Payload | When |
|------|---------|------|
| `connected` | `{}` | Client connects |
| `agent.created` | `{name, role, tool}` | Agent created |
| `agent.started` | `{name, session_id}` | Agent started/restarted |
| `agent.stopped` | `{name, reason}` | Agent stopped |
| `agent.deleted` | `{name}` | Agent deleted |
| `agent.state_changed` | `{name, state, task}` | State transition (idle/working/stuck) |
| `gateway.message` | `{channel, platform, sender, content, mentions, timestamp}` | Inbound platform message |
| `cost.updated` | `{agent, cost_usd, tokens}` | Cost import completed |
| `team.updated` | `{team_id, action}` | Team membership changed |

---

## Settings

Workspace configuration management. See [settings.md](settings.md) for full details.

### `GET /api/settings`
Returns the full workspace configuration.

**Response:** `200 OK` -- full config object

### `PUT /api/settings`
Partial update of the full configuration. Send only the sections you want to change.

**Body:** JSON object with one or more config sections.

**Supported sections:** `user`, `providers`, `env`, `logs`, `runtime`, `performance`, `tui`, `workspace`, `roster`, `services`

**Response:** `200 OK` -- full updated config

### `PATCH /api/settings/{section}`
Update a single config section. The request body is the section object directly (not wrapped in a parent key).

**Supported sections:** `user`, `tui`, `runtime`, `providers`, `services`, `logs`, `performance`, `env`, `roster`

**Example:**
```bash
curl -X PATCH http://localhost:9374/api/settings/tui \
  -H "Content-Type: application/json" \
  -d '{ "theme": "matrix", "mode": "dark" }'
```

**Response:** `200 OK` -- full updated config

---

## Stats

System metrics and workspace summary.

### `GET /api/stats/system`
System-level resource metrics.

**Response:** `200 OK`
```json
{
  "hostname": "macbook",
  "os": "darwin",
  "arch": "arm64",
  "cpus": 10,
  "cpu_usage_percent": 23.5,
  "memory_total_bytes": 17179869184,
  "memory_used_bytes": 12884901888,
  "memory_usage_percent": 75.0,
  "disk_total_bytes": 500000000000,
  "disk_used_bytes": 250000000000,
  "disk_usage_percent": 50.0,
  "go_version": "go1.25.4",
  "uptime_seconds": 3600,
  "goroutines": 42
}
```

### `GET /api/stats/summary`
Workspace-level summary counts.

**Response:** `200 OK`
```json
{
  "agents_total": 5,
  "agents_running": 3,
  "agents_stopped": 2,
  "notification_sources_total": 4,
  "notifications_total": 120,
  "total_cost_usd": 12.50,
  "roles_total": 3,
  "tools_total": 2,
  "uptime_seconds": 3600
}
```

### `GET /api/stats/notifications`
Notification-level statistics.

**Response:** `200 OK`

---

## Workspace

Workspace lifecycle management.

### `GET /api/workspace`
### `GET /api/workspace/status`
Workspace status including agent counts, runtime info, and config.

**Response:** `200 OK`

### `GET /api/workspace/roles`
List all available roles.

**Response:** `200 OK`

### `POST /api/workspace/up`
Start the workspace (equivalent to `bc up`).

**Response:** `200 OK`

### `POST /api/workspace/down`
Stop the workspace and all agents (equivalent to `bc down`).

**Response:** `200 OK`

---

## MCP Protocol

### `GET /mcp/sse`
MCP SSE transport -- server-to-client events.

### `POST /mcp/message`
MCP JSON-RPC 2.0 -- client-to-server. 4MB body limit.

See [backend/mcp.md](../backend/mcp.md) for resources, tools, and notifications.
