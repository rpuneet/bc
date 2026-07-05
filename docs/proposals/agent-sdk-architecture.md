# bc + Claude Agent SDK — Architecture Design

## Problem

bc currently orchestrates Claude Code agents via:
- Docker containers with tmux sessions
- `claude --dangerously-skip-permissions` CLI inside containers
- File writes for config (CLAUDE.md, .mcp.json, .claude/settings.json, rules/, commands/)
- Terminal output parsing (regex) for state detection
- OAuth account login with volume mounts for auth persistence
- docker exec / tmux send-keys for MCP setup

This causes: MCP identity bugs, terminal parsing failures, stuck detection heuristics, file sync race conditions, OAuth session management complexity.

## Solution

Replace the tmux + claude CLI layer with the Claude Agent SDK. Keep Docker for isolation. Gain structured events, programmatic control, and eliminate file-based config.

---

## Where Agents Run

### Current architecture

```
Host machine
├── /workspace (project root)
│   └── .bc/agents/<name>/
│       ├── worktree/          ← git worktree (agent's isolated copy)
│       ├── claude/            ← mounted as /home/agent/.claude (OAuth state)
│       └── claude.json        ← OAuth tokens
│
└── Docker container (bc-agent-claude)
    ├── /workspace              ← bind mount of worktree (read-write)
    ├── /repo                   ← bind mount of project root (read-only)
    ├── /home/agent/.claude     ← bind mount for OAuth persistence
    ├── /home/agent/.claude.json ← bind mount for OAuth tokens
    └── tmux session
        └── claude --dangerously-skip-permissions
```

### With SDK — Option A: SDK inside Docker (recommended)

```
Host machine
├── /workspace
│   └── .bc/agents/<name>/worktree/    ← same as today
│
└── Docker container (bc-agent-sdk)
    ├── /workspace                      ← same worktree bind mount
    ├── Node.js process                 ← SDK runner (replaces tmux + claude CLI)
    │   ├── ClaudeAgent(systemPrompt=..., mcpServers=...)
    │   └── HTTP server on :8080        ← bcd talks to this
    └── No tmux, no OAuth mounts
```

- Worktrees: same as today
- Volume mounts: fewer (no OAuth mounts needed)
- Difference: thin SDK wrapper process replaces tmux + claude CLI
- bcd communicates via HTTP instead of terminal scraping
- Container image needs Node.js + SDK instead of claude CLI binary

### With SDK — Option B: SDK in bcd (no Docker)

```
Host machine
├── bcd process (Go) on :9374
│   ├── For each agent:
│   │   ├── Spawn SDK subprocess (node/python)
│   │   │   └── ClaudeAgent(workingDir=worktree, ...)
│   │   └── Communicate via stdin/stdout or HTTP
│   └── Web dashboard
│
├── /workspace/.bc/agents/<name>/worktree/  ← same worktrees
```

- No Docker — agents are SDK processes on host
- Worktrees still provide filesystem isolation
- Simpler but less sandboxed

### With SDK — Option C: Hybrid (best of both)

```
Host machine
├── bcd (Go) on :9374
│
├── Docker runtime:
│   └── Docker container per agent
│       ├── /workspace              ← worktree mount
│       ├── sdk-runner              ← thin Node.js wrapper
│       │   ├── ClaudeAgent(...)
│       │   ├── HTTP :8080          ← bcd calls this
│       │   └── SSE stream          ← live events to bcd
│       └── No tmux, no claude CLI
│
├── Localhost runtime:
│   └── sdk-runner process          ← same wrapper, no container
│       └── ClaudeAgent(workingDir=worktree)
```

---

## Auth Model Change: OAuth → API Key

### Current (Claude CLI)
- Uses OAuth — agents need a Claude.ai account login
- Requires mounting ~/.claude/ and claude.json for session persistence
- Agents share the account's rate limits (Pro/Team subscription)
- Costs through Claude subscription billing
- Plugins (GitHub, etc.) tied to OAuth account

### With SDK
- Uses ANTHROPIC_API_KEY — no OAuth, no account login
- No ~/.claude/ mount needed — no auth state to persist
- Rate limits from API plan, not account
- Costs through API billing (pay-per-token)
- No Claude.ai plugins — MCP servers replace them:
  - GitHub plugin → @modelcontextprotocol/server-github MCP
  - Web search → built-in WebSearch tool in SDK
  - File operations → built-in Read/Write/Edit tools

### Volume mounts comparison

| Volume mount | Current | With SDK |
|---|---|---|
| `-v worktree:/workspace` | Yes | Yes — agent still needs code access |
| `-v agent-dir/.claude:/home/agent/.claude` | Yes | No — no OAuth state |
| `-v claude.json:/home/agent/.claude.json` | Yes | No — no OAuth tokens |
| `-v bc-shared-tmp:/tmp/bc-shared` | Yes | Yes — screenshot sharing |

### Cost comparison

| | Claude CLI (current) | Claude Agent SDK |
|---|---|---|
| Auth | OAuth (Claude Pro/Team) | API key |
| Billing | Subscription ($20-100/mo/seat) | Pay-per-token |
| Input tokens | Included in sub | $3/M (Sonnet), $15/M (Opus) |
| Output tokens | Included in sub | $15/M (Sonnet), $75/M (Opus) |
| Rate limits | Account tier | API tier (higher with scale) |
| Caching | Automatic | 90% discount on cached prompts |
| Best for | Few agents, lots of usage | Many agents, controlled usage |

---

## What Changes vs What Stays

### Stays the same

| Component | Changes? | Why |
|---|---|---|
| Git worktrees | No | Still need filesystem isolation per agent |
| Volume mounts (worktree) | No | Agent still needs access to its code |
| .bc/ directory | No | State, config, secrets stay on host |
| bcd server | No | Still the orchestrator, dashboard, API |
| Channels/messaging | No | SQLite-backed, bcd serves them |
| Roles | No | Still define prompt, tools, rules — just passed as SDK params |
| Cost tracking | Simpler | API gives exact per-request costs via hooks |

### Goes away

| Component | Why |
|---|---|
| tmux sessions | SDK manages the agent loop natively |
| Terminal output parsing | Structured events via hooks replace regex |
| .mcp.json file writes | MCP servers passed as SDK config |
| CLAUDE.md file writes | System prompt passed as parameter |
| .claude/settings.json | Permissions/hooks configured in SDK options |
| .claude/commands/*.md | Custom tools defined as @tool functions |
| .claude/rules/*.md | Rules passed in system prompt or SDK config |
| claude mcp add commands | Not needed — SDK configures MCP directly |
| State detection regex | Hooks give exact state transitions |
| --dangerously-skip-permissions | permissionMode: "none" in SDK |
| OAuth mount volumes | API key auth, no OAuth state |
| claude login / plugin auth | API key only |

---

## Capability Comparison

| What bc does now | Current method | With Claude Agent SDK |
|---|---|---|
| Start agent | docker run + tmux + claude CLI | sdk.query(prompt, options) |
| Set prompt/role | Write CLAUDE.md file | Pass systemPrompt parameter |
| Add MCP servers | Write .mcp.json or claude mcp add | Pass mcpServers in options |
| Add custom tools | Write .claude/commands/*.md | Define @tool functions inline |
| Agent attach (view) | tmux attach -t session | session.receiveMessages() stream |
| Resume session | claude --resume uuid | sdk.query(prompt, { resume: sessionId }) |
| Subagents | Claude spawns via Agent tool | Define subagents inline with agents option |
| Monitor state | Parse tmux output for symbols | Hook into PostToolUse, Stop, Notification |
| Permissions | --dangerously-skip-permissions | permissionMode: "none" or custom hooks |
| File safety | Hope for the best | fileCheckpointing: true — auto backups + rollback |

---

## New Capabilities Unlocked

### 1. Structured event streaming
Instead of parsing terminal output for spinner symbols, get typed events:
- PreToolUse — before each tool call
- PostToolUse — after each tool call (with result)
- Stop — agent finished
- Notification — agent wants attention
- SubagentStart/Stop — subagent lifecycle
- PermissionRequest — tool needs approval

### 2. In-process MCP servers
Define bc's tools as @tool functions — no separate MCP server process:

```python
@tool
def send_message(channel: str, message: str) -> str:
    """Send a message to a bc channel"""
    requests.post(f"{BCD_URL}/api/channels/{channel}/messages", json={"message": message})
    return f"Sent to #{channel}"
```

### 3. Session forking
Branch a conversation to explore different approaches from the same point.
Could enable "try two solutions in parallel" for complex tasks.

### 4. Custom permission hooks
Intercept each tool call and approve/deny programmatically:

```python
def permission_hook(event):
    if event.tool == "Bash" and "rm -rf" in event.input:
        return "deny"
    if agent.role != "root" and event.tool == "Write" and "/workspace/" in event.input:
        return "deny"  # non-root can't write to project root
    return "allow"
```

### 5. File checkpointing
Automatic file backups before every edit. Rollback to any point:

```python
agent = ClaudeAgent(fileCheckpointing=True)
# Later, if agent broke something:
agent.restoreCheckpoint(checkpoint_id)
```

### 6. Native cost tracking
Every API call returns token usage. No JSONL parsing needed:

```python
hooks={
    "PostToolUse": lambda e: cost_store.record(
        agent=name, tokens=e.usage, cost=e.cost
    )
}
```

---

## The SDK Runner — New Component

A thin wrapper (~100 lines) that runs inside the container (or on host for localhost runtime):

```typescript
// mycel-agent-runner/index.ts
import { ClaudeAgent } from '@anthropic-ai/claude-agent-sdk'
import express from 'express'

const app = express()
const AGENT_NAME = process.env.MYCEL_AGENT_NAME
const BCD_URL = process.env.MYCEL_DAEMON_ADDR

const agent = new ClaudeAgent({
  systemPrompt: process.env.MYCEL_ROLE_PROMPT,
  workingDir: '/workspace',
  mcpServers: JSON.parse(process.env.MYCEL_MCP_SERVERS || '[]'),
  allowedTools: JSON.parse(process.env.MYCEL_ALLOWED_TOOLS || '[]'),
  permissionMode: "none",
  fileCheckpointing: true,
  hooks: {
    PostToolUse: (event) => {
      // Stream activity to bcd dashboard
      fetch(`${BCD_URL}/api/events`, {
        method: 'POST',
        body: JSON.stringify({ agent: AGENT_NAME, type: 'tool_use', data: event })
      })
    },
    Stop: () => {
      fetch(`${BCD_URL}/api/agents/${AGENT_NAME}/state`, {
        method: 'PUT',
        body: JSON.stringify({ state: 'idle' })
      })
    },
    Notification: (event) => {
      fetch(`${BCD_URL}/api/agents/${AGENT_NAME}/state`, {
        method: 'PUT',
        body: JSON.stringify({ state: 'stuck', reason: event.message })
      })
    }
  }
})

// HTTP API for bcd to control this agent
app.use(express.json())

// Send a prompt to the agent
app.post('/query', async (req, res) => {
  res.setHeader('Content-Type', 'text/event-stream')
  for await (const event of agent.query(req.body.prompt)) {
    res.write(`data: ${JSON.stringify(event)}\n\n`)
  }
  res.end()
})

// Get agent status
app.get('/status', (req, res) => {
  res.json({ state: agent.isRunning ? 'working' : 'idle' })
})

// Stream messages (for "agent attach" equivalent)
app.get('/messages', async (req, res) => {
  res.setHeader('Content-Type', 'text/event-stream')
  for await (const msg of agent.receiveMessages()) {
    res.write(`data: ${JSON.stringify(msg)}\n\n`)
  }
})

// Resume a previous session
app.post('/resume', async (req, res) => {
  res.setHeader('Content-Type', 'text/event-stream')
  for await (const event of agent.query(req.body.prompt, { resume: req.body.sessionId })) {
    res.write(`data: ${JSON.stringify(event)}\n\n`)
  }
  res.end()
})

// Stop the agent
app.post('/stop', (req, res) => {
  agent.abort()
  res.json({ status: 'stopped' })
})

app.listen(8080, () => console.log(`mycel-agent-runner for ${AGENT_NAME} on :8080`))
```

### How bcd uses it

```go
// In pkg/container/container.go — CreateSessionWithEnv
// Instead of:
//   docker run ... bash -c "tmux new-session 'claude --dangerously-skip-permissions'"
// Now:
//   docker run ... node /app/mycel-agent-runner/index.js

// In pkg/agent/agent.go — sending work to agent
// Instead of:
//   tmux send-keys -t <session> "do this task" Enter
// Now:
//   POST http://<container>:8080/query {"prompt": "do this task"}

// In pkg/agent/agent.go — checking state
// Instead of:
//   tmux capture-pane → regex match for spinner/prompt
// Now:
//   GET http://<container>:8080/status → {"state": "working"}

// In internal/cmd/agent.go — agent attach
// Instead of:
//   tmux attach -t <session>
// Now:
//   GET http://<container>:8080/messages → SSE stream of conversation
```

---

## Docker Image Changes

### Current: bc-agent-claude
```dockerfile
FROM node:22-slim
RUN npm install -g @anthropic-ai/claude-code
# Claude CLI installed, needs OAuth login
```

### With SDK: bc-agent-sdk
```dockerfile
FROM node:22-slim
COPY mycel-agent-runner/ /app/mycel-agent-runner/
RUN cd /app/mycel-agent-runner && npm install
# SDK uses API key, no login needed
ENV ANTHROPIC_API_KEY=${secret:ANTHROPIC_API_KEY}
CMD ["node", "/app/mycel-agent-runner/index.js"]
```

- Smaller image (no claude CLI, no tmux needed inside container)
- No OAuth setup
- API key passed as env var from bc secret store

---

## Migration Path

### Phase 1: Build mycel-agent-runner (SMALL)
- Create the thin SDK wrapper as shown above
- Test locally with one agent
- Verify: query, status, messages, stop all work

### Phase 2: Add SDK runtime to bcd (MEDIUM)
- New runtime backend: "sdk" alongside "docker" and "tmux"
- bcd talks to agents via HTTP instead of terminal
- Agent state from /status endpoint instead of regex
- Activity events from hooks instead of output parsing

### Phase 3: Update role_setup for SDK (SMALL)
- Instead of writing files, pass config as env vars:
  - MYCEL_ROLE_PROMPT (system prompt)
  - MYCEL_MCP_SERVERS (JSON array of MCP configs)
  - MYCEL_ALLOWED_TOOLS (JSON array)
- ConfigAdapter already exists from #2852 — add SdkAdapter

### Phase 4: Dashboard integration (MEDIUM)
- /messages SSE stream replaces tmux capture-pane
- Structured events in activity tree (tool name, duration, result)
- Real-time cost per tool call
- "Agent attach" becomes web-based message viewer

### Phase 5: Remove tmux/CLI code paths (SMALL)
- Delete terminal parsing regex
- Delete tmux session management for SDK agents
- Delete OAuth volume mount logic
- Keep tmux for non-SDK providers (gemini, cursor, codex, pi)

---

## Risks and Considerations

### Language mismatch
- bc is Go, SDK is Python/TypeScript
- Mitigation: SDK runs as sidecar process, bcd communicates via HTTP
- No Go rewrite needed — just HTTP client calls

### Debugging / agent attach
- tmux attach gives raw terminal view — developers are used to this
- Mitigation: /messages endpoint gives same content, rendered in web UI
- Could still run tmux inside container as optional debug mode

### API costs
- Subscription → pay-per-token could be expensive for heavy agents
- Mitigation: model selection per role (cheap model for simple tasks)
- Built-in cost tracking via hooks enables budgets and alerts
- Prompt caching (90% discount) helps with repeated context

### Offline / air-gapped
- CLI can work with cached credentials; SDK needs live API access
- Mitigation: not relevant for bc's use case (always online)

### Provider lock-in
- SDK is Claude-specific
- Mitigation: ConfigAdapter abstraction (#2852) already handles this
- Other providers use their own SDKs (ADK for Gemini, Codex SDK for OpenAI)
- mycel-agent-runner would be Claude-specific; other providers get their own runners

---

## Summary

The Claude Agent SDK replaces the bottom layer of bc's stack:

```
BEFORE:  bcd → Docker → tmux → claude CLI → terminal output → regex
AFTER:   bcd → Docker → mycel-agent-runner → Claude SDK → typed events → HTTP
```

Same isolation (Docker + worktrees). Same orchestration (bcd). Same dashboard.
But: no file sync bugs, no terminal parsing, no OAuth headaches, structured events,
programmatic cost tracking, custom permissions, file checkpointing.

The migration is incremental — SDK runtime can coexist with tmux runtime.
Agents can be moved one at a time. No big bang required.
