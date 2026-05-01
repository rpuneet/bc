# Getting Started with mycel

Welcome to **mycel** – the multi-agent orchestration system for coordinated software development. This guide will walk you through installation, setup, and your first workflow.

---

## Table of Contents

1. [Installation](#installation)
2. [Initial Setup](#initial-setup)
3. [Your First Workflow](#your-first-workflow)
4. [Common Commands](#common-commands)
5. [Troubleshooting](#troubleshooting)
6. [Next Steps](#next-steps)

---

## Installation

### macOS (Apple Silicon / Intel)

**Install script:**
```bash
curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash
```

**Using Homebrew:**
```bash
brew install rpuneet/mycel/mycel
```

### Linux

**Install script:**
```bash
curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash
```

### Docker

```bash
# Stable release
docker run -p 9374:9374 -v $(pwd):/workspace ghcr.io/rpuneet/mycel:latest mycel up --addr 0.0.0.0:9374

# Bleeding-edge (main branch)
docker run -p 9374:9374 -v $(pwd):/workspace ghcr.io/rpuneet/mycel:main mycel up --addr 0.0.0.0:9374
```

### npm / bun

```bash
npm install -g bc-cli
# or
bunx bc-cli
```

### Go

```bash
go install github.com/rpuneet/bc/cmd/bc@latest
```

### After Install

```bash
mycel init          # Initialize workspace
mycel up            # Start server
mycel up -d         # Start as daemon
```

---

## Initial Setup

### 1. Initialize Your Workspace

```bash
# Create a new project directory
mkdir my-project && cd my-project

# Initialize mycel workspace
mycel init

# Verify setup
mycel status
```

**Output:**
```
✓ Workspace initialized (.bc/)
✓ Config file created (.bc/config.yaml)
✓ Ready to create agents
```

### 2. Start the Root Agent

```bash
# Start root coordinator
mycel up

# Check status
mycel status
```

**Expected Output:**
```
ROOT AGENT: running
  • State: idle
  • Uptime: 0h 2m
```

### 3. Create Your First Agent

```bash
# Create a manager agent
mycel agent create manager-atlas --role manager --tool cursor

# Create an engineer agent
mycel agent create engineer-pixel --role engineer --tool claude

# List agents
mycel agent list
```

**Output:**
```
AGENTS (3):
  • root-prime (root, 100% uptime)
  • manager-atlas (manager, idle)
  • engineer-pixel (engineer, idle)
```

---

## Your First Workflow

### Scenario: Build a Feature with mycel

**Step 1: Create a Work Queue Task**

```bash
# Add task to work queue
mycel queue add "Implement user authentication feature" \
  --priority high \
  --epic "auth-v2"

# View queue
mycel queue work
```

**Output:**
```
WORK QUEUE:
  1. Implement user authentication (priority: high, epic: auth-v2)
```

**Step 2: Assign Work via Channels**

```bash
# Send task assignment to #engineering channel
mycel channel send engineering "@engineer-pixel: Take task #1 - user auth implementation. Use jwt tokens, verify against db. DM when ready."

# Check messages
mycel channel history engineering --limit 5
```

**Step 3: Engineer Works on Task**

```bash
# Simulate engineer picking up work
mycel agent peek engineer-pixel
```

**Output:**
```
AGENT: engineer-pixel (state: tool)
  ⏺ created branch: feat/user-auth
  ⏺ implementing jwt middleware
  ✽ pushing to remote
  ✻ running tests
```

**Step 4: Manager Reviews & Merges**

```bash
# Check manager's queue
mycel queue merge

# Merge PR #42
mycel merge feature/user-auth --branch main

# Verify merge
mycel queue merge
```

**Step 5: Celebrate!**

```bash
# Post status to channel
mycel channel send general "🎉 Task complete! User auth feature merged and live. Great work team!"
```

---

## Common Commands

### Agent Management

```bash
# View all agents
mycel agent list

# Peek at agent's current work
mycel agent peek engineer-pixel

# Send direct message to agent
mycel agent send engineer-pixel "Status update please?"

# Attach to agent's live session
mycel agent attach engineer-pixel
```

### Work Queue

```bash
# View incoming work
mycel queue work

# View merge queue
mycel queue merge

# Add task to queue
mycel queue add "Task description" --priority high

# Complete task
mycel queue complete 42
```

### Channels (Team Communication)

```bash
# List channels
mycel channel list

# Send message to channel
mycel channel send #general "Update: Feature X shipped 🚀"

# Check channel history
mycel channel history #engineering --limit 10

# Create new channel
mycel channel create #product-team
```

### Memory & Learning

```bash
# View agent memory
mycel memory show --agent engineer-pixel

# Record learning
mycel memory record "Pattern: Always validate user input before processing"

# Search past experiences
mycel memory search "authentication patterns"
```

### Automation (Demons)

```bash
# List scheduled tasks
mycel cron list

# Run a demon manually
mycel cron run nightly-tests

# Create new demon
mycel cron create test-suite --schedule "0 2 * * *" --role qa --task "Nightly test run"

# View demon logs
mycel cron logs test-suite
```

---

## Troubleshooting

### Issue: "Workspace not initialized"

**Solution:**
```bash
# Make sure you're in project directory
pwd

# If needed, reinitialize
mycel init

# Verify
mycel status
```

### Issue: Agent stuck or unresponsive

**Solution:**
```bash
# Check agent status
mycel agent list

# Restart agent
mycel agent restart engineer-pixel

# View agent logs
mycel agent logs engineer-pixel --tail 20
```

### Issue: Merge conflict preventing PR merge

**Solution:**
```bash
# Check merge queue for conflicts
mycel queue merge

# Agent should resolve automatically, if not:
# Contact tech lead for manual resolution

# Once resolved, retry merge
mycel merge feature/branch-name --branch main
```

### Issue: Channel messages not appearing

**Solution:**
```bash
# Verify channel exists
mycel channel list

# Verify you're sending to correct channel
mycel channel history #general

# Resend message
mycel channel send #general "Test message"
```

### Issue: Performance slow or timeouts

**Solution:**
```bash
# Check system resources
mycel status --verbose

# Clear cache
mycel cache clear

# Restart root agent
mycel down && mycel up
```

---

## Next Steps

### 1. Explore Documentation
- [API Reference](/docs/api)
- [CLI Reference](/docs/cli)
- [Architecture Guide](/docs/architecture)

### 2. Build Your Team Structure
- Create agents for your team roles (engineers, QA, product)
- Set up channels for team communication
- Define workflows and automation

### 3. Integrate with Tools
- Connect GitHub for PR management
- Link Jira for task tracking
- Setup Slack notifications

### 4. Advanced Features
- Custom agent behaviors
- Performance optimization
- Audit logging and compliance

### 5. Get Support
- GitHub Issues: [Report bugs](https://github.com/rpuneet/mycel/issues)
- Documentation: [Full docs](https://docs.mycel.dev)
- Community: [Discord server](https://discord.gg/mycel-dev)

---

## Example: Complete Workflow

Here's a realistic end-to-end workflow:

```bash
# 1. Initialize
mycel init && mycel up

# 2. Create team
mycel agent create pm-alex --role product-manager --tool notion
mycel agent create eng-sam --role engineer --tool cursor
mycel agent create qa-jamie --role qa --tool chrome

# 3. Define work
mycel queue add "Build user profile page" --priority high --epic "user-feature"

# 4. Assign via channel
mycel channel send #engineering "@eng-sam: Pick up user profile task. UI design in #product. Ship within 2 hours?"

# 5. Monitor progress
mycel agent peek eng-sam
mycel channel history #engineering

# 6. Approve & merge
mycel queue merge
mycel merge feature/user-profile --branch main

# 7. Announce
mycel channel send #general "🎉 User profile feature live! Thanks team!"

# 8. Record learning
mycel memory record "User profile workflow successful. Took 1.5 hours end-to-end"
```

---

## Quick Reference Card

| Task | Command |
|------|---------|
| Check status | `mycel status` |
| List agents | `mycel agent list` |
| View work queue | `mycel queue work` |
| Send message | `mycel channel send #channel "message"` |
| View agent work | `mycel agent peek engineer-name` |
| Merge PR | `mycel merge branch-name --branch main` |
| Schedule task | `mycel cron create name --schedule "0 2 * * *"` |
| Search memory | `mycel memory search "pattern"` |
| View logs | `mycel agent logs agent-name --tail 20` |
| Get help | `mycel --help` |

---

## Getting Help

**Command line help:**
```bash
mycel --help
bc <command> --help
```

**Documentation:**
- [Full Documentation](https://docs.mycel.dev)
- [API Docs](https://docs.mycel.dev/api)
- [CLI Reference](https://docs.mycel.dev/cli)

**Support:**
- [GitHub Issues](https://github.com/rpuneet/mycel/issues)
- [Discord Community](https://discord.gg/mycel-dev)
- Email: hello@mycel.dev

---

**Happy building! 🚀**

*Last Updated: 2026-02-09*
