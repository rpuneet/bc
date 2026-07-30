# Architecture Decision Records

This document records the key architectural decisions in mycel, their
context, and the reasoning behind each choice.

---

## ADR-1: SQLite as the Default Database

**Status:** Accepted

**Context:** mycel needs persistent storage for notifications, costs, events,
secrets, MCP servers, and tools. The storage must work out of the
box for every developer without any setup steps.

**Decision:** Use SQLite for all persistent storage: one global database at
`~/.mycel/mycel.db` for every store (agents, roles, events, notifications,
MCP servers, tools), plus a `secrets.vault` alongside it. Costs are not
persisted at all — they are computed from provider session files on demand.

**Rationale:**

- **Zero configuration**: no database server to install, configure, or manage.
- **Embedded**: the database is a single file linked into the Go binary.
- **Repos stay pristine**: all state lives under `~/.mycel/`, never inside
  the project. State survives repo moves and clones.
- **Concurrent-safe**: SQLite WAL mode handles the concurrency level mycel
  needs (one server, a few CLI readers).
- **Tables use `IF NOT EXISTS`**: schema is applied idempotently at startup,
  so there is no migration tooling to maintain.

**Tradeoffs:**

- Not suitable for multi-node deployments (not a current requirement).
- Write concurrency is limited to one writer at a time (acceptable for a
  local tool).

---

## ADR-2: Tmux + Docker Dual Runtime

**Status:** Accepted

**Context:** Agents need an interactive session environment for running AI
tools (Claude Code, Gemini, etc.) that expect a terminal. The system must
work for local development and for isolated, reproducible builds.

**Decision:** Support two runtime backends — Docker (isolated, the default)
and tmux (local) — selectable via `runtime.default` in `prefs.json`.

**Rationale:**

- **Tmux for local development**: zero overhead, instant startup, direct
  filesystem access. Developers already have tmux installed. Each agent gets
  its own tmux session with a per-agent git worktree.
- **Docker for isolation**: each agent runs in its own container with
  resource limits (CPU, memory), controlled volume mounts, and optional
  network restrictions. Provides reproducible environments across machines.
- **Unified interface**: both backends implement the `runtime.Backend`
  interface (`HasSession`, `CreateSession`, `SendKeys`, `Capture`,
  `KillSession`, etc.), so the agent manager code is backend-agnostic.
- **Docker uses tmux internally**: even Docker containers run tmux inside for
  session management. Communication uses `docker exec ... tmux send-keys`,
  requiring no persistent connections or FIFOs.

**Tradeoffs:**

- Docker backend requires Docker daemon and pre-built agent images.
- Docker agents start without auth and need manual `mycel agent attach` for
  initial login.

---

## ADR-3: SSE (Server-Sent Events) for Real-Time Updates

**Status:** Accepted

**Context:** The web dashboard needs real-time updates when agent
state changes, channel messages arrive, or costs are recorded.

**Decision:** Use Server-Sent Events (SSE) at `/api/events` instead of
WebSockets.

**Rationale:**

- **Simpler protocol**: SSE is plain HTTP — one long-lived GET request with
  `text/event-stream` content type. No upgrade handshake, no frame parsing.
- **Auto-reconnect**: browsers and SSE client libraries handle reconnection
  automatically with `EventSource`. No custom reconnection logic needed.
- **Works through proxies**: SSE uses standard HTTP, so it works through
  reverse proxies, load balancers, and firewalls without special
  configuration (unlike WebSocket upgrade requests).
- **Unidirectional is sufficient**: the server pushes state updates to
  clients. Client-to-server communication uses REST API calls, which is the
  natural fit for command/query separation.
- **Implementation**: the `ws.Hub` struct manages SSE subscribers and
  broadcasts JSON events. The `WriteTimeout` on the HTTP server is set to 0
  to allow long-lived SSE connections, with per-handler timeouts used
  elsewhere.

**Tradeoffs:**

- Unidirectional only (server→client). Not suitable if bidirectional
  streaming were needed.
- Maximum ~6 concurrent SSE connections per browser per domain (browser
  limit, not relevant for localhost single-user use).

---

## ADR-4: Embedded Web UI in the Server Binary

**Status:** Accepted

**Context:** mycel ships a web dashboard for managing agents, repos, and
costs. It needs to be easy to deploy and use without a separate frontend
server.

**Decision:** Embed the compiled web UI (from `server/web/dist/`) into the
`mycel` binary using Go's `embed.FS`, served as static files with SPA
fallback.

**Rationale:**

- **Single binary deployment**: `mycel` is one binary that contains the API
  server, SSE hub, MCP server, and the complete web UI. No separate `npm
  start` or nginx configuration.
- **SPA routing**: the server tries to serve the exact file path first; if
  the file does not exist, it falls back to `index.html` for client-side
  routing.
- **Development mode**: during development, `make run-web` runs a Vite
  dev server with hot reload, proxying API calls to the mycel server.
- **Build pipeline**: `make build-local-mycel` runs `make build-local-web`
  first to produce `server/web/dist/`, then embeds it into the Go binary.

**Tradeoffs:**

- Web UI changes require rebuilding the Go binary (mitigated by the dev
  server workflow).
- Binary size increases by the size of the compiled frontend assets.

---

## ADR-5: HTTP Hooks for Agent State Detection

**Status:** Accepted

**Context:** the mycel server needs to know when agents transition between
states (working, idle, stuck, stopped). Agents run inside tmux sessions or
Docker containers.

**Decision:** Use HTTP hooks: Claude Code lifecycle events (`SessionStart`,
`UserPromptSubmit`, `PreToolUse`, `Stop`, and others) POST a JSON payload to
the server's `/api/agents/{name}/hook` endpoint. The server address comes
from the `MYCEL_DAEMON_ADDR` env var set per agent.

**Rationale:**

- **Instant updates**: state changes reach the server the moment the event
  fires — no poll cycle. The server updates `mycel.db` and broadcasts over
  SSE, so the web UI stays live.
- **Full payload preserved**: the hook command reads Claude's raw stdin
  JSON, merges in mycel's `event`/`state`/`task` fields with `jq`, and
  POSTs everything — tool names, tool input, and session IDs included.
- **Works in Docker**: containers reach the host server over the network
  via `MYCEL_DAEMON_ADDR`; no shared-filesystem tricks needed.
- **Fire-and-forget**: each hook ends in `|| true`, so a down or slow
  server never blocks or breaks the agent.
- **Claude Code integration**: hooks are configured in
  `.claude/settings.json` using Claude Code's native hook system. The
  `WriteClaudeHookSettings()` function generates the settings
  idempotently, merging with any existing user hooks.

**Tradeoffs:**

- Events fired while the server is down are lost; state re-syncs on the
  next event.
- Only works with Claude Code's hook system. Other AI tools need different
  state detection mechanisms.

---

## ADR-6: BFS Role Inheritance

**Status:** Accepted

**Context:** Roles can inherit from parent roles to share capabilities,
prompts, MCP servers, and secrets. The inheritance model must be simple and
predictable.

**Decision:** Use breadth-first search (BFS) for role inheritance resolution
via the `parent_roles` JSON column on the `roles` database table.

**Rationale:**

- **Simple**: BFS is easy to understand and implement. Walk the parent chain
  level by level.
- **Predictable**: the resolution order is deterministic — closer parents
  take priority over distant ancestors.
- **No diamond problem**: BFS with visited-set tracking naturally handles
  cases where two parents share a common ancestor. Each role is visited only
  once, and the first encounter wins.
- **Flat hierarchy in practice**: most setups use 2-3 levels at most
  (e.g., `engineer` inherits from `base`, `lead` inherits from `engineer`).

**Tradeoffs:**

- No support for method-resolution-order (MRO) style linearization like
  Python's C3. Not needed given the simple role hierarchies in practice.
- No override/conflict detection — last-writer-wins for merged fields.
