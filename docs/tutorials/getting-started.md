# Quick Start Guide

Get bc running in 5 minutes.

## Prerequisites

- **Go 1.25.4+** for building from source
- **tmux** for agent session management
- **An AI agent tool** (Claude Code, Cursor, or similar)

## Installation

### From Source

```bash
git clone https://github.com/rpuneet/mycel.git
cd bc
make build
make install  # Installs to $GOPATH/bin
```

### Verify Installation

```bash
bc version
```

## Your First Workspace

### Step 1: Initialize

Navigate to your project directory and run:

```bash
cd your-project
bc init
```

Or use the quick-start wizard:

```bash
bc init --quick
```

This creates a `.bc/` directory with:
- `settings.json` - Workspace configuration
- `agents/` - Agent state storage
- `roles/` - Role definitions

### Step 2: Start the Root Agent

```bash
bc up
```

This spawns the root agent in a tmux session. The root agent can create and manage other agents.

### Step 3: Check Status

```bash
bc status
```

Output:
```
Workspace: my-project | Agents: 1 | Active: 1 | Working: 1

AGENT   ROLE   STATE     UPTIME   TASK
root    root   working   10s      Initializing...
```

### Step 4: Create an Engineer

```bash
bc agent create eng-01 --role engineer
```

### Step 5: Send Work

```bash
bc agent send eng-01 "Implement the login feature per issue #42"
```

### Step 6: Monitor Progress

Open the TUI dashboard:

```bash
bc home
```

Or check specific agent output:

```bash
bc agent peek eng-01
```

### Step 7: Stop When Done

```bash
bc down
```

## Common Workflows

### Notification Subscriptions

```bash
# List available notification sources
bc notify list

# Subscribe an agent to a Slack channel
bc notify subscribe slack:engineering eng-01

# Check recent notifications
bc notify history slack:engineering --last 10
```

### Agent Reporting

Agents report their status:

```bash
bc agent report working "Implementing login API"
bc agent report done "Feature complete"
bc agent report stuck "Need database access"
```

### Cost Tracking

```bash
bc cost show           # Recent spending
bc cost summary        # Aggregate stats
bc cost dashboard      # Full view
```

## Next Steps

- Learn about the [Architecture](../overview.md) to understand the system
- Configure [Settings](../how-to/configure-workspace.md) for your workspace
- Set up [Notifications](../how-to/set-up-notifications.md) for platform event routing
- Explore the [REST API](../reference/api-rest.md) for programmatic access

## Troubleshooting

If you encounter issues:

1. Check `bc logs` for recent events
2. Verify tmux is installed: `tmux -V`
3. Ensure your AI tool is configured correctly
4. See [Troubleshooting Guide](../how-to/troubleshoot.md) for common issues
