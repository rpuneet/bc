# mycel CLI Reference Guide

Complete command reference for the mycel multi-agent orchestration system.

## Table of Contents

1. [Workspace Commands](#workspace-commands)
2. [Agent Management](#agent-management)
3. [Channels & Communication](#channels--communication)
4. [Cost Tracking](#cost-tracking)
5. [Configuration](#configuration)
6. [Scheduled Tasks](#scheduled-tasks)
7. [Daemon & Processes](#daemon--processes)
8. [Secrets & Environment](#secrets--environment)
9. [Tools & Roles](#tools--roles)
10. [Monitoring & Diagnostics](#monitoring--diagnostics)

---

## Workspace Commands

### mycel init
Initialize a new mycel workspace.

```bash
mycel init                        # Interactive wizard
mycel init --quick                # Quick init with defaults
mycel init --preset solo          # Use solo developer preset
mycel init --preset small-team    # Use small team preset
mycel init --preset full-team     # Use full team preset
mycel init ~/Projects/myapp       # Initialize specific directory
```

**Creates:**
- `.bc/settings.json` - Workspace configuration
- `.bc/roles/` - Agent role definitions
- `.bc/agents/` - Per-agent state files

---

### mycel up
Start the root agent via the bcd daemon.

```bash
mycel up                      # Start root agent
mycel up --agent cursor       # Use Cursor AI for agents
mycel up --runtime docker     # Use Docker runtime
```

---

### mycel down
Stop all running agents.

```bash
mycel down          # Stop all agents
mycel down --force  # Force kill without cleanup
```

---

### mycel status
Show agent status overview.

```bash
mycel status                   # Show all agents
mycel status --json            # Output as JSON
mycel status --activity        # Show recent channel activity
```

**Output:**
```
AGENT     ROLE      STATE    UPTIME    TASK
eng-01    engineer  working  2h 15m    Implementing feature X
eng-02    engineer  idle     1h 30m    -
```

---

### mycel home
Open the TUI dashboard.

```bash
mycel home
```

**Navigation:**
- `[1-4]` Switch tabs (Dashboard, Agents, Channels, Costs)
- `[j/k]` Navigate lists (down/up)
- `[?]` Show help
- `[q]` Quit

---

### mycel workspace
Manage workspaces.

```bash
mycel workspace info                   # Show workspace details
mycel workspace status                 # Show agents and health
mycel workspace list                   # List discovered workspaces
mycel workspace list --scan ~/Projects # Scan additional paths
mycel workspace discover               # Discover and register new workspaces
mycel ws up                            # Start all roster agents
```

---

## Agent Management

### mycel agent create
Create and start a new agent.

```bash
mycel agent create --role engineer              # Create with random name
mycel agent create worker-01                    # Create with explicit name
mycel agent create eng-01 --role engineer       # Create engineer
mycel agent create qa-01 --role qa --tool cursor # Create QA with Cursor
```

**Options:**
- `--role` - Agent role (required). Use `mycel role list` to see available roles
- `--tool` - AI tool (claude, gemini, cursor, codex, opencode, openclaw, aider)
- `--runtime` - Runtime backend: tmux or docker
- `--parent` - Parent agent ID
- `--team` - Team name
- `--env` - Path to env file

---

### mycel agent list
List all agents.

```bash
mycel agent list                  # List all agents
mycel agent list --json           # Output as JSON
mycel agent list --role engineer  # Filter by role
mycel agent list --status running # Filter by status
```

---

### mycel agent attach
Attach to an agent's tmux session.

```bash
mycel agent attach eng-01   # Attach to eng-01
```

Use `Ctrl+b d` to detach and return to your shell.

---

### mycel agent peek
View recent output from an agent's session.

```bash
mycel agent peek eng-01              # Show last 500 lines
mycel agent peek eng-01 --lines 100  # Show last 100 lines
mycel agent peek eng-01 --follow     # Stream live output (Ctrl+C to stop)
```

---

### mycel agent send
Send a message to an agent.

```bash
mycel agent send eng-01 "run the tests"
mycel agent send eng-01 "implement login" --preview  # Preview before sending
```

---

### mycel agent broadcast
Send a message to all running agents.

```bash
mycel agent broadcast "run tests"
mycel agent broadcast "check status"
```

---

### mycel agent send-to-role
Send a message to all agents of a role.

```bash
mycel agent send-to-role engineer "run the tests"
mycel agent send-to-role manager "check status"
```

---

### mycel agent send-pattern
Send a message to agents matching a name pattern.

```bash
mycel agent send-pattern "eng-*" "run tests"
mycel agent send-pattern "*-lead" "review PRs"
```

---

### mycel agent stop / start / delete

```bash
mycel agent stop eng-01               # Stop agent
mycel agent stop eng-01 --force       # Force stop
mycel agent start eng-01              # Restart stopped agent
mycel agent start eng-01 --fresh      # Force new session
mycel agent delete eng-01             # Delete agent (preserves memory)
mycel agent delete eng-01 --purge     # Delete including memory
```

---

### mycel agent report
Report agent state (used inside agent sessions).

```bash
mycel agent report working "fixing auth bug"
mycel agent report done "auth bug fixed"
mycel agent report stuck "need database credentials"
mycel agent report stuck --reason "TUI freezes" --severity high
```

---

### mycel agent health
Check agent health status.

```bash
mycel agent health              # Check all agents
mycel agent health eng-01       # Check specific agent
mycel agent health --detect-stuck --alert eng  # Alert on stuck
```

---

### Other agent commands

```bash
mycel agent show eng-01         # Show agent details
mycel agent rename old new      # Rename an agent
mycel agent cost eng-01         # Show agent cost
mycel agent logs eng-01         # Show agent event history
mycel agent sessions eng-01     # Show session IDs
mycel agent stats eng-01        # Docker resource stats
mycel agent auth my-agent       # Authenticate for Docker
```

---

## Channels & Communication

### mycel channel create / delete

```bash
mycel channel create workers            # Create a channel
mycel channel create workers --desc "Worker discussion"
mycel channel delete workers            # Delete a channel
```

---

### mycel channel send

```bash
mycel channel send workers "run tests"  # Send to all members
```

---

### mycel channel list / show / status

```bash
mycel channel list                      # List all channels
mycel channel show workers              # Show channel details
mycel channel status                    # Overview with activity
```

---

### mycel channel history

```bash
mycel channel history eng                       # Last 50 messages
mycel channel history eng --last 20             # Last 20 messages
mycel channel history eng --since 1h            # Messages from last hour
mycel channel history eng --agent agent-core    # Filter by sender
```

---

### mycel channel add / remove / join / leave

```bash
mycel channel add workers worker-01     # Add member
mycel channel remove workers worker-01  # Remove member
mycel channel join workers              # Join (agent session)
mycel channel leave workers             # Leave (agent session)
```

---

### mycel channel react / edit

```bash
mycel channel react engineering 5 thumbsup  # React to message
mycel channel edit eng --desc "Engineering" # Edit description
```

---

## Cost Tracking

```bash
mycel cost                              # Show cost records
mycel cost show eng-01                  # Show costs for agent
mycel cost summary                      # Workspace cost overview
mycel cost daily                        # Daily cost totals
mycel cost agent                        # Per-agent breakdown
mycel cost model                        # Per-model breakdown
mycel cost dashboard                    # Rich cost dashboard
mycel cost usage                        # Claude Code usage via ccusage
mycel cost usage --monthly              # Monthly summary
mycel cost budget show                  # Show budget status
```

---

## Configuration

### mycel config

```bash
mycel config show                        # Show all config
mycel config get providers.default       # Get a specific value
mycel config set providers.default claude # Set a value
mycel config list                        # List all config keys
mycel config edit                        # Open in editor
mycel config validate                    # Validate config file
mycel config reset                       # Reset to defaults
mycel config user init                   # User config wizard
mycel config user show                   # Show user config
```

---

## Scheduled Tasks

### mycel cron

```bash
mycel cron add daily-lint --schedule "0 9 * * *" --agent qa-01 --prompt "Run make lint"
mycel cron list                          # List all cron jobs
mycel cron show daily-lint               # Show job details
mycel cron enable daily-lint             # Enable a disabled job
mycel cron disable daily-lint            # Disable without deleting
mycel cron run daily-lint                # Trigger manually
mycel cron logs daily-lint --last 10     # Show last 10 executions
mycel cron remove daily-lint             # Delete a job
```

---

## Daemon & Processes

### mycel daemon

```bash
mycel daemon start          # Start the bcd HTTP server
mycel daemon status         # Check bcd server health
mycel daemon stop           # Stop bcd server
mycel daemon stop myproc    # Stop a named process
mycel daemon run --name db  # Run a workspace process
mycel daemon list           # List running workspace processes
mycel daemon logs           # View bcd logs
mycel daemon logs myproc    # View process logs
mycel daemon restart myproc # Restart a process
mycel daemon rm myproc      # Remove a stopped process
```

---

## Secrets & Environment

### mycel secret

```bash
mycel secret set ANTHROPIC_API_KEY                    # Prompt for value
mycel secret set ANTHROPIC_API_KEY --value "sk-..."   # Set directly
mycel secret set GITHUB_TOKEN --from-env GITHUB_TOKEN # Import from env
mycel secret list                                     # List names (no values)
mycel secret show ANTHROPIC_API_KEY                   # Show metadata
mycel secret show ANTHROPIC_API_KEY --reveal          # Show actual value
mycel secret get ANTHROPIC_API_KEY                    # Print value to stdout
mycel secret delete ANTHROPIC_API_KEY                 # Delete a secret
```

Reference secrets in config with `${secret:NAME}` syntax.

---

### mycel env

```bash
mycel env set SHARED_VAR global                           # Workspace env
mycel env set --provider claude CLAUDE_CODE_USE_BEDROCK 1 # Provider env
mycel env list                                            # All env vars
mycel env list --provider claude                          # Provider-specific
mycel env get SHARED_VAR                                  # Get value
mycel env unset SHARED_VAR                                # Remove
```

---

## Tools & Roles

### mycel tool

```bash
mycel tool list              # Show all tools with status
mycel tool add myagent       # Add a custom tool
mycel tool show claude       # Show tool details
mycel tool setup claude      # Install and configure
mycel tool status claude     # Check installation status
mycel tool upgrade claude    # Upgrade an installed tool
mycel tool delete mytool     # Remove a custom tool
mycel tool run claude        # Run a tool directly
mycel tool edit mytool       # Edit tool configuration
```

---

### mycel role

```bash
mycel role list              # List all roles
mycel role show engineer     # Show role details
```

---

## Monitoring & Diagnostics

### mycel logs
View the event log.

```bash
mycel logs                     # Show all events
mycel logs --agent eng-01      # Filter by agent
mycel logs --type agent.report # Filter by event type
mycel logs --since 1h          # Events from last hour
mycel logs --tail 20           # Last N events
mycel logs --full              # Show full messages
```

---

### mycel doctor
Run health checks.

```bash
mycel doctor                          # Full health check
mycel doctor check workspace          # Check specific category
mycel doctor fix                      # Auto-fix fixable issues
mycel doctor fix --dry-run            # Preview fixes
mycel doctor fix --category git       # Fix specific category
```

---

### mycel version

```bash
mycel version       # Show version info
mycel --version     # Same as above
mycel -V            # Same as above
```

---

### mycel mcp
Manage MCP server configurations.

```bash
mycel mcp list                                     # List all MCP servers
mycel mcp add github --command npx --args "@modelcontextprotocol/server-github"
mycel mcp add remote --transport sse --url "https://api.example.com/mcp"
mycel mcp show github                              # Show server details
mycel mcp remove github                            # Remove a server
mycel mcp disable github                           # Disable a server
mycel mcp enable github                            # Re-enable a server
mycel mcp register                                 # Register mycel as MCP server
mycel mcp serve                                    # Start mycel as MCP server
```

---

## Quick Reference

```bash
# Daily workflow
mycel up                                    # Start root agent
mycel status                                # Check status
mycel agent create eng-01 --role engineer   # Create agent
mycel agent send eng-01 "implement X"       # Send work
mycel agent peek eng-01                     # Watch output
mycel home                                  # Open dashboard
mycel down                                  # Stop all

# Communication
mycel channel send eng "starting tests"     # Channel message
mycel agent broadcast "check status"        # Broadcast to all

# Monitoring
mycel logs --tail 20                        # Recent events
mycel doctor                                # Health check
mycel cost summary                          # Cost overview
```

---

## Global Flags

```bash
-v, --verbose   # Enable verbose output
    --json      # Output in JSON format
-V, --version   # Print version information
```

---

## Environment Variables

```bash
BC_AGENT_ID       # Current agent name (set in agent sessions)
BC_AGENT_ROLE     # Current agent role
BC_WORKSPACE      # Path to workspace root
BC_AGENT_WORKTREE # Path to agent's worktree
BC_BIN            # Path to bc binary
BC_ROOT           # Workspace root directory
NO_COLOR          # Disable colored output
```

---

**For more help:** `mycel --help` or `mycel <command> --help`
