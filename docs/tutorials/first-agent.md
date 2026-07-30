# Tutorial: Create Your First Agent

This tutorial walks you through creating, running, and communicating with your first AI agent in mycel.

## Prerequisites

- The mycel server running (`mycel up -d` from your repo)
- An AI provider configured (e.g., Claude Code or Gemini)

## Step 1: Create an agent

Create an engineer agent named `eng-01`:

```bash
mycel agent create eng-01 --template engineer
```

This creates:
- A git worktree under `~/.mycel/`, checked out from the agent's repo
- A Docker container (or tmux session) for the agent
- Role-specific configuration files (CLAUDE.md, .mcp.json)

## Step 2: Verify the agent is running

```bash
mycel status
```

You should see `eng-01` listed with state `idle`:

```
AGENT     ROLE      STATE    UPTIME    TASK
eng-01    engineer  idle     10s       -
```

## Step 3: Send work to the agent

```bash
mycel agent send eng-01 "Add a health check endpoint that returns JSON with status and uptime"
```

The agent receives the message in its session and begins working.

## Step 4: Monitor progress

Watch the agent's recent output:

```bash
mycel agent peek eng-01
```

Or attach directly to the agent's tmux session:

```bash
mycel agent attach eng-01
```

Press `Ctrl+B` then `D` to detach from the session without stopping the agent.

## Step 5: Subscribe to notifications

Subscribe the agent to a notification channel so it receives platform events:

```bash
# Show gateway connection status
mycel notify status

# List all agent subscriptions
mycel notify list

# Subscribe eng-01 to a Slack channel
mycel notify subscribe slack:engineering eng-01

# Show recent delivery activity
mycel notify activity slack:engineering --limit 10
```

## Step 6: Check costs

After the agent has been working for a while, check spending:

```bash
mycel cost show
mycel cost agent
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

- Learn how to [configure mycel](../how-to/configure.md) with providers, runtime backends, and polling settings
- Set up [notifications from external platforms](../how-to/set-up-apps.md)
- Read about the [agent lifecycle and state machine](../explanation/agents.md)
- Browse the [CLI reference](../reference/cli/mycel.md)
