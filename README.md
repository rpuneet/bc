# mycel

[![CI](https://github.com/rpuneet/mycel/actions/workflows/ci.yml/badge.svg)](https://github.com/rpuneet/mycel/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/rpuneet/mycel?include_prereleases)](https://github.com/rpuneet/mycel/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/rpuneet/mycel)](https://go.dev/)
[![License](https://img.shields.io/github/license/rpuneet/mycel)](LICENSE)

AI agents are powerful alone — chaotic when they work together. mycel fixes that.

Coordinate teams of Claude, Gemini, Cursor, and other AI agents with isolated worktrees, shared channels, and cost controls. One binary. No login. MIT licensed.

## Install

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash

# Homebrew
brew install rpuneet/mycel/mycel

# Go
go install github.com/rpuneet/mycel/cmd/mycel@latest

# From source
git clone https://github.com/rpuneet/mycel && cd mycel && make install-local-mycel
```

**Prerequisites:** Go 1.25+, tmux, git. For TUI: Bun.

## Quick Start

```bash
mycel up                      # Start server + web UI on localhost:9374 (bootstraps the workspace)
mycel agent create eng-01 \
  --role engineer \
  --tool claude               # Spawn an agent
mycel status                  # See what's running
mycel agent peek eng-01       # Watch agent output
mycel cost show               # Check spending
```

Open **http://localhost:9374** for the web dashboard.

## What mycel Does

**Without mycel:** One agent at a time. Context lost between sessions. Merge conflicts from parallel edits. No visibility. Surprise bills.

**With mycel:** Multiple agents in parallel. Each gets its own git worktree — zero conflicts. Persistent channels for structured communication. Per-agent budgets with hard stops. Everything observable in real time.

### How It Works

1. **Isolated by design** — Each agent runs in its own tmux session (or Docker container) with a dedicated git worktree. They can't step on each other's work.

2. **Structured communication** — Agents talk through persistent channels with mentions, reviews, and handoffs. Not through you.

3. **Full visibility** — Costs, activity, resource usage — all in real time through CLI, TUI, or web dashboard.

## Supported Agents

| Agent | Status |
|-------|--------|
| [Claude Code](https://claude.ai/code) | Fully supported |
| [Gemini CLI](https://github.com/google-gemini/gemini-cli) | Fully supported |
| [Cursor](https://cursor.com) | Supported |
| [Codex](https://github.com/openai/codex) | Supported |
| Custom | Any CLI tool via provider config |

## Commands

### Core

| Command | Description |
|---------|-------------|
| `mycel` | Open TUI dashboard |
| `mycel up` | Start server (foreground); bootstraps the workspace |
| `mycel up -d` | Start server (daemon) |
| `mycel down` | Stop server |
| `mycel status` | Show agent status |
| `mycel doctor` | Diagnose workspace issues |

### Agents

| Command | Description |
|---------|-------------|
| `mycel agent create [name]` | Create agent with role and tool |
| `mycel agent list` | List all agents |
| `mycel agent attach <agent>` | Attach to agent's tmux session |
| `mycel agent peek <agent>` | Watch agent output |
| `mycel agent send <agent> <msg>` | Send message to agent |
| `mycel agent stop <agent>` | Stop agent |
| `mycel agent delete <agent>` | Delete agent |

### Channels

| Command | Description |
|---------|-------------|
| `mycel channel list` | List channels |
| `mycel channel create <name>` | Create channel |
| `mycel channel send <ch> <msg>` | Send message |
| `mycel channel history <ch>` | View history |

### Cost & Scheduling

| Command | Description |
|---------|-------------|
| `mycel cost show` | Show cost records |
| `mycel cost budget show` | Budget status |
| `mycel cron add <name>` | Schedule recurring task |
| `mycel cron list` | List scheduled tasks |

### Configuration

| Command | Description |
|---------|-------------|
| `mycel config show` | Show configuration |
| `mycel config set <key> <val>` | Set config value |
| `mycel secret set <name>` | Store encrypted secret |
| `mycel tool list` | List available tools |
| `mycel mcp list` | List MCP servers |
| `mycel role list` | List agent roles |

Full reference: `mycel --help` or `mycel <command> --help`

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  mycel binary                    │
│                                                  │
│  ┌──────────┐  ┌──────────┐  ┌───────────────┐  │
│  │   CLI    │  │   TUI    │  │  HTTP Server  │  │
│  │ (Cobra)  │  │ (Ink/    │  │  + Web UI     │  │
│  │          │  │  React)  │  │  + MCP + SSE  │  │
│  └────┬─────┘  └────┬─────┘  └───────┬───────┘  │
│       │              │                │          │
│  ┌────┴──────────────┴────────────────┴───────┐  │
│  │              pkg/ (Go packages)            │  │
│  │  agent · channel · cost · cron · mcp       │  │
│  │  provider · runtime · secret · workspace   │  │
│  └────────────────────┬───────────────────────┘  │
│                       │                          │
│  ┌────────────────────┴───────────────────────┐  │
│  │           Agent Runtimes                   │  │
│  │     tmux sessions │ Docker containers      │  │
│  │     git worktrees │ isolated .bc/ state    │  │
│  └────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
                        │
            ┌───────────┼───────────┐
            │           │           │
         Claude      Gemini     Cursor ...
```

- **Single binary** — CLI, server, web UI, MCP all in one
- **SQLite storage** — zero external dependencies
- **Agent isolation** — each agent gets its own git worktree and tmux/Docker session
- **Gateway integrations** — Slack, Telegram, Discord for notifications

## Configuration

Workspace config lives in `.bc/settings.json`:

```json
{
  "version": "2",
  "server": {
    "host": "127.0.0.1",
    "port": 9374
  },
  "providers": {
    "default": "claude"
  },
  "runtime": {
    "default": "tmux"
  },
  "storage": {
    "default": "sqlite"
  }
}
```

## Development

```bash
make build            # Build everything
make test             # Run all tests
make lint             # Run linters
make check            # Full quality gate
make run-mycel        # Run from source
make run-web          # Web UI dev server (hot reload)
```

See `make help` for all targets.

## Contributing

Contributions welcome. Please run `make check` before submitting PRs.

## License

[MIT](LICENSE)
