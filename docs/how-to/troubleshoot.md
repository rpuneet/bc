# Troubleshooting Guide

Common issues and solutions for bc.

## Installation Issues

### "command not found: bc"

**Cause**: bc binary not in PATH.

**Solution**:
```bash
# Check if installed
ls $GOPATH/bin/bc

# Add to PATH
export PATH=$PATH:$GOPATH/bin

# Or install to /usr/local/bin
sudo cp $GOPATH/bin/bc /usr/local/bin/
```

### "tmux: command not found"

**Cause**: tmux is required but not installed.

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

# Update dependencies
make build

# Clean build
make clean build
```

## Workspace Issues

### "not in a bc workspace"

**Cause**: Running bc command outside a workspace, or BC_WORKSPACE not set.

**Solution**:
```bash
# Initialize workspace
bc init

# Or set environment variable (for agents in worktrees)
export BC_WORKSPACE=/path/to/workspace
```

### "workspace already initialized"

**Cause**: Trying to init in an existing workspace.

**Solution**:
```bash
# Check for existing .bc directory
ls -la .bc/

# Remove and reinitialize (WARNING: loses data)
rm -rf .bc/
bc init
```

### Config File Errors

**Cause**: Invalid TOML syntax in settings.json.

**Solution**:
```bash
# Validate TOML
bc config show

# Check for syntax errors
cat .bc/settings.json

# Regenerate default config
bc config reset
```

## Agent Issues

### Agent Won't Start

**Causes**:
1. tmux session already exists
2. Git worktree creation failed
3. AI tool not configured

**Solutions**:
```bash
# Check for existing session
tmux list-sessions | grep bc-

# Kill stale session
tmux kill-session -t bc-workspace-agent

# Check worktree
git worktree list

# Remove corrupted worktree
git worktree remove .bc/worktrees/agent-name --force

# Restart agent
bc agent start agent-name
```

### Agent Not Responding

**Cause**: Agent process hung or crashed.

**Solution**:
```bash
# Check agent status
bc agent health agent-name

# View recent output
bc agent peek agent-name

# Attach to investigate
bc agent attach agent-name

# Force restart
bc agent stop agent-name
bc agent start agent-name
```

### "BC_AGENT_ID not set"

**Cause**: Running agent-only command outside agent session.

**Solution**:
```bash
# These commands only work inside agent sessions:
bc agent report working "..."
# Use agent send instead:
bc agent send eng-01 "bc agent report working '...'"
```

## Notification Issues

### Notifications Not Delivered

**Cause**: Target agent not running, adapter disconnected, or subscription missing.

**Solution**:
```bash
# Check agent status
bc status

# Check adapter connection health
bc notify status

# Verify subscriptions
bc notify list

# Check delivery log
bc notify history slack:engineering --last 10
```

### Adapter Disconnected

**Cause**: Invalid credentials, network issue, or platform revoked access.

**Solution**:
```bash
# Check adapter status for error details
bc notify status

# Verify credentials
bc secret list

# Restart bcd to reconnect adapters
bc down && bc up
```

### Database Locked

**Cause**: Multiple processes accessing SQLite simultaneously.

**Solution**:
```bash
# Check for running bc processes
pgrep -f "bc "

# Wait a moment, then retry
```

## TUI Issues

### TUI Won't Start

**Causes**:
1. Node.js not installed
2. TUI not built
3. Terminal too small

**Solutions**:
```bash
# Check Node/Bun
node --version
bun --version

# Rebuild TUI
cd tui && bun install && bun run build

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
export NO_COLOR=1
bc home
```

### Keyboard Shortcuts Not Working

**Cause**: Terminal capturing keys before bc.

**Solution**:
- Check if running inside tmux (prefix key conflicts)
- Try different terminal emulator
- Check keybinding documentation in TUI

## Git/Worktree Issues

### "worktree already exists"

**Cause**: Stale worktree from crashed agent.

**Solution**:
```bash
# List worktrees
git worktree list

# Remove stale worktree
git worktree remove .bc/worktrees/agent-name --force

# Prune worktree references
git worktree prune
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
# Attach to agent
bc agent attach agent-name

# Inside agent session, resolve conflicts
git status
git merge --abort  # or resolve manually
```

## Cost Tracking Issues

### Costs Not Recording

**Cause**: Cost tracking disabled or database issue.

**Solution**:
```bash
# Check cost data
bc cost show

# View per-agent breakdown
bc cost agent

# Check database
sqlite3 .bc/bc.db "SELECT * FROM costs LIMIT 5;"
```

## Performance Issues

### bc Commands Slow

**Cause**: Large workspace or database.

**Solution**:
```bash
# Check database sizes
du -sh .bc/*.db

# Vacuum database
sqlite3 .bc/bc.db "VACUUM;"
```

### High CPU Usage

**Cause**: Runaway agent or infinite loop.

**Solution**:
```bash
# Check which agent
bc agent health

# View recent activity
bc logs --tail 50

# Stop problematic agent
bc agent stop agent-name
```

## Common Error Messages

### "permission denied"

**Cause**: File permission issues.

**Solution**:
```bash
# Fix .bc directory permissions
chmod -R u+rw .bc/

# Check for root-owned files
ls -la .bc/
sudo chown -R $USER:$USER .bc/
```

### "connection refused"

**Cause**: MCP server not running or wrong port.

**Solution**:
```bash
# Check MCP server status
bc mcp list

# Restart servers
bc mcp remove all
```

### "timeout waiting for response"

**Cause**: Agent or server taking too long.

**Solution**:
```bash
# Increase timeout in config
bc config set agent.timeout 120

# Check if agent is stuck
bc agent peek agent-name
```

## Getting Help

### Logs

```bash
# View recent events
bc logs

# Filter by agent
bc logs --agent eng-01

# Filter by type
bc logs --type agent.report
```

### Debug Mode

```bash
# Enable verbose output
bc -v status

# Enable debug logging
BC_DEBUG=1 bc agent start eng-01
```

### Reporting Issues

When reporting issues, include:

1. bc version: `bc version`
2. OS and version
3. Go version: `go version`
4. tmux version: `tmux -V`
5. Relevant logs: `bc logs --tail 100`
6. Config (without secrets): `bc config show`

File issues at: https://github.com/rpuneet/bc/issues
