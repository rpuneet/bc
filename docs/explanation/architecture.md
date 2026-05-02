# Architecture

This document describes the internal architecture of bc, covering component relationships, data flow, and key design decisions.

## Component Diagram

```
                          +-----------+
                          |  User /   |
                          |  Terminal |
                          +-----+-----+
                                |
              +-----------------+------------------+
              |                 |                   |
       +------v------+  +------v------+   +--------v-------+
       |  bc CLI     |  |  TUI        |   |  Web Browser   |
       |  (Go binary)|  |  (React Ink)|   |                |
       +------+------+  +------+------+   +--------+-------+
              |                 |                   |
              |  HTTP/JSON      |  HTTP/JSON        |  HTTP + SSE
              |                 |                   |
       +------v-----------------v-------------------v-------+
       |                    bcd Daemon                       |
       |                  127.0.0.1:9374                     |
       |                                                     |
       |  Middleware: Recovery > RequestID > CORS > Gzip     |
       |              > MaxBody > Routes                     |
       |                                                     |
       |  +------------+  +-----------+  +----------------+  |
       |  | REST API   |  | SSE Hub   |  | MCP Server     |  |
       |  | /api/*     |  | /api/     |  | /mcp/sse       |  |
       |  | 41 endpts  |  | events    |  | /mcp/message   |  |
       |  +-----+------+  +-----+----+  | (JSON-RPC 2.0) |  |
       |        |                |       +--------+-------+  |
       |  +-----v----------------v----------------v-------+  |
       |  |              Service Layer                     |  |
       |  |                                                |  |
       |  |  AgentService    NotifyService    TeamService  |  |
       |  |  CostStore       SecretStore      CronService  |  |
       |  |  DaemonManager   EventLog         RoleManager  |  |
       |  |  ToolStore       MCPStore         StatsHandler |  |
       |  +-----+--------------------+--------------------+  |
       |        |                    |                        |
       |  +-----v---------+  +------v---------------------+  |
       |  | Runtime       |  | Storage                    |  |
       |  |               |  |                            |  |
       |  | +----------+  |  | ~/.bc/bc.db (SQLite WAL)   |  |
       |  | | tmux     |  |  | ~/.bc/settings.json        |  |
       |  | | sessions |  |  | ~/.bc/secret-key           |  |
       |  | +----------+  |  | ~/.bc/agents/<name>/       |  |
       |  | +----------+  |  |                            |  |
       |  | | Docker   |  |  | Tables:                    |  |
       |  | |containers|  |  |  agents, notify_subscriptions,|  |
       |  | +----------+  |  |  notify_delivery_log,       |  |
       |  +---------------+  |  notify_gateways,           |  |
       |                     |  notify_channels,           |  |
       |  +---------------+  |  notify_messages, teams,    |  |
       |  | Web UI (SPA)  |  |  team_members, costs,       |  |
       |  | / (embedded)  |  |  secrets, cron_jobs,        |  |
       |  | 15 views      |  |  cron_logs, daemons,       |  |
       |  +---------------+  |  events, tools,            |  |
       |                     |  mcp_servers, roles        |  |
       |                     +----------------------------+  |
       +---------+-------------------------------------------+
                 |
       +---------v-------------------------------------------+
       |              AI Agent Sessions                       |
       |                                                      |
       |  +----------+  +----------+  +----------+           |
       |  | Claude   |  | Gemini   |  | Cursor   |  ...      |
       |  | Code     |  | CLI      |  |          |           |
       |  +----------+  +----------+  +----------+           |
       |                                                      |
       |  Each agent runs in:                                 |
       |  - Isolated tmux session OR Docker container         |
       |  - Dedicated git worktree                            |
       |  - Role-defined prompt + MCP servers + secrets       |
       +------------------------------------------------------+
```

## Data Flow

### Request Lifecycle

1. **Client** (bc CLI, Web UI, or TUI) sends HTTP request to bcd
2. **Middleware chain** processes: Recovery, RequestID, CORS, Gzip, MaxBody
3. **Handler** dispatches to the appropriate service method
4. **Service** performs business logic, interacts with runtime backends and SQLite
5. **SSE Hub** broadcasts events to connected clients for real-time updates
6. **Response** returns JSON to the caller

### Agent Lifecycle

```
                  POST /api/agents
                        |
                        v
               +--------+--------+
               | INSERT into DB  |
               | state: starting |
               +--------+--------+
                        |
              +---------+---------+
              |                   |
     +--------v--------+ +-------v--------+
     | git worktree add| | Write role     |
     | (new branch)    | | CLAUDE.md      |
     +---------+-------+ | .mcp.json      |
               |         | settings.json  |
               |         +-------+--------+
               +---------+-------+
                         |
                +--------v--------+
                | Start runtime   |
                | tmux or Docker  |
                +--------+--------+
                         |
                +--------v--------+
                | Launch provider |
                | (claude, gemini)|
                +--------+--------+
                         |
                +--------v--------+
                | state: idle     |
                | SSE: agent.     |
                |   created       |
                +-----------------+
```

### MCP Integration

AI agents connect to bcd via MCP (Model Context Protocol) for workspace operations:

```
AI Agent (Claude Code)              bcd MCP Server
        |                                  |
        |-- initialize (JSON-RPC 2.0) ---->|
        |<-- capabilities + tools ---------|
        |                                  |
        |-- tools/call send_message ------>|
        |   {channel, message, sender}     |
        |<-- result ----------------------|
        |                                  |
        |-- tools/call report_status ----->|
        |   {agent, task}                  |
        |<-- result ----------------------|
```

Two transports are supported:
- **SSE**: `/mcp/sse` (server events) + `/mcp/message` (client requests) -- used by web/remote clients
- **stdio**: standard input/output -- used by AI agents running locally

## Key Design Decisions

### Why bc/bcd Split?

The CLI (`bc`) is a thin HTTP client that delegates all operations to the daemon (`bcd`). This means:
- CLI starts instantly (no DB connections, no state loading)
- Multiple CLI invocations share the same daemon state
- Web UI, TUI, and CLI all see the same data
- Daemon can maintain long-lived connections (SSE, cost polling)

### Why SQLite?

- Zero configuration -- no external database to install or manage
- WAL mode enables concurrent reads with single-writer
- Local-first architecture matches the single-machine use case
- goose migrations provide proper schema versioning with rollback

### Why tmux + Docker?

- **tmux**: Zero overhead for local development, instant session creation
- **Docker**: Isolation for untrusted agents, reproducible environments
- Both backends present a uniform interface (start, stop, send-keys, capture-pane)
- Agents are unaware of their runtime -- the abstraction is transparent

### Why Embedded Web UI?

The React SPA is compiled and embedded in the bcd binary via `server/web/dist/`. This means:
- Single binary deployment -- no separate web server
- Version-locked UI -- always matches the API
- Works offline with no CDN dependencies

### Why SSE over WebSocket?

- Simpler protocol for server-to-client push (one-way sufficient for events)
- Native browser support via `EventSource` API
- Automatic reconnection built into the protocol
- REST API handles all client-to-server communication

### Why MCP?

- Standard protocol for AI agent integration (JSON-RPC 2.0)
- Agents can discover and call workspace tools dynamically
- Curated tool subset prevents agents from performing admin operations
- Supports both SSE (remote) and stdio (local) transports

## Package Dependencies

```
cmd/mycel/       -->  internal/cmd/  -->  pkg/client/
                 -->  server/        -->  pkg/*

server/
  handlers/      -->  pkg/agent/, pkg/notify/, pkg/cost/, ...
  mcp/           -->  pkg/agent/, pkg/notify/, pkg/cost/

pkg/ (self-contained, no cross-imports between packages)
  agent/         -->  pkg/tmux/, pkg/git/
  notify/        -->  (SQLite only, notification dispatch)
  cost/          -->  (SQLite only)
  workspace/     -->  config/
  tmux/          -->  (external: tmux binary)
  git/           -->  (external: git binary)
```

Rule: `cmd/` imports `pkg/`, never vice versa. `pkg/` packages are self-contained.
