# BC TUI User Guide

The BC Terminal User Interface (TUI) provides a rich, interactive way to monitor and manage your BC workspace directly from the terminal.

## Starting the TUI

```bash
bc tui
```

## Overview

The TUI is organized into tabs, each providing a different view of your workspace:

1. **Dashboard** - Overview of workspace status
2. **Agents** - List and details of all agents
3. **Channels** - Communication channels between agents
4. **Costs** - Token usage and cost tracking
5. **Demons** - Scheduled background tasks
6. **Processes** - Running processes and logs
7. **Teams** - Team organization and members

## Navigation

### Tab Navigation

- Press number keys `1-7` to jump directly to a tab
- Use `Tab` / `Shift+Tab` to cycle through tabs
- Press `q` to quit or go back

### List Navigation

Most views use vim-style navigation:

- `j` or `↓` - Move down
- `k` or `↑` - Move up
- `g` - Jump to first item
- `G` - Jump to last item
- `Enter` - Select/open item

## Views

### Dashboard

The dashboard shows a high-level overview:

- **Agent Stats** - Total, active, working, idle, stuck counts
- **Cost Summary** - Total spend, token breakdown
- **Recent Activity** - Latest workspace events

Quick actions:
- `a` - Go to Agents
- `c` - Go to Channels
- `$` - Go to Costs
- `r` - Refresh

### Agents View

Lists all agents in your workspace with:

- Name and role
- Current state (idle, working, stuck, done, error)
- Current task description

Press `Enter` on an agent to see details including:
- Full task description
- Session ID
- Working directory
- Memory status

### Channels View

Shows communication channels between agents.

Select a channel and press `Enter` to:
- View message history
- Send messages to the channel

#### Sending Messages

1. Press `i` to enter input mode
2. Type your message
3. Press `Enter` to send
4. Press `Escape` to exit input mode

#### @Mentions

Type `@` followed by an agent name to mention them:
- `@eng-01` - Mention specific agent
- `@all` or `@everyone` - Broadcast to all

Autocomplete suggestions appear as you type after `@`.

### Costs View

Tracks API usage and costs:

- Total cost in USD
- Input/output token counts
- Breakdown by agent, team, and model

### Processes View

Shows running processes with:

- Process name and status
- Start time and duration
- Exit codes

Select a process to view its logs.

### Teams View

Displays team organization:

- Team name and description
- Team lead
- Member list

Press `Enter` to expand/collapse team details.

## Auto-Refresh

Most views automatically refresh data:

- Dashboard: Every 10 seconds
- Agents: Every 5 seconds
- Channels: Every 3 seconds
- Other views: Every 5 seconds

Press `r` to manually refresh at any time.

## Tips

1. **Quick Navigation** - Use number keys for instant tab switching
2. **Vim Users** - j/k/g/G work as expected
3. **Help** - Press `?` for context help
4. **Mentions** - Use `@` in channels for quick agent mentions
5. **Refresh** - Press `r` if data seems stale

## Troubleshooting

### TUI Not Starting

Ensure bc is properly installed:
```bash
bc --version
```

### No Data Showing

Check that agents are running:
```bash
bc status
```

### Input Not Working

Some terminal emulators may have key conflicts. Try:
- Using a different terminal
- Checking terminal key bindings
- Running in a tmux/screen session

## Keyboard Reference

See [KEYBOARD.md](./KEYBOARD.md) for complete keyboard shortcuts.
