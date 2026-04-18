# Tutorial: Create Your First Agent

This tutorial walks you through creating, running, and communicating with your first AI agent in bc.

## Prerequisites

- A bc workspace initialized (`bc init` completed)
- The root agent running (`bc up`)
- An AI provider configured (e.g., Claude Code or Gemini)

## Step 1: Create an agent

Create an engineer agent named `eng-01`:

```bash
bc agent create eng-01 --role engineer
```

This creates:
- A git worktree at `.bc/agents/eng-01/worktree/`
- A tmux session (or Docker container) for the agent
- Role-specific configuration files (CLAUDE.md, .mcp.json)

## Step 2: Verify the agent is running

```bash
bc status
```

You should see `eng-01` listed with state `idle`:

```
AGENT    ROLE      STATE   UPTIME   TASK
root     root      idle    5m
eng-01   engineer  idle    10s
```

## Step 3: Send work to the agent

```bash
bc agent send eng-01 "Add a health check endpoint that returns JSON with status and uptime"
```

The agent receives the message in its tmux session and begins working.

## Step 4: Monitor progress

Watch the agent's output in real time:

```bash
bc agent peek eng-01
```

Or attach directly to the agent's tmux session:

```bash
bc agent attach eng-01
```

Press `Ctrl+B` then `D` to detach from the session without stopping the agent.

## Step 5: Set up a communication channel

Create a channel so agents can communicate:

```bash
bc channel create eng
bc channel send eng "eng-01 is working on the health check endpoint"
```

View channel history:

```bash
bc channel history eng
```

## Step 6: Check costs

After the agent has been working for a while, check spending:

```bash
bc cost show
bc cost agent
```

## Step 7: Stop the agent

When the work is complete:

```bash
bc agent stop eng-01
```

To clean up entirely (removes worktree and state):

```bash
bc agent delete eng-01
```

## Next steps

- Learn how to [configure your workspace](../how-to/configure-workspace.md) with providers, runtime backends, and polling settings
- Set up [channels for platform notifications](../architecture/channels.md)
- Read about the [agent lifecycle and state machine](../explanation/agents.md)
- Browse the [CLI reference](../reference/cli/bc.md)
