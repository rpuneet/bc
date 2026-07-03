# Quick Start Guide

Get mycel running in 5 minutes.

## Prerequisites

- **tmux** for agent session management (or Docker for the container runtime)
- **An AI agent tool** (Claude Code, Cursor, or similar)
- **Go 1.25.4+** only if building from source

## Installation

### Install Script (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash
```

### Homebrew

```bash
brew install rpuneet/mycel/mycel
```

### Go Install

```bash
go install github.com/rpuneet/mycel/cmd/mycel@latest
```

### From Source

```bash
git clone https://github.com/rpuneet/mycel.git
cd mycel
make build-local-mycel
make install  # Installs to $GOPATH/bin
```

### Verify Installation

```bash
mycel version
```

## Your First Workspace

### Step 1: Initialize

Navigate to your project directory and run:

```bash
cd your-project
mycel init
```

This launches an interactive wizard. To skip it and use defaults:

```bash
mycel init --quick
```

Workspace configuration lives outside your project directory, at
`~/.mycel/workspaces/<id>/`:

- `preferences.json` — workspace configuration
- `roles/` — agent role definitions
- `agents/` — per-agent state files

Agent working copies (git worktrees) are created inside your project under
`.bc/agents/<name>/`.

### Step 2: Start the Server

```bash
mycel up -d
```

This starts the mycel server (API, web UI, MCP, agent management) as a
background daemon on `127.0.0.1:9374`. Run `mycel up` without `-d` to keep it
in the foreground instead.

### Step 3: Create an Engineer

```bash
mycel agent create eng-01 --role engineer
```

### Step 4: Check Status

```bash
mycel status
```

```
AGENT     ROLE      STATE    UPTIME    TASK
eng-01    engineer  idle     10s       -
```

### Step 5: Send Work

```bash
mycel agent send eng-01 "Implement the login feature per issue #42"
```

### Step 6: Monitor Progress

Run `mycel` with no arguments to open the TUI dashboard:

```bash
mycel
```

Or check a specific agent's recent output:

```bash
mycel agent peek eng-01
```

### Step 7: Stop When Done

```bash
mycel down
```

## Common Workflows

### Notification Subscriptions

```bash
# Show gateway connection status and subscriptions
mycel notify status

# List all agent subscriptions
mycel notify list

# Subscribe an agent to a Slack channel
mycel notify subscribe slack:engineering eng-01

# Show recent delivery activity for a channel
mycel notify activity slack:engineering --limit 10
```

### Agent Reporting

Agents report their own state from inside their sessions:

```bash
mycel agent report working "Implementing login API"
mycel agent report done "Feature complete"
mycel agent report stuck "Need database access"
```

Valid states are `idle`, `working`, `done`, `stuck`, and `error`.

### Cost Tracking

```bash
mycel cost show           # Recent cost records
mycel cost summary        # Workspace cost overview
mycel cost dashboard      # Rich cost dashboard
```

## Next Steps

- Learn about the [Architecture](../overview.md) to understand the system
- Configure [Settings](../how-to/configure-workspace.md) for your workspace
- Set up [Notifications](../how-to/set-up-notifications.md) for platform event routing
- Explore the [REST API](../reference/api-rest.md) for programmatic access

## Troubleshooting

If you encounter issues:

1. Run `mycel doctor` for a full health check
2. Check `mycel logs` for recent events
3. Verify tmux is installed: `tmux -V`
4. Ensure your AI tool is configured correctly
5. See the [Troubleshooting Guide](../how-to/troubleshoot.md) for common issues
