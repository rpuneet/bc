# bc-agent-runner

Thin HTTP wrapper around the [Claude Agent SDK](https://platform.claude.com/docs/en/agent-sdk).
One process == one Claude agent. bcd POSTs prompts in and listens to typed
events out instead of scraping a tmux pane.

This is **Phase 1** of the SDK runtime migration described in
`docs/proposals/agent-sdk-architecture.md`. It is the building block for the
upcoming `sdk` runtime backend in bcd (Phase 2) and the Railway template
(Phase 4).

## Why

The current bc agent stack runs `claude --dangerously-skip-permissions` inside
a tmux session inside a Docker container, then parses the terminal output to
detect state. This causes MCP identity bugs, regex-based state-detection
failures, and OAuth volume-mount complexity.

The runner replaces the tmux + claude CLI layer with a Node process that
talks to Claude through the official SDK. bcd then talks to the runner over
HTTP. Same isolation (Docker + worktrees), no terminal scraping.

## HTTP API

| Method | Path        | Body / Response |
|--------|-------------|-----------------|
| `GET`  | `/health`   | `{ ok, agent_name, uptime_seconds, sdk_version }` |
| `GET`  | `/status`   | `StatusResponse` (state, tokens, cost, working_dir) |
| `POST` | `/query`    | `QueryRequest` → `202 QueryResponse`. Returns `409` if busy. |
| `POST` | `/stop`     | `{ state, session_id }`. Interrupts the active query. |
| `GET`  | `/messages` | `{ messages: MessageLogEntry[] }`. Full conversation log. |
| `GET`  | `/events`   | SSE stream of `RunnerEvent` (assistant_message, tool_use, tool_result, result, error, stop). |

See `src/types.ts` for the full request/response shapes.

### Query request

```json
{
  "prompt": "implement the auth refactor",
  "system_prompt": "(optional override of BC_ROLE_PROMPT)",
  "max_turns": 50,
  "max_budget_usd": 5.0,
  "resume_session": "(optional session id to resume)",
  "allowed_tools": ["Read", "Edit", "Write", "Bash"],
  "permission_mode": "bypassPermissions"
}
```

Only `prompt` is required. Everything else falls back to runner-level defaults
from environment variables.

## Environment

| Var | Required | Description |
|-----|----------|-------------|
| `BC_AGENT_NAME` | yes | Agent identity used in logs and `/status`. |
| `ANTHROPIC_API_KEY` | yes | Forwarded to the Claude SDK. |
| `BC_AGENT_RUNNER_PORT` | no (`8080`) | HTTP listener port. |
| `BC_AGENT_RUNNER_HOST` | no (`0.0.0.0`) | HTTP listener bind address. |
| `BC_AGENT_WORKING_DIR` | no (`cwd`) | Directory the agent operates in (worktree path inside the container). |
| `BC_ROLE_PROMPT` | no | Default system prompt for queries. |
| `BC_ALLOWED_TOOLS` | no | JSON array of allowed tool names (default = SDK default). |
| `BC_MAX_TURNS` | no | Default `maxTurns` for queries. |
| `BC_MAX_BUDGET_USD` | no | Default budget cap for queries. |
| `BC_MCP_SERVERS` | no | JSON object of MCP server configs forwarded to the SDK. |

## Running locally

```bash
cd packages/bc-agent-runner
bun install
bun run build
ANTHROPIC_API_KEY=sk-... \
BC_AGENT_NAME=local-test \
BC_AGENT_WORKING_DIR=/tmp/scratch \
node dist/index.js
```

Then in another terminal:

```bash
curl localhost:8080/health
curl -X POST localhost:8080/query \
  -H 'content-type: application/json' \
  -d '{"prompt":"list the files in this directory"}'
curl -N localhost:8080/events
curl localhost:8080/messages | jq .
```

## What this is not

- Not a daemon manager — bcd is responsible for spawning, monitoring, and
  restarting runner processes.
- Not multi-agent — one runner == one agent. Concurrency happens at the
  process level.
- Not a permission UI — `permission_mode` is passed through to the SDK; bcd
  is the policy authority.

## Status

Phase 1 of the SDK runtime migration. The runner is a standalone TypeScript
package; it is not yet wired into bcd. Phase 2 (`sdk` runtime backend in bcd)
will add a Go HTTP client that talks to one of these processes per agent.
