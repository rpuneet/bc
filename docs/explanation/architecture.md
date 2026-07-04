# Architecture

This document describes the internal architecture of mycel, covering component relationships, data flow, and key design decisions.

## Component Diagram

mycel ships as a single binary. `mycel <verb>` subcommands are thin HTTP clients; `mycel up` runs the server (API, web UI, MCP, agent management) in the same binary — foreground by default, background daemon with `-d`.

```
                          +-----------+
                          |  User /   |
                          |  Terminal |
                          +-----+-----+
                                |
              +-----------------+------------------+
              |                 |                   |
       +------v------+  +------v------+   +--------v-------+
       |  mycel CLI  |  |  TUI        |   |  Web Browser   |
       |  (Go binary)|  |  (React Ink)|   |                |
       +------+------+  +------+------+   +--------+-------+
              |                 |                   |
              |  HTTP/JSON      |  HTTP/JSON        |  HTTP + SSE
              |                 |                   |
       +------v-----------------v-------------------v-------+
       |               mycel server (mycel up)               |
       |                  127.0.0.1:9374                     |
       |                                                     |
       |  Middleware (outermost first):                      |
       |    RateLimit > APIKeyAuth > RequestID >             |
       |    RequestLogger > Recovery > Gzip >                |
       |    MaxBodySize(1MB) > CORS > mux                    |
       |                                                     |
       |  +------------+  +-----------+  +----------------+  |
       |  | REST API   |  | SSE Hub   |  | MCP Server     |  |
       |  | /api/*     |  | /api/     |  | /_mcp/...      |  |
       |  | (see REST  |  | events    |  | (JSON-RPC 2.0) |  |
       |  | API ref)   |  +-----+-----+  +--------+-------+  |
       |  +-----+------+        |                 |          |
       |  +-----v----------------v----------------v-------+  |
       |  |              Service Layer                     |  |
       |  |                                                |  |
       |  |  AgentService    NotifyService    CronService  |  |
       |  |  CostStore       SecretStore      EventLog     |  |
       |  |  RoleStore       ToolStore        MCPStore     |  |
       |  |  WorkspaceManager                              |  |
       |  +-----+--------------------+--------------------+  |
       |        |                    |                        |
       |  +-----v---------+  +------v---------------------+  |
       |  | Runtime       |  | Storage                    |  |
       |  |               |  |                            |  |
       |  | +----------+  |  | <ws>/.bc/bc.db (SQLite WAL |  |
       |  | | tmux     |  |  |   or TimescaleDB)          |  |
       |  | | sessions |  |  | <ws>/.bc/settings.json     |  |
       |  | +----------+  |  | ~/.mycel/ global tree      |  |
       |  | +----------+  |  |   (registry, secrets vault,|  |
       |  | | Docker   |  |  |    costs.db, templates,    |  |
       |  | |containers|  |  |    workspaces/<id>/agents) |  |
       |  | +----------+  |  +----------------------------+  |
       |  +---------------+                                  |
       |                                                     |
       |  +---------------+                                  |
       |  | Web UI (SPA)  |                                  |
       |  | / (embedded)  |                                  |
       |  +---------------+                                  |
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

The repo has two entry points under `cmd/`: `cmd/mycel` (the binary) and `cmd/gendocs` (CLI reference generation).

## Data Flow

### Request Lifecycle

1. **Client** (mycel CLI, Web UI, or TUI) sends an HTTP request to the server. Clients discover the address via `BC_DAEMON_ADDR`, the `~/.mycel/daemon.addr` file written by `mycel up`, or the `127.0.0.1:9374` default.
2. **Middleware chain** processes (outermost first, from `server/server.go`): RateLimit (token bucket, 100 rps / burst 200) → APIKeyAuth (Bearer token, only when an API key is configured) → RequestID → RequestLogger → Recovery → Gzip → MaxBodySize (1 MB) → CORS → mux.
3. **Handler** dispatches to the appropriate service method.
4. **Service** performs business logic, interacts with runtime backends and the workspace database.
5. **SSE Hub** broadcasts events to connected clients for real-time updates.
6. **Response** returns JSON to the caller.

The REST surface is documented in the REST API reference; endpoint counts are deliberately not repeated here because they drift.

### Agent Lifecycle

```
                  POST /api/agents
                        |
                        v
               +--------+--------+
               | Record agent    |
               | state: starting |
               +--------+--------+
                        |
              +---------+---------+
              |                   |
     +--------v--------+ +-------v--------+
     | git worktree add| | Write role     |
     | (pkg/worktree)  | | CLAUDE.md      |
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
                | SSE: agent      |
                |   created       |
                +-----------------+
```

### MCP Integration

AI agents connect to the server via MCP (Model Context Protocol) for workspace operations:

```
AI Agent (Claude Code)             mycel MCP Server
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

HTTP transport is mounted under `/_mcp/`:
- `/_mcp/<agent>/{sse,message}` — SSE stream plus client-request endpoint, agent identity in the path
- `/_mcp/<wsID>/<agent>/…` — workspace-scoped form, dispatched via the WorkspaceManager
- A compatibility shim (`server/mcp_compat.go`) keeps older `/_mcp` URL shapes working

stdio transport is used by locally launched agent tooling.

## Key Design Decisions

### Why a Single Binary with a Daemon Subcommand?

`mycel` is one binary: subcommands are HTTP clients, and `mycel up` is the server. This means:
- CLI commands start instantly (no DB connections, no state loading)
- Multiple CLI invocations share the same server state
- Web UI, TUI, and CLI all see the same data
- The server maintains long-lived concerns (SSE, cost polling, cron)
- One artifact to build, version, and ship — the web UI is embedded in it

### Why SQLite (with a TimescaleDB Option)?

- Zero configuration — no external database to install or manage
- WAL mode enables concurrent reads with a single writer
- Local-first architecture matches the single-machine use case
- Schema is created idempotently (`CREATE TABLE IF NOT EXISTS` per store) — no migration framework
- For server deployments, the same stores run against TimescaleDB (Postgres 17) via `DATABASE_URL` or `storage.default` in settings.json — see `docs/explanation/database.md`

### Why tmux + Docker?

- **tmux**: Zero overhead for local development, instant session creation
- **Docker**: Isolation for untrusted agents, reproducible environments (default runtime)
- Both backends present a uniform interface (start, stop, send-keys, capture-pane)
- Agents are unaware of their runtime — the abstraction is transparent

### Why Embedded Web UI?

The React SPA is compiled and embedded in the binary via `server/web/dist/` (`//go:embed`). This means:
- Single binary deployment — no separate web server
- Version-locked UI — always matches the API
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
- Supports both HTTP/SSE and stdio transports

## Package Dependencies

```
cmd/mycel/       -->  internal/cmd/  -->  pkg/client/
                                     -->  server/  -->  pkg/*

server/
  handlers/      -->  pkg/agent/, pkg/notify/, pkg/cost/, ...
  mcp/           -->  pkg/agent/, pkg/notify/, pkg/cost/

pkg/ (self-contained, minimal cross-imports)
  agent/         -->  pkg/tmux/, pkg/container/, pkg/worktree/
  notify/        -->  pkg/db/ (shared workspace DB)
  cost/          -->  pkg/db/
  workspace/     -->  config/
  tmux/          -->  (external: tmux binary)
  container/     -->  (external: docker binary)
  worktree/      -->  (external: git binary)
```

Rule: `cmd/` imports `pkg/`, never vice versa. `pkg/` packages are self-contained.
