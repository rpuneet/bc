# mycel Features - Detailed Guide

## Overview

mycel provides core features designed specifically for coordinating multiple AI agents on complex development tasks. Each feature solves a specific problem in multi-agent workflows.

---

## 1. Git Worktrees - Conflict-Free Parallel Development

### The Problem
Traditional git workflows create merge conflicts when multiple developers (or agents) modify the same files. With AI agents, this problem is magnified:
- Multiple agents working simultaneously
- Agents may edit same files independently
- Manual conflict resolution slows progress

### The Solution: Per-Agent Worktrees
Each agent gets an isolated git worktree at `.bc/worktrees/<agent-id>/`:

```
project/
├── .git/                          # Main repository
├── src/
│   └── app.js                     # Main code
├── .bc/
│   └── worktrees/
│       ├── eng-01/                # eng-01's isolated copy
│       │   └── src/app.js         # eng-01 edits independently
│       └── eng-02/                # eng-02's isolated copy
│           └── src/app.js         # eng-02 edits independently
```

### How It Works
```bash
# Initialize workspace
mycel init

# Two engineers spawned
bc spawn eng-01 --role engineer
bc spawn eng-02 --role engineer

# Both work on app.js simultaneously
cd .bc/worktrees/eng-01/
# eng-01: add email notifications to app.js
git commit -m "add email service"

cd .bc/worktrees/eng-02/
# eng-02: add logging to app.js
git commit -m "add logging"

# Merge both to main - NO CONFLICTS!
mycel merge process
# Result: app.js has both changes merged cleanly
```

### Benefits
- **Zero conflicts** - Agents work independently
- **True parallelization** - No waiting for locks
- **Easy review** - Each agent's changes isolated
- **Safe merging** - Git handles merges cleanly
- **Isolation** - Agents can't interfere with each other

### Real Example: API Development
```
Backend API with 3 services:

eng-01 (Authentication Service)
├── auth.js - Implement login/logout
├── tokens.js - Generate JWT tokens
└── tests.js - Write auth tests

eng-02 (User Service)
├── users.js - User CRUD operations
├── email.js - Send verification emails
└── tests.js - Write user tests

eng-03 (Payment Service)
├── payments.js - Handle transactions
├── webhooks.js - Process callbacks
└── tests.js - Write payment tests

Result: All merge to main with zero conflicts
```

---

## 2. Persistent Memory - Context That Survives Crashes

### The Problem
AI agents lose context when they restart. Traditional solutions:
- Saving transcripts (loses structure)
- Manual notes (incomplete)
- Chat history (loses system context)

### The Solution: Git-Backed Persistence
All mycel state stored in git-tracked files:

```
.bc/
├── agents/agents.json             # Agent state & history
├── queue.json                     # Work queue & assignments
├── events.jsonl                   # Complete event log
└── worktrees/
    └── eng-01/                    # Agent's full git state
```

### Persistent Information
1. **Agent State** - Role, capabilities, current task
2. **Work Queue** - All tasks, assignments, status
3. **Event Log** - Every action (JSON Lines format)
4. **Code Context** - Complete git history in worktree
5. **Communication** - Channel messages logged

### How It Works
```bash
# Session 1: Agent starts work
bc spawn eng-01 --role engineer
mycel queue add "Implement authentication"
mycel queue assign work-0001 eng-01
bc attach eng-01
# eng-01 works on authentication...
bc report working "Implementing JWT validation"
# ... agent crashes or is stopped

# Session 2: Complete recovery
mycel status  # Shows: eng-01 was working on work-0001
cd .bc/worktrees/eng-01/
git log --oneline  # Shows all previous work
cat .bc/queue.json  # Shows eng-01 still assigned to work-0001
bc attach eng-01  # Resume from exact point
```

### Benefits
- **Crash recovery** - No work lost
- **Audit trail** - Complete history of all work
- **Reproducibility** - Replay exact sequence
- **Debugging** - Understand how something went wrong
- **Compliance** - Full record for auditing

### Real Example: Deployment Freeze
```
Scenario: Production deployment in progress

eng-01: Deploying to staging
├── Time 14:00 - Start deployment
├── Time 14:05 - Tests passing
├── Time 14:10 - CRASH (server failure)
│
Session recovered - complete state preserved
├── Knows deployment was in progress
├── Can resume from exact point
├── No need to restart from beginning

Result: Fast recovery without data loss
```

---

## 3. Role-Based Hierarchy - Organized Team Structure

### The Problem
Unorganized agent teams can:
- Create redundant work
- Miss important tasks
- Lack clear authority
- Become chaotic at scale

### The Solution: Clear Role Hierarchy
mycel implements 3-level hierarchy with defined capabilities:

```
Level 0: ProductManager (Strategic)
├── Can: Create epics, spawn managers, assign work
├── Cannot: Implement code
└── Role: Set direction and coordinate

Level 1: Manager (Tactical)
├── Can: Spawn engineers/QA, assign work, review
├── Cannot: Create epics or implement
└── Role: Execute strategy, delegate work

Level 2: Engineer/QA (Execution)
├── Can: Implement code, run tests
├── Cannot: Create agents or assign work
└── Role: Build and validate
```

### How It Works
```bash
# Build hierarchy
bc spawn pm-01 --role product-manager

bc spawn mgr-01 --role manager --parent pm-01
bc spawn mgr-02 --role manager --parent pm-01

bc spawn eng-01 --role engineer --parent mgr-01
bc spawn eng-02 --role engineer --parent mgr-01
bc spawn qa-01 --role qa --parent mgr-01

bc spawn eng-03 --role engineer --parent mgr-02
bc spawn eng-04 --role engineer --parent mgr-02
bc spawn qa-02 --role qa --parent mgr-02

# Structure:
# pm-01 (Product Manager)
# ├── mgr-01 (Manager)
# │   ├── eng-01 (Engineer)
# │   ├── eng-02 (Engineer)
# │   └── qa-01 (QA)
# └── mgr-02 (Manager)
#     ├── eng-03 (Engineer)
#     ├── eng-04 (Engineer)
#     └── qa-02 (QA)
```

### Capability Enforcement
```bash
# Valid: PM can spawn managers
bc spawn mgr-01 --role manager --parent pm-01  # ✓ Works

# Invalid: Engineer cannot spawn agents
bc spawn eng-02 --role engineer --parent pm-01  # ✗ Fails

# Valid: Manager can spawn engineers
bc spawn eng-01 --role engineer --parent mgr-01  # ✓ Works

# Invalid: Engineer cannot assign work
mycel queue assign work-0001 eng-01  # ✗ Fails (engineers execute, don't assign)
```

### Benefits
- **Clear authority** - Who can do what
- **Scalability** - Organize large teams
- **Accountability** - Traceable decision chain
- **Safety** - Prevent unauthorized actions
- **Coordination** - Structured communication

### Real Example: Startup Organization
```
Startup with 12 agents:

Product Manager (pm-01)
├── Creates epics for quarterly goals
├── Spawns managers for each functional area
└── Reviews completed work

Engineering Manager (mgr-01)
├── Owns backend/API
├── Spawns 3 backend engineers
├── Assigns tasks from pm-01's backlog
└── Reviews code before merge

Backend Engineers (eng-01, eng-02, eng-03)
├── Implement features independently
├── Work in own worktrees (zero conflicts)
├── Report completion status
└── QA tests their work

QA Lead (qa-01)
├── Validates all merged changes
├── Spawns additional QA agents if needed
└── Reports bugs back to managers

Result: 12-agent team operating smoothly with clear structure
```

---

## 4. Real-Time Channels - Context-Preserving Communication

### The Problem
Agents lose context when switching communications:
- Slack messages fade into history
- Email chains are hard to follow
- Mentions get lost in channels

### The Solution: Native Channel System
mycel provides first-class channels for agent coordination:

```bash
# Send to individual
bc send eng-01 "Check requirements in /docs/spec.md"

# Send to channel
bc send #engineering "API endpoint ready for integration testing"
bc send #qa "New build available - test login flow"

# View history
bc logs #engineering
bc logs eng-01

# Structured messages preserve context
```

### Channel Types
1. **Direct Messages** - Agent to agent (1:1)
2. **Team Channels** - Role-based (#engineering, #design)
3. **Project Channels** - Task-specific (#user-auth, #payment-api)
4. **Status Channels** - Broadcast updates (#deployments, #builds)

### How It Works
```bash
# Create project team
bc spawn eng-01 --role engineer
bc spawn eng-02 --role engineer
bc spawn qa-01 --role qa

# Assign task
mycel queue add "Build user authentication"
mycel queue assign work-0001 eng-01

# Send context
bc send eng-01 "User auth task assigned. See requirements at /docs/auth.md"

# Eng-01 works
bc attach eng-01  # Starts work

# Meanwhile, eng-02 can ask questions
bc send #engineering "Does auth need 2FA?"

# eng-01 can respond when ready
bc send #engineering "2FA optional - can add later"

# qa-01 prepares tests
bc send qa-01 "I'll test auth flow once eng-01 finishes"

# Complete workflow with clear communication
```

### Benefits
- **Context preserved** - All related info in channels
- **Async coordination** - Agents don't block each other
- **Audit trail** - Full communication history
- **No context switching** - Work and communication together
- **Team awareness** - Everyone knows what's happening

### Real Example: Feature Development
```
Feature: "Add password reset"

1. Product sends requirements
   bc send #engineering "Password reset requirements in /docs/password-reset.md"

2. Engineering discusses approach
   bc send #engineering "I'll use JWT tokens with 1-hour expiry"
   bc send #engineering "Will hash tokens for security"

3. QA prepares tests
   bc send qa-01 "I'm ready to test password reset flow"

4. Development completes
   bc report done "Password reset implemented and tested"

5. QA validates
   bc send #engineering "All password reset tests passing"

6. Ready to merge
   mycel merge process

All communication in context, none lost
```

---

## 5. Work Queue - Task Lifecycle Management

### The Problem
Tracking work across multiple agents is complex:
- Which agent is working on what?
- What's completed vs. in progress?
- What's blocked or failed?

### The Solution: Native Work Queue
mycel provides built-in task tracking:

```
Lifecycle: pending → assigned → working → done
                                        ↓ (conflict)
                                       stuck/failed
```

### How It Works
```bash
# Create work item
mycel queue add "Implement user registration"
# Creates: work-0001 (status: pending)

# Assign to agent
mycel queue assign work-0001 eng-01
# Updates: work-0001 (status: assigned, assigned_to: eng-01)

# Agent reports progress
bc attach eng-01
bc report working "Building registration form"
# Updates: work-0001 (status: working)

# Agent completes
bc report done "Registration form complete - ready for testing"
# Updates: work-0001 (status: done, merge_status: unmerged)

# Review and merge
mycel merge list  # Shows: work-0001 ready to merge
mycel merge process
# Updates: work-0001 (merge_status: merged, merged_at: timestamp)
```

### Queue Operations
```bash
# List all work
mycel queue list

# Show details
mycel queue show work-0001

# View progress
mycel queue status

# Clear completed
mycel queue clear completed

# Track metrics
mycel queue metrics  # Shows: completed, avg time, etc.
```

### Queue Fields
```json
{
  "id": "work-0001",
  "title": "Implement user registration",
  "description": "Create registration form and backend handler",
  "status": "done",
  "assigned_to": "eng-01",
  "assigned_at": "2026-02-09T10:00:00Z",
  "started_at": "2026-02-09T10:05:00Z",
  "completed_at": "2026-02-09T14:30:00Z",
  "duration_minutes": 265,
  "priority": "high",
  "merge_status": "merged",
  "merged_at": "2026-02-09T15:00:00Z",
  "merge_conflicts": 0
}
```

### Benefits
- **Visibility** - See all work at a glance
- **Accountability** - Know who's doing what
- **Progress tracking** - Measure velocity
- **Bottleneck detection** - Find stuck work
- **Metrics** - Optimize team performance

### Real Example: Sprint Management
```
Sprint Planning:
- PM creates 10 work items for 2-week sprint
- Mgr assigns 5 to team 1, 5 to team 2
- Teams execute independently

Progress Tracking:
- mycel queue list shows: 2 done, 3 working, 5 pending
- Can drill down: eng-01 blocked on API response
- Can prioritize: move API work to top

Sprint Complete:
- All work done and merged
- Metrics: 10 items, 8 days, avg 32 mins per task
- Ready for next sprint

Continuous visibility without overhead
```

---

## 6. TUI Dashboard - Real-Time Monitoring

### The Problem
Text-based status lacks visibility:
- Hard to see overall progress
- Must run multiple commands
- No real-time updates

### The Solution: Interactive TUI Dashboard
```bash
bc home
```

Provides:
- **Agent Status** - Who's working, who's idle
- **Work Queue** - Tasks and progress
- **Channel Activity** - Recent messages
- **System Health** - Resource usage
- **Merge Queue** - Ready to integrate changes

### Dashboard Features
```
╔════════════════════════════════════════════════════════════╗
║ mycel Dashboard                                               ║
╠════════════════════════════════════════════════════════════╣
║ AGENTS                                                     ║
║ pm-01     ├─ ProductManager   ├─ idle      ├─ 2h 15m      ║
║ mgr-01    ├─ Manager          ├─ working   ├─ 45m (task 1) ║
║ eng-01    ├─ Engineer         ├─ working   ├─ 2h 30m      ║
║ eng-02    ├─ Engineer         ├─ idle      ├─ waiting...   ║
║ qa-01     ├─ QA               ├─ idle      ├─ ready        ║
║                                                            ║
║ WORK QUEUE                                                 ║
║ work-0001 ├─ user-auth       ├─ done      ├─ ready merge  ║
║ work-0002 ├─ payments         ├─ working   ├─ eng-01       ║
║ work-0003 ├─ notifications   ├─ assigned  ├─ eng-02       ║
║ work-0004 ├─ docs             ├─ pending   ├─ unassigned   ║
║                                                            ║
║ RECENT ACTIVITY                                            ║
║ 14:35 eng-01: "Payments API ready for testing"            ║
║ 14:20 qa-01: "Auth flow tests passing"                    ║
║ 14:10 eng-02: "Started notifications feature"            ║
╚════════════════════════════════════════════════════════════╝
```

### Dashboard Operations
```bash
bc home          # Open dashboard
↑/↓              # Navigate
Enter            # View details
q                # Quit
:refresh         # Force refresh
```

---

## Summary: Feature Comparison

| Feature | Traditional Teams | mycel Teams |
|---------|------------------|----------|
| **Merge Conflicts** | Common, manual resolution | Zero (worktree isolation) |
| **Context Loss** | On restart (chat history) | Never (git-backed) |
| **Team Organization** | Flat or informal | Hierarchical, role-based |
| **Communication** | Separate tools (Slack) | Integrated channels |
| **Task Tracking** | Separate system (Jira) | Built-in work queue |
| **Visibility** | Multiple dashboards | Single TUI dashboard |
| **Parallelization** | Limited by conflicts | True parallel work |
| **Scalability** | Manual coordination | Automatic via hierarchy |
| **Audit Trail** | Scattered | Complete in git |

---

**Next:** See [Getting Started Guide](./getting-started.md) to try these features.
