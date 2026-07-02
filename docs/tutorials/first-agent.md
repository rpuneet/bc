# Tutorial: Create Your First Agent

This tutorial walks you through creating, running, and communicating with your first AI agent in bc.

## Prerequisites

- A mycel workspace initialized (`mycel init` completed)
- The root agent running (`mycel up`)
- An AI provider configured (e.g., Claude Code or Gemini)

## Step 1: Create an agent

Create an engineer agent named `eng-01`:

```bash
mycel agent create eng-01 --role engineer
```

This creates:
- A git worktree at `.bc/agents/eng-01/worktree/`
- A tmux session (or Docker container) for the agent
- Role-specific configuration files (CLAUDE.md, .mcp.json)

## Step 2: Verify the agent is running

```bash
mycel status
```

You should see `eng-01` listed with state `idle`:

```
AGENT    ROLE      STATE   UPTIME   TASK
root     root      idle    5m
eng-01   engineer  idle    10s
```

## Step 3: Send work to the agent

```bash
mycel agent send eng-01 "Add a health check endpoint that returns JSON with status and uptime"
```

The agent receives the message in its tmux session and begins working.

## Step 4: Monitor progress

Watch the agent's output in real time:

```bash
mycel agent peek eng-01
```

Or attach directly to the agent's tmux session:

```bash
mycel agent attach eng-01
```

Press `Ctrl+B` then `D` to detach from the session without stopping the agent.

## Step 5: Subscribe to notifications

Subscribe the agent to a notification source so it receives platform events:

```bash
# List available notification sources
bc notify list

# Subscribe eng-01 to a Slack channel
bc notify subscribe slack:engineering eng-01

# Check recent notifications
bc notify history slack:engineering --last 10
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
mycel agent stop eng-01
```

To clean up entirely (removes worktree and state):

```bash
mycel agent delete eng-01
```

## Next steps

- Learn how to [configure your workspace](../how-to/configure-workspace.md) with providers, runtime backends, and polling settings
- Set up [notifications from external platforms](../how-to/set-up-notifications.md)
- Read about the [agent lifecycle and state machine](../explanation/agents.md)
- Browse the [CLI reference](../reference/cli/bc.md)
