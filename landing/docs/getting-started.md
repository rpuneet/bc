# Getting Started with bc

Welcome to bc! This guide will walk you through installation, setup, and your first multi-agent workflow.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Installation](#installation)
3. [Quick Start (5 min)](#quick-start-5-min)
4. [First Workflow](#first-workflow)
5. [Common Commands](#common-commands)
6. [Troubleshooting](#troubleshooting)
7. [Next Steps](#next-steps)

---

## Prerequisites

Before installing bc, ensure you have:

### Required
- **Go 1.25.4+** - [Download Go](https://go.dev/dl)
- **tmux** - Terminal multiplexer for session management
- **Claude Code** (or compatible AI agent like Cursor)
- **git** - Version control system

### Recommended
- **macOS/Linux** - Primary supported platforms
- **Terminal/Shell** - bash, zsh, or compatible shell
- **Git experience** - Familiarity with git commands

### Optional
- **Docker** - For isolated testing environments
- **Node.js 18+** - If running mycel on Node.js projects

---

## Installation

### macOS

```bash
# Install Go (if not already installed)
brew install go

# Install tmux
brew install tmux

# Clone mycel repository
git clone https://github.com/bcinfra1/bc.git
cd bc

# Build from source
make build

# Install to GOPATH/bin
make install

# Verify installation
bc --version
```

### Linux

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y golang-go tmux git

# Fedora/RHEL
sudo dnf install -y golang tmux git

# Clone and build
git clone https://github.com/bcinfra1/bc.git
cd bc
make build
make install

# Verify installation
bc --version
```

### Windows (WSL2 Recommended)

```bash
# Install WSL2 and Ubuntu
wsl --install -d Ubuntu-22.04

# Inside WSL2:
sudo apt-get update
sudo apt-get install -y golang-go tmux git

# Clone and build
git clone https://github.com/bcinfra1/bc.git
cd bc
make build
make install

# Verify installation
bc --version
```

---

## Quick Start (5 min)

### 1. Initialize Workspace

```bash
# Create a project directory
mkdir my-project
cd my-project

# Initialize bc workspace
bc init

# Expected output:
# ✓ Workspace initialized at /Users/you/my-project/.bc/
# ✓ Config created: .bc/settings.json
# ✓ Queue initialized: .bc/queue.json
```

### 2. Start Root Agent

```bash
# Start the root coordinator agent
bc up

# Expected output:
# ✓ Root agent started
# ✓ Workspace ready
# ✓ Use 'bc home' to view dashboard

# Check status
bc status
# Shows: root (running)
```

### 3. Open TUI Dashboard

```bash
# View the interactive dashboard
bc home

# Navigation:
# - Arrow keys: Navigate agents/tasks
# - Enter: View details
# - q: Quit dashboard
```

### 4. Spawn Your First Agent

```bash
# In a new terminal, spawn an engineer agent
bc spawn eng-01 --role engineer

# Expected output:
# ✓ Agent eng-01 spawned
# ✓ Worktree created: .bc/worktrees/eng-01/
# ✓ tmux session started
```

### 5. Add Your First Task

```bash
# Add a task to the queue
bc queue add "Implement login feature"

# List tasks
bc queue list

# Output:
# ID          Status     Title
# work-0001   pending    Implement login feature
```

### 6. Assign Work

```bash
# Assign task to agent
bc queue assign work-0001 eng-01

# Check assignment
bc queue list

# Output:
# work-0001   assigned   Implement login feature   (assigned to: eng-01)
```

### 7. Agent Executes Work

```bash
# Attach to agent session to see execution
bc attach eng-01

# Inside agent session:
# - View current working directory: pwd
# - Agent has access to full project
# - Agent can make commits, create branches
# - Agent reports progress via: bc report working "message"
```

### 8. Report Completion

```bash
# Agent signals completion (from within session or external)
bc report done "Login feature implemented and tested"

# Check queue
bc queue list

# Output:
# work-0001   done   Implement login feature   (completed by: eng-01)
```

### 9. Merge and Clean Up

```bash
# List mergeable work
bc merge list

# Merge all completed work to main
bc merge process

# Verify main branch has changes
git log --oneline -5
```

---

## First Workflow

### Real Example: Feature Development

This walkthrough shows a complete feature development workflow.

#### Scenario: Add Email Notifications Feature

**Step 1: Initialize Project**

```bash
mkdir notification-service
cd notification-service
git init
git branch main
bc init
bc up
```

**Step 2: Create Work Items**

```bash
# Add feature task
bc queue add "Add email notification service"

# Add parallel work
bc queue add "Create email templates"
bc queue add "Write tests for notifications"
```

**Step 3: Spawn Team**

```bash
# Spawn engineers for parallel work
bc spawn eng-01 --role engineer  # Backend API
bc spawn eng-02 --role engineer  # Templates
bc spawn qa-01 --role qa         # Testing

# Spawn tech lead for reviews
bc spawn tech-lead-01 --role tech-lead
```

**Step 4: Assign Work**

```bash
# Assign tasks
bc queue assign work-0001 eng-01  # API task
bc queue assign work-0002 eng-02  # Templates task
bc queue assign work-0003 qa-01   # Tests task

# Check progress
bc status
```

**Step 5: Monitor Execution**

```bash
# View real-time status
bc home

# See all agents working in parallel:
# - eng-01: working (in .bc/worktrees/eng-01/)
# - eng-02: working (in .bc/worktrees/eng-02/)
# - qa-01:  idle   (waiting for work)
```

**Step 6: Agents Report Progress**

Each agent works independently in their worktree:

```bash
# eng-01's work:
cd .bc/worktrees/eng-01/
git checkout -b feature/email-api
# ... implement email service ...
git add .
git commit -m "feat: email notification API"
bc report done "Email API complete - ready for review"
```

**Step 7: Review and Merge**

```bash
# List completed work
bc merge list

# Merge to main
bc merge process

# Verify on main
git log --oneline

# Output:
# abc1234 feat: email notification API (eng-01)
# def5678 feat: email templates (eng-02)
# ghi9012 test: notifications test suite (qa-01)
```

**Step 8: Next Cycle**

```bash
# Spawn next team for additional features
bc spawn eng-03 --role engineer
bc queue add "Add SMS notification provider"
bc queue assign work-0004 eng-03
```

---

## Common Commands

### Workspace Management

```bash
# Initialize new workspace
bc init

# Start root agent
bc up

# Check workspace status
bc status

# Stop all agents
bc down

# View logs
bc logs

# View dashboard
bc home
```

### Agent Management

```bash
# Spawn new agent
bc spawn eng-01 --role engineer
bc spawn tech-lead-01 --role tech-lead

# List agents
bc status

# Attach to agent session
bc attach eng-01

# View agent logs
bc logs eng-01

# Stop agent
bc stop eng-01

# Kill agent session
bc kill eng-01
```

### Work Queue

```bash
# Add task
bc queue add "Implement feature X"

# List all tasks
bc queue list

# Show task details
bc queue show work-0001

# Assign task
bc queue assign work-0001 eng-01

# Mark task status
bc report working "In progress..."
bc report done "Completed!"
bc report stuck "Blocked by dependency"
bc report failed "Error occurred"

# Clear completed
bc queue clear completed
```

### Git & Merge

```bash
# View branches to merge
bc merge list

# Merge all ready branches
bc merge process

# Merge specific task
bc merge work-0001

# View merge conflicts
bc merge conflicts

# Abort merge
bc merge abort

# View main branch
git log main --oneline
```

### Channels (Communication)

```bash
# Send message to agent
bc send eng-01 "Check our conversation for details"

# Send to channel
bc send #eng-team "Update ready for review"

# List channels
bc channels list

# View channel history
bc channels read #eng-team
```

---

## Troubleshooting

### Issue: "tmux not found"

**Error:**
```
error: tmux is not installed or not in PATH
```

**Solution:**
```bash
# macOS
brew install tmux

# Linux (Ubuntu)
sudo apt-get install tmux

# Linux (Fedora)
sudo dnf install tmux

# Verify installation
tmux --version
```

### Issue: "Agent won't start"

**Error:**
```
error: failed to spawn agent eng-01
```

**Solutions:**
```bash
# Check if tmux is running
tmux list-sessions

# Kill stale sessions
tmux kill-server

# Re-init workspace
bc down
rm -rf .bc/
bc init
bc up
```

### Issue: "Merge conflicts"

**Error:**
```
error: merge conflict in src/app/page.tsx
```

**Solution:**
```bash
# View conflicts
bc merge conflicts

# Resolve manually or use agent
cd .bc/worktrees/eng-01/
# ... fix conflicts ...
git add .
git commit -m "resolve merge conflicts"

# Continue merge
bc merge process
```

### Issue: "Permission denied" on worktree

**Error:**
```
fatal: permission denied while trying to open: .bc/worktrees/eng-01/
```

**Solution:**
```bash
# Check permissions
ls -la .bc/worktrees/

# Fix permissions
chmod -R 755 .bc/

# Retry operation
bc status
```

### Issue: "Agent not in workspace"

**Error:**
```
error: workspace not initialized
```

**Solution:**
```bash
# Check if in correct directory
pwd
ls .bc/

# If missing, re-initialize
bc init

# Or navigate to correct project
cd /path/to/project
```

### Issue: "Claude Code not found"

**Error:**
```
error: no AI agent configured
```

**Solution:**
```bash
# Install Claude Code CLI
# Visit: https://claude.com/claude-code

# Configure mycel to use Claude Code
bc config set agent-tool claude-code

# Verify configuration
bc config show
```

---

## Architecture Overview

### Directory Structure

```
my-project/
├── .bc/                          # mycel state directory
│   ├── settings.json              # Workspace configuration
│   ├── queue.json               # Work queue
│   ├── events.jsonl             # Event log
│   ├── agents/                  # Agent state
│   │   └── agents.json
│   └── worktrees/               # Per-agent git worktrees
│       ├── pm-01/               # Product Manager
│       ├── eng-01/              # Engineer 1
│       ├── eng-02/              # Engineer 2
│       └── qa-01/               # QA Agent
├── src/                         # Your source code
├── .git/                        # Git repository
└── README.md                    # Project documentation
```

### Agent Hierarchy

```
Root (Coordinator)
├── Product Manager (pm-01)
│   └── Manager (mgr-01)
│       ├── Engineer (eng-01)
│       ├── Engineer (eng-02)
│       └── QA (qa-01)
```

### Work Lifecycle

```
pending → assigned → working → done
                               ↓ (conflict)
                            stuck/failed
```

---

## Key Concepts

### Worktrees
Each agent works in isolated git worktrees (`.bc/worktrees/<agent>/`). This prevents merge conflicts when multiple agents work on the same files simultaneously.

### Persistent Memory
All work state is stored in git-tracked files. If an agent crashes or restarts, it continues from where it left off.

### Role-Based Capabilities
Agents have defined roles with specific capabilities:
- **ProductManager**: Create epics, spawn managers, assign work
- **Manager**: Spawn engineers/QA, assign work, review
- **Engineer**: Implement code
- **QA**: Test and validate

### Channels
Real-time messaging between agents for coordination without losing context.

---

## Next Steps

### Learn More
- [Architecture Overview](https://github.com/bcinfra1/bc/.ctx/01-architecture-overview.md)
- [Agent Types and Roles](https://github.com/bcinfra1/bc/.ctx/02-agent-types.md)
- [CLI Reference](https://github.com/bcinfra1/bc/.ctx/03-cli-reference.md)

### Build Your First Project
1. Create a small project (e.g., API service, web app)
2. Initialize bc workspace
3. Spawn 2-3 agents for different features
4. Assign work and monitor progress
5. Merge completed work to main

### Advanced Topics
- Custom agent roles and capabilities
- Workflow automation
- Integration with CI/CD pipelines
- Team scaling patterns

---

## Support

### Documentation
- Full docs: [bc GitHub Repository](https://github.com/bcinfra1/bc)
- Issues: [GitHub Issues](https://github.com/bcinfra1/bc/issues)
- Discussions: [GitHub Discussions](https://github.com/bcinfra1/bc/discussions)

### Community
- Join bc workspace for real-time support
- Ask questions in #help channel
- Share workflows in #showcase

---

## Quick Reference Card

```bash
# Initialization
bc init                                # Initialize workspace
bc up                                  # Start root agent

# Agents
bc spawn eng-01 --role engineer       # Spawn agent
bc status                              # List agents
bc attach eng-01                       # Connect to agent
bc down                                # Stop all agents

# Work
bc queue add "Title"                   # Create task
bc queue assign work-0001 eng-01       # Assign task
bc report done "Message"               # Complete task
bc merge process                       # Merge to main

# Monitoring
bc home                                # View dashboard
bc status                              # Check status
bc logs                                # View logs
bc queue list                          # List tasks
```

---

**Happy building with bc! 🚀**

For more information, visit [bc on GitHub](https://github.com/bcinfra1/bc)
