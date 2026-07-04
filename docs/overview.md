# Architecture Overview

## System Design

mycel is a CLI-first orchestration system for coordinating teams of AI coding agents. It ships as a single `mycel` binary that contains both the CLI and the server: `mycel up` starts the long-running server, and every other command talks to it over HTTP. The server manages agents across multiple git repositories from a single global installation at `~/.mycel/` (a legacy `~/.bc/` tree is migrated automatically).

Key numbers:
- **44 REST API endpoints** across 14 resource groups
- **SQLite WAL** database with goose migrations
- **16 web dashboard views** with Cmd+K command palette
- **13 TUI views** with k9s-style keyboard navigation
- **MCP server** with JSON-RPC 2.0 over SSE + stdio transports
- **7 supported AI providers**: Claude, Gemini, Cursor, Aider, Codex, OpenCode, OpenClaw

### Global Installation

Per-workspace runtime state lives under `~/.mycel/workspaces/<id>/` — the
canonical config file is `preferences.json` there, alongside `state.db`,
`agents/`, and `logs/`. mycel also keeps a per-project `.bc/` directory
inside the workspace root:

```
project/
  .bc/
    settings.json          # Workspace config (providers, runtime, defaults)
    agents/
      <name>/
        .claude/            # Claude config (mounted into containers)
          CLAUDE.md         # Role prompt
          settings.json     # Claude Code settings + hooks
          .mcp.json         # MCP server configs
        worktree/           # Git worktree checkout
    roles/                 # Role definitions
    notifications/         # Notification data
    prompts/               # Default prompt templates
```

`mycel up` starts the server and bootstraps the workspace (state directory and workspace registration) automatically — there is no separate init step.

## Architecture Layers

```mermaid
graph TB
    subgraph Clients
        CLI[mycel CLI<br/>thin HTTP client]
        WebUI[Web Dashboard<br/>16 views + Cmd+K]
        TUI[TUI<br/>13 views, k9s-style]
        AI[AI Agents<br/>Claude, Gemini, etc.]
    end

    subgraph "mycel server :9374"
        REST[REST API<br/>44 endpoints]
        SSE[SSE Hub<br/>real-time events]
        MCP[MCP Server<br/>JSON-RPC 2.0]
    end

    subgraph Services
        AgentSvc[Agent Service]
        NotifySvc[Notify Service]
        TeamSvc[Team Service]
        CostSvc[Cost Service]
        SecretSvc[Secret Service]
        CronSvc[Cron Service]
        DaemonSvc[Daemon Manager]
        EventSvc[Event Log]
        StatsSvc[Stats Service]
    end

    subgraph "Runtime Backends"
        Tmux[Tmux Runtime<br/>local sessions]
        Docker[Docker Runtime<br/>isolated containers]
    end

    subgraph Storage
        DB[(.bc/bc.db<br/>SQLite WAL)]
    end

    CLI -->|HTTP/JSON| REST
    WebUI -->|HTTP + SSE| REST
    TUI -->|HTTP/JSON| REST
    AI -->|stdio / SSE| MCP

    REST --> AgentSvc & NotifySvc & TeamSvc & CostSvc & SecretSvc & CronSvc & DaemonSvc & EventSvc & StatsSvc
    MCP --> AgentSvc & NotifySvc & CostSvc

    AgentSvc --> Tmux & Docker
    DaemonSvc --> Tmux & Docker
    AgentSvc & NotifySvc & TeamSvc & CostSvc & SecretSvc & CronSvc & DaemonSvc & EventSvc --> DB

    Tmux & Docker --> AI
```

## Components

### mycel CLI (`cmd/mycel/`)

Thin HTTP client. All commands are HTTP requests to the daemon -- no direct DB/filesystem access. Opens the TUI if a workspace exists, prompts init if not, shows help in non-interactive mode.

### Server (`cmd/mycel/`, `server/`)

Long-running HTTP server on `127.0.0.1:9374`, started with `mycel up`. Single process managing all state.

| Component | Path | Purpose |
|-----------|------|---------|
| REST API | `/api/*` | CRUD for all resources (44 endpoints) |
| SSE Hub | `/api/events` | Real-time event stream |
| MCP Server | `/mcp/*` | AI agent integration (JSON-RPC 2.0 over SSE + stdio) |
| Web UI | `/` | Embedded React dashboard (16 views) |
| Health | `/health`, `/health/ready` | Liveness + readiness probes |

Middleware chain (outermost first): RateLimit, APIKeyAuth (optional, via `--api-key`/`BC_API_KEY`), RequestID, RequestLogger, Recovery, Gzip, MaxBodySize (1 MB), CORS, WorkspaceScope, Routes.

### Web Dashboard

React SPA with 16 views, embedded in the `mycel` binary via `server/web/dist/`:

- **Dashboard** -- workspace overview with agent/notification/cost summary
- **Agents** -- list, create, start/stop, send messages, peek output
- **Agent Detail** -- per-agent terminal output, metrics, sessions
- **Notifications** -- notification sources, subscriptions, delivery feed
- **Costs** -- per-agent, per-team, per-model breakdown with daily charts
- **Cron** -- scheduled jobs with enable/disable, manual trigger, logs
- **Daemons** -- long-running process management
- **Doctor** -- health checks and diagnostics
- **Logs** -- event log with type filtering
- **MCP** -- external MCP server configuration
- **Roles** -- role CRUD with prompt editor
- **Secrets** -- encrypted secret management
- **Settings** -- workspace configuration editor
- **Stats** -- system metrics (CPU, memory, disk) and workspace summary
- **Tools** -- AI tool provider configuration

Features: Cmd+K command palette, dark/light theming, responsive layout, SSE real-time updates, inline terminal output.

### TUI

React Ink terminal UI with 13 views and k9s-style keyboard navigation:

- Dashboard, Agents, Agent Detail, Notifications, Costs, Logs, MCP
- Processes, Roles, Secrets, Tools, Worktrees, Help

Built with Bun, compiled to CommonJS in `tui/dist/`.

### Agents

AI coding assistants running in isolated sessions. Each agent has:
- A tmux session or Docker container (runtime backend)
- A git worktree (created and managed by mycel)
- A role defining its prompt, MCP servers, and secrets
- An associated workspace (git repo path)
- Optional team membership for organizational grouping

See [explanation/agents.md](explanation/agents.md) for lifecycle, state machine, and runtime details.

### Teams

Hierarchical organizational groups for visualizing agents. Decoupled from agent lifecycle:

```mermaid
graph TD
    Root[root-team<br/>workspace: ~/repos/main] --> Backend[backend-team<br/>workspace: ~/repos/api]
    Root --> Frontend[frontend-team<br/>workspace: ~/repos/web]
    Backend --> E1[eng-01]
    Backend --> E2[eng-02]
    Frontend --> E3[eng-03]
    E5[devops-01<br/>workspace: ~/repos/infra] -.->|member of| Root
    E5 -.->|member of| Backend
```

- Teams are **views**, not ownership -- agents exist independently
- Agents can appear in **multiple teams** (many-to-many via `team_members`)
- Teams form a tree via `parent_id`
- Teams can have a default workspace; agents inherit it but can override
- Deleting a team does NOT delete its agents

### Notifications

Inbound-only gateway that bridges external platforms (Slack, Telegram, GitHub, and others) to agents:
- Gateway adapters connect via socket, webhook, or polling patterns
- Agents subscribe to notification sources (`platform:channel`)
- Mention-only filtering for noisy channels
- Delivery via `tmux send-keys` with JSON payload
- Delivery logging with retry for failed dispatches
- Self-skip filtering prevents agents from receiving their own echoed messages

### Secrets

AES-256-GCM encrypted secret store. Referenced in agent env vars as `${secret:NAME}`, resolved at runtime. Key derived via PBKDF2-SHA256 (600k iterations).

### Cost Tracking

Automatic import from Claude Code JSONL session files every 5 minutes. Per-agent, per-team, per-model breakdown with budget enforcement.

### Daemons

Long-running processes managed by the mycel server. Support tmux and Docker runtimes with restart policies. Used for workspace infrastructure (databases, services, etc.).

### Cron

Scheduled bash commands that run on a timer. Supports enable/disable, manual trigger, and execution log history.

### Stats

System-level metrics (CPU, memory, disk, uptime, goroutines) and workspace summary (agent counts, notification source counts, cost totals, role/tool counts).

## Data Flow

### Agent Creation

```mermaid
sequenceDiagram
    participant CLI as mycel CLI
    participant API as mycel API
    participant Svc as Agent Service
    participant RT as Runtime
    participant DB as SQLite

    CLI->>API: POST /api/agents
    API->>Svc: Create(name, role, workspace, team)
    Svc->>DB: INSERT INTO agents
    Svc->>RT: git worktree add
    Svc->>RT: Write role files (CLAUDE.md, .mcp.json)
    Svc->>RT: Create tmux session / Docker container
    Svc->>RT: cd worktree && provider-command
    RT-->>Svc: Session alive
    Svc->>DB: state = idle
    Svc-->>CLI: 201 Created
```

### Notification Delivery

```mermaid
sequenceDiagram
    participant Platform as External Platform
    participant Adapter as Gateway Adapter
    participant Notify as notify.Service
    participant DB as SQLite
    participant Hub as SSE Hub
    participant Agent as Subscribed Agent

    Platform->>Adapter: Inbound event
    Adapter->>Notify: Dispatch(channel, sender, content)
    Notify->>DB: Save to notification_log
    Notify->>DB: Query subscribers
    loop Each subscriber (with self-skip + mention filter)
        Notify->>Agent: tmux send-keys (JSON payload)
        Notify->>DB: Log delivery status
    end
    Notify->>Hub: Publish gateway.message SSE event
```

### Agent State via Hooks

```mermaid
sequenceDiagram
    participant Claude as Claude Code
    participant API as mycel API
    participant DB as SQLite
    participant Hub as SSE Hub

    Claude->>API: POST /api/agents/{name}/hook (tool_use_start)
    API->>DB: state = working
    API->>Hub: agent.state_changed
    Claude->>API: POST /api/agents/{name}/hook (tool_use_end)
    API->>DB: state = idle
    API->>Hub: agent.state_changed
```

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Per-project `.bc/` | Per-project directory | Each workspace has its own `.bc/` directory for config, agents, and state |
| Single binary, CLI + server | `mycel up` runs the server | CLI stays fast; the server process holds state and connections |
| SQLite WAL | Single database file | Zero-config, local-first, WAL for concurrent reads |
| Embedded web UI | Single binary | No separate web server; version-locked to API |
| SSE not WebSocket | Server-sent events | Simpler protocol, sufficient for one-way server push |
| Teams as views | Decoupled, many-to-many | No lifecycle coupling; pure organization |
| bc owns worktrees | All providers, uniform | Avoids nesting; consistent across Claude/Gemini/etc. |
| tmux send-keys | Only delivery mechanism | Hooks are one-way; no other way into agent session |
| Auth optional | Localhost by default | Local dev tool; optional Bearer auth via `--api-key`/`BC_API_KEY` for anything beyond loopback |
| MCP curated tools | Subset of API | Agents get key operations, not full admin |
| INTEGER timestamps | Unix millis | Faster range queries, smaller storage than TEXT ISO8601 |
| goose migrations | Versioned schema | Proper versioning, rollback support |
