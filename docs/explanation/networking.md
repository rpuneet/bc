# Networking & Communication Architecture

## Component Communication

```mermaid
graph TB
    CLI[mycel CLI] -->|HTTP REST| BCD[mycel server :9374]
    WEB[Web UI] -->|HTTP + SSE| BCD
    TUI[TUI] -->|mycel CLI| CLI
    AGENT_MCP[AI Agents] -->|MCP stdio/SSE| BCD

    BCD -->|SQL| DB[(~/.mycel/mycel.db)]
    BCD -->|docker exec<br/>tmux send-keys| AGENTS[Agent Containers]
    BCD -->|SSE broadcast| WEB
    BCD -->|SSE broadcast| TUI

    AGENTS -->|hook POST| BCD
```

All communication flows through the **mycel server** as the central hub. No component talks directly to another.

## Protocol Reference

| Interface | Protocol | Endpoint | Purpose |
|-----------|----------|----------|---------|
| REST API | HTTP/JSON | `/api/*` (see the [REST API reference](../reference/api-rest.md)) | CRUD for all resources |
| SSE Events | HTTP SSE | `/api/events` | Real-time state updates |
| MCP (stdio) | JSON-RPC 2.0 | stdin/stdout | Agent -> server integration |
| MCP (SSE) | JSON-RPC 2.0 | `/_mcp/sse` + `/_mcp/message` | Remote MCP clients |
| Health | HTTP | `/health` | Liveness probe |

## Notification Delivery Flow

External platform events are delivered to subscribed agents via the notification gateway:

```mermaid
sequenceDiagram
    participant Platform as External Platform
    participant Adapter as Gateway Adapter
    participant Notify as notify.Service
    participant DB as SQLite
    participant Hub as SSE Hub
    participant Agent as Subscribed Agents
    participant Web as Web UI

    Platform->>Adapter: Inbound event (message/webhook)
    Adapter->>Notify: Dispatch(channel, sender, content)
    Notify->>DB: Save message + query subscribers
    Notify->>Hub: Publish gateway.message event
    Hub->>Web: SSE: gateway.message

    loop Each subscriber (with self-skip + mention filter)
        Notify->>Agent: tmux send-keys (JSON payload)
        Notify->>DB: Log delivery (delivered/failed)
    end
```

See [Notification Architecture](../architecture-notifications.md) for the full notification system design.

## Agent Hook Event Flow

Claude Code hooks fire on tool use start/stop, updating agent state:

```mermaid
sequenceDiagram
    participant Claude as Claude Code
    participant Hook as Hook Script
    participant API as mycel API
    participant Hub as SSE Hub
    participant Web as Web UI

    Claude->>Hook: tool_use_start event
    Hook->>API: POST /api/agents/{name}/hook
    API->>API: UpdateAgentState(working)
    API->>Hub: Publish agent.state event
    Hub->>Web: SSE: agent state = working

    Claude->>Hook: tool_use_end event
    Hook->>API: POST /api/agents/{name}/hook
    API->>API: UpdateAgentState(idle)
    API->>Hub: Publish agent.state event
    Hub->>Web: SSE: agent state = idle
```

## MCP Integration

AI agents connect to the mycel server's MCP endpoint to read system state and take actions:

```mermaid
sequenceDiagram
    participant Agent as Claude Code
    participant MCP as MCP Server
    participant Svc as Services

    Agent->>MCP: initialize (protocol handshake)
    MCP->>Agent: capabilities (resources + tools)

    Agent->>MCP: resources/read bc://agents
    MCP->>Svc: List agents
    Svc->>MCP: Agent data
    MCP->>Agent: JSON response

    Agent->>MCP: tools/call send_message
    MCP->>Svc: Send to gateway channel
    Svc->>MCP: Result
    MCP->>Agent: Tool result
```

### MCP Transports

| Transport | Connection | Use Case |
|-----------|-----------|----------|
| **stdio** | `mycel mcp serve` via `.mcp.json` | Claude Code agents (local) |
| **SSE** | `GET /_mcp/sse` + `POST /_mcp/message` (agent-scoped: `/_mcp/{agent}/sse\|message`) | Remote/browser MCP clients |

Messages sent over the SSE transport go through the server's global HTTP
middleware, so they are subject to the **1 MB** request body cap
(`MaxBodySize`). The stdio transport has no such cap.

## SSE Event System

The mycel server maintains an in-memory SSE hub. All connected clients (web UI, TUI) receive real-time events.

```mermaid
graph LR
    subgraph Sources
        AGENT_SVC[Agent Service]
        NOTIFY_SVC[Notify Service]
        COST_SVC[Cost Importer]
    end

    HUB[SSE Hub<br/>in-memory]

    subgraph Subscribers
        WEB1[Web UI Client 1]
        WEB2[Web UI Client 2]
        TUI1[TUI via CLI]
    end

    AGENT_SVC -->|agent.created<br/>agent.stopped<br/>agent.state| HUB
    NOTIFY_SVC -->|gateway.message<br/>gateway.delivery| HUB
    HUB --> WEB1
    HUB --> WEB2
    HUB --> TUI1
```

### Event Types

| Event | Trigger | Payload |
|-------|---------|---------|
| `connected` | Client connects to SSE | `{"status":"connected"}` |
| `agent.created` | Agent created | `{"name","role","tool"}` |
| `agent.started` | Agent started/restarted | `{"name"}` |
| `agent.stopped` | Agent stopped | `{"name","reason"}` |
| `agent.deleted` | Agent deleted | `{"name"}` |
| `agent.renamed` | Agent renamed | `{"old_name","new_name"}` |
| `agents.stopped_all` | All agents stopped | `{"count"}` |
| `gateway.message` | Inbound platform message | `{"channel","platform","sender","content"}` |

## Request/Response Format

### Success Response
```json
{
  "name": "eng-01",
  "role": "engineer",
  "state": "idle"
}
```

### Error Response
```json
{
  "error": "agent not found: eng-01"
}
```

All responses use `Content-Type: application/json`.

## CORS Policy

- **Default**: `Access-Control-Allow-Origin: *` (safe on loopback)
- **Methods**: GET, POST, PUT, PATCH, DELETE, OPTIONS
- **Headers**: Content-Type, Authorization

Wildcard CORS is acceptable because the server binds to `127.0.0.1` by default. When exposed beyond loopback (Docker `0.0.0.0`), CORS should be restricted and API-key auth enabled (`mycel up --api-key`).

## Connection Lifecycle

### SSE Connections
- Server sends `data: {"type":"connected"}` immediately on connect
- No keepalive pings (relies on TCP keepalive)
- Client reconnects on disconnect (EventSource auto-reconnect)
- WriteTimeout disabled on server for long-lived SSE connections
- IdleTimeout: 120 seconds

### MCP SSE Connections
- Server sends `event: endpoint` with message POST URL on connect
- Client POSTs JSON-RPC to the message endpoint
- Server sends responses via SSE stream
- ReadHeaderTimeout: 10 seconds (Slowloris protection)

## Port Allocation

| Port | Service | Binding |
|------|---------|---------|
| 9374 | mycel server (REST + SSE + MCP + Web UI) | `127.0.0.1` (default) |
| 5432 | bc-db (TimescaleDB/Postgres, optional) | `127.0.0.1` |

A single port serves everything: REST API, SSE events, MCP protocol, and embedded web UI (SPA with client-side routing).