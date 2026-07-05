# Troubleshooting Guide

Common issues and solutions for mycel.

Start with the built-in health checks — they catch most problems automatically:

```bash
mycel doctor                 # Full health check (workspace, database, agents, tools, git)
mycel doctor check agents    # Check a specific category
mycel doctor fix             # Auto-fix fixable issues
mycel doctor fix --dry-run   # Preview fixes first
```

## Installation Issues

### "command not found: mycel"

**Cause**: mycel binary not in PATH.

**Solution**:
```bash
# Check if installed
ls $(go env GOPATH)/bin/mycel

# Add to PATH
export PATH=$PATH:$(go env GOPATH)/bin

# Or install to /usr/local/bin
sudo cp $(go env GOPATH)/bin/mycel /usr/local/bin/
```

### "tmux: command not found"

**Cause**: tmux is required for the tmux runtime but not installed.

**Solution**:
```bash
# macOS
brew install tmux

# Ubuntu/Debian
sudo apt install tmux

# Fedora
sudo dnf install tmux
```

### Build Fails with Go Errors

**Cause**: Go version too old or missing dependencies.

**Solution**:
```bash
# Check Go version (need 1.25.4+)
go version

# Rebuild
make build-local-mycel

# Clean build
make clean && make build-local-mycel
```

## Repo & Configuration Issues

### "not in a mycel workspace"

**Cause**: Running a repo-scoped mycel command outside a git repo that mycel knows about, or `MYCEL_WORKSPACE` not set.

**Solution**:
```bash
# Start the server from your repo (or add the repo in the web UI)
mycel up

# Or point at the repo explicitly (for agents in worktrees)
export MYCEL_WORKSPACE=/path/to/repo
```

### Config File Errors

**Cause**: Invalid JSON in `preferences.json` (config lives at
`~/.mycel/workspaces/<id>/preferences.json`).

**Solution**:
```bash
# Validate the configuration file
mycel config validate

# Inspect current configuration
mycel config show

# Open in your editor to fix syntax
mycel config edit

# Or regenerate the default config (WARNING: loses customizations)
mycel config reset
```

## Agent Issues

### Agent Won't Start

**Causes**:
1. tmux session already exists
2. Git worktree creation failed
3. AI tool not configured

**Solutions**:
```bash
# Check for existing session (sessions are prefixed mycel-)
tmux list-sessions | grep mycel-

# Kill stale session
tmux kill-session -t <session-name>

# Check worktrees (run from the agent's repo)
git worktree list

# Remove the corrupted worktree (path from `git worktree list`)
git worktree remove <worktree-path> --force
git worktree prune

# Restart the agent
mycel agent start <name>
```

### Agent Not Responding

**Cause**: Agent process hung or crashed.

**Solution**:
```bash
# Check agent health
mycel agent health <name>

# View recent output
mycel agent peek <name>

# Attach to investigate
mycel agent attach <name>

# Force restart
mycel agent stop <name>
mycel agent start <name>
```

### "MYCEL_AGENT_ID not set"

**Cause**: Running an agent-only command outside an agent session.
`mycel agent report` must be run from within an agent session, where
`MYCEL_AGENT_ID` is set automatically.

**Solution**:
```bash
# Runs inside an agent session:
mycel agent report working "implementing feature"

# From outside, ask the agent to report instead:
mycel agent send eng-01 "report your current status"
```

## Notification Issues

### Notifications Not Delivered

**Cause**: Target agent not running, gateway disconnected, or subscription missing.

**Solution**:

```bash
# Check agent status
mycel status

# Check gateway connection health
mycel notify status

# Verify subscriptions
mycel notify list

# Check the delivery activity log
mycel notify activity slack:engineering --limit 10
```

### Gateway Disconnected

**Cause**: Invalid credentials, network issue, or platform revoked access.

**Solution**:

```bash
# Check gateway status for error details
mycel notify status

# Verify credentials are present (names only, values stay encrypted)
mycel secret list

# Restart the server to reconnect gateways
mycel down && mycel up -d
```

### Database Locked

**Cause**: Multiple processes accessing SQLite simultaneously.

**Solution**:
```bash
# Check for running mycel processes
pgrep -f mycel

# Wait a moment, then retry
```

## TUI Issues

### TUI Won't Start

The TUI opens when you run `mycel` with no arguments and a server is reachable.

**Causes**:
1. Node.js or Bun not installed
2. Terminal too small

**Solutions**:
```bash
# Check for a JS runtime
node --version
bun --version

# Check terminal size (need at least 80x24)
echo "Columns: $COLUMNS, Lines: $LINES"
```

### Display Garbled

**Cause**: Terminal encoding or color issues.

**Solution**:
```bash
# Set proper terminal
export TERM=xterm-256color

# Disable colors if needed
NO_COLOR=1 mycel
```

### Keyboard Shortcuts Not Working

**Cause**: Terminal capturing keys before mycel.

**Solution**:
- Check if running inside tmux (prefix key conflicts)
- Try a different terminal emulator
- Check the keybinding help inside the TUI

## Git/Worktree Issues

### "worktree already exists"

**Cause**: Stale worktree from a crashed agent.

**Solution**:
```bash
# List worktrees
git worktree list

# Remove the stale worktree (path from `git worktree list`)
git worktree remove <worktree-path> --force

# Prune worktree references
git worktree prune

# Or let mycel fix it
mycel doctor fix --category git
```

### "cannot create worktree"

**Cause**: Not in a git repository or branch issues.

**Solution**:
```bash
# Ensure you're in a git repo
git status

# Initialize if needed
git init

# Check for branch issues
git branch -a
```

### Merge Conflicts in Worktree

**Cause**: Agent's branch has conflicts with main.

**Solution**:
```bash
# Attach to the agent
mycel agent attach <name>

# Inside the agent session, resolve conflicts
git status
git merge --abort  # or resolve manually
```

## Cost Tracking Issues

### Costs Not Recording

**Cause**: Cost data missing or database issue.

**Solution**:
```bash
# Check cost data
mycel cost show

# View per-agent breakdown
mycel cost agent

# Run the database health check
mycel doctor check database
```

## Performance Issues

### Commands Slow

**Cause**: Large database.

**Solution**:
```bash
# Check database sizes
du -sh ~/.mycel/*.db

# Vacuum the database
sqlite3 ~/.mycel/mycel.db "VACUUM;"
```

### High CPU Usage

**Cause**: Runaway agent or infinite loop.

**Solution**:
```bash
# Check agent health
mycel agent health

# View recent activity
mycel logs --tail 50

# Stop the problematic agent
mycel agent stop <name>
```

## Common Error Messages

### "permission denied"

**Cause**: File permission issues.

**Solution**:
```bash
# Fix ~/.mycel directory permissions
chmod -R u+rw ~/.mycel/

# Check for root-owned files
ls -la ~/.mycel/
sudo chown -R $USER:$USER ~/.mycel/
```

### "bcd is not running"

**Cause**: A command needs the mycel server, but it isn't running.

**Solution**:
```bash
# Start the server as a background daemon
mycel up -d

# Verify it's healthy
mycel doctor
```

### "connection refused" (MCP)

**Cause**: An MCP server is misconfigured or unreachable.

**Solution**:
```bash
# List configured MCP servers
mycel mcp list

# Inspect a server's configuration
mycel mcp show <name>

# Disable or remove a broken server
mycel mcp disable <name>
mycel mcp remove <name>
```

### "timeout waiting for response"

**Cause**: Agent taking too long or stuck.

**Solution**:
```bash
# Check if the agent is stuck
mycel agent peek <name>

# Run stuck detection with a longer work timeout
mycel agent health <name> --detect-stuck --work-timeout 1h
```

## Getting Help

### Logs

```bash
# View recent events
mycel logs

# Filter by agent
mycel logs --agent eng-01

# Filter by type
mycel logs --type agent.report

# Last N events
mycel logs --tail 100
```

### Debug Mode

```bash
# Enable verbose output on any command
mycel -v status
```

### Reporting Issues

When reporting issues, include:

1. mycel version: `mycel version`
2. OS and version
3. tmux version: `tmux -V`
4. Health check output: `mycel doctor`
5. Relevant logs: `mycel logs --tail 100`
6. Config (without secrets): `mycel config show`

File issues at: https://github.com/rpuneet/mycel/issues
