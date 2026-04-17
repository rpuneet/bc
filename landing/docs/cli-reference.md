# bc CLI Reference Guide

Complete command reference for the bc multi-agent orchestration system.

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

### bc init
Initialize a new bc v2 workspace.

```bash
bc init                        # Interactive wizard
bc init --quick                # Quick init with defaults
bc init --preset solo          # Use solo developer preset
bc init --preset small-team    # Use small team preset
bc init --preset full-team     # Use full team preset
bc init ~/Projects/myapp       # Initialize specific directory
```

**Creates:**
- `.bc/settings.json` - Workspace configuration
- `.bc/roles/` - Agent role definitions
- `.bc/agents/` - Per-agent state files

---

### bc up
Start the root agent via the bcd daemon.

```bash
bc up                      # Start root agent
bc up --agent cursor       # Use Cursor AI for agents
bc up --runtime docker     # Use Docker runtime
```

---

### bc down
Stop all running agents.

```bash
bc down          # Stop all agents
bc down --force  # Force kill without cleanup
```

---

### bc status
Show agent status overview.

```bash
bc status                   # Show all agents
bc status --json            # Output as JSON
bc status --activity        # Show recent channel activity
```

**Output:**
```
AGENT     ROLE      STATE    UPTIME    TASK
eng-01    engineer  working  2h 15m    Implementing feature X
eng-02    engineer  idle     1h 30m    -
```

---

### bc home
Open the TUI dashboard.

```bash
bc home
```

**Navigation:**
- `[1-4]` Switch tabs (Dashboard, Agents, Channels, Costs)
- `[j/k]` Navigate lists (down/up)
- `[?]` Show help
- `[q]` Quit

---

### bc workspace
Manage workspaces.

```bash
bc workspace info                   # Show workspace details
bc workspace status                 # Show agents and health
bc workspace list                   # List discovered workspaces
bc workspace list --scan ~/Projects # Scan additional paths
bc workspace discover               # Discover and register new workspaces
bc ws up                            # Start all roster agents
```

---

## Agent Management

### bc agent create
Create and start a new agent.

```bash
bc agent create --role engineer              # Create with random name
bc agent create worker-01                    # Create with explicit name
bc agent create eng-01 --role engineer       # Create engineer
bc agent create qa-01 --role qa --tool cursor # Create QA with Cursor
```

**Options:**
- `--role` - Agent role (required). Use `bc role list` to see available roles
- `--tool` - AI tool (claude, gemini, cursor, codex, opencode, openclaw, aider)
- `--runtime` - Runtime backend: tmux or docker
- `--parent` - Parent agent ID
- `--team` - Team name
- `--env` - Path to env file

---

### bc agent list
List all agents.

```bash
bc agent list                  # List all agents
bc agent list --json           # Output as JSON
bc agent list --role engineer  # Filter by role
bc agent list --status running # Filter by status
```

---

### bc agent attach
Attach to an agent's tmux session.

```bash
bc agent attach eng-01   # Attach to eng-01
```

Use `Ctrl+b d` to detach and return to your shell.

---

### bc agent peek
View recent output from an agent's session.

```bash
bc agent peek eng-01              # Show last 500 lines
bc agent peek eng-01 --lines 100  # Show last 100 lines
bc agent peek eng-01 --follow     # Stream live output (Ctrl+C to stop)
```

---

### bc agent send
Send a message to an agent.

```bash
bc agent send eng-01 "run the tests"
bc agent send eng-01 "implement login" --preview  # Preview before sending
```

---

### bc agent broadcast
Send a message to all running agents.

```bash
bc agent broadcast "run tests"
bc agent broadcast "check status"
```

---

### bc agent send-to-role
Send a message to all agents of a role.

```bash
bc agent send-to-role engineer "run the tests"
bc agent send-to-role manager "check status"
```

---

### bc agent send-pattern
Send a message to agents matching a name pattern.

```bash
bc agent send-pattern "eng-*" "run tests"
bc agent send-pattern "*-lead" "review PRs"
```

---

### bc agent stop / start / delete

```bash
bc agent stop eng-01               # Stop agent
bc agent stop eng-01 --force       # Force stop
bc agent start eng-01              # Restart stopped agent
bc agent start eng-01 --fresh      # Force new session
bc agent delete eng-01             # Delete agent (preserves memory)
bc agent delete eng-01 --purge     # Delete including memory
```

---

### bc agent report
Report agent state (used inside agent sessions).

```bash
bc agent report working "fixing auth bug"
bc agent report done "auth bug fixed"
bc agent report stuck "need database credentials"
bc agent report stuck --reason "TUI freezes" --severity high
```

---

### bc agent health
Check agent health status.

```bash
bc agent health              # Check all agents
bc agent health eng-01       # Check specific agent
bc agent health --detect-stuck --alert eng  # Alert on stuck
```

---

### Other agent commands

```bash
bc agent show eng-01         # Show agent details
bc agent rename old new      # Rename an agent
bc agent cost eng-01         # Show agent cost
bc agent logs eng-01         # Show agent event history
bc agent sessions eng-01     # Show session IDs
bc agent stats eng-01        # Docker resource stats
bc agent auth my-agent       # Authenticate for Docker
```

---

## Channels & Communication

### bc channel create / delete

```bash
bc channel create workers            # Create a channel
bc channel create workers --desc "Worker discussion"
bc channel delete workers            # Delete a channel
```

---

### bc channel send

```bash
bc channel send workers "run tests"  # Send to all members
```

---

### bc channel list / show / status

```bash
bc channel list                      # List all channels
bc channel show workers              # Show channel details
bc channel status                    # Overview with activity
```

---

### bc channel history

```bash
bc channel history eng                       # Last 50 messages
bc channel history eng --last 20             # Last 20 messages
bc channel history eng --since 1h            # Messages from last hour
bc channel history eng --agent agent-core    # Filter by sender
```

---

### bc channel add / remove / join / leave

```bash
bc channel add workers worker-01     # Add member
bc channel remove workers worker-01  # Remove member
bc channel join workers              # Join (agent session)
bc channel leave workers             # Leave (agent session)
```

---

### bc channel react / edit

```bash
bc channel react engineering 5 thumbsup  # React to message
bc channel edit eng --desc "Engineering" # Edit description
```

---

## Cost Tracking

```bash
bc cost                              # Show cost records
bc cost show eng-01                  # Show costs for agent
bc cost summary                      # Workspace cost overview
bc cost daily                        # Daily cost totals
bc cost agent                        # Per-agent breakdown
bc cost model                        # Per-model breakdown
bc cost dashboard                    # Rich cost dashboard
bc cost usage                        # Claude Code usage via ccusage
bc cost usage --monthly              # Monthly summary
bc cost budget show                  # Show budget status
```

---

## Configuration

### bc config

```bash
bc config show                        # Show all config
bc config get providers.default       # Get a specific value
bc config set providers.default claude # Set a value
bc config list                        # List all config keys
bc config edit                        # Open in editor
bc config validate                    # Validate config file
bc config reset                       # Reset to defaults
bc config user init                   # User config wizard
bc config user show                   # Show user config
```

---

## Scheduled Tasks

### bc cron

```bash
bc cron add daily-lint --schedule "0 9 * * *" --agent qa-01 --prompt "Run make lint"
bc cron list                          # List all cron jobs
bc cron show daily-lint               # Show job details
bc cron enable daily-lint             # Enable a disabled job
bc cron disable daily-lint            # Disable without deleting
bc cron run daily-lint                # Trigger manually
bc cron logs daily-lint --last 10     # Show last 10 executions
bc cron remove daily-lint             # Delete a job
```

---

## Daemon & Processes

### bc daemon

```bash
bc daemon start          # Start the bcd HTTP server
bc daemon status         # Check bcd server health
bc daemon stop           # Stop bcd server
bc daemon stop myproc    # Stop a named process
bc daemon run --name db  # Run a workspace process
bc daemon list           # List running workspace processes
bc daemon logs           # View bcd logs
bc daemon logs myproc    # View process logs
bc daemon restart myproc # Restart a process
bc daemon rm myproc      # Remove a stopped process
```

---

## Secrets & Environment

### bc secret

```bash
bc secret set ANTHROPIC_API_KEY                    # Prompt for value
bc secret set ANTHROPIC_API_KEY --value "sk-..."   # Set directly
bc secret set GITHUB_TOKEN --from-env GITHUB_TOKEN # Import from env
bc secret list                                     # List names (no values)
bc secret show ANTHROPIC_API_KEY                   # Show metadata
bc secret show ANTHROPIC_API_KEY --reveal          # Show actual value
bc secret get ANTHROPIC_API_KEY                    # Print value to stdout
bc secret delete ANTHROPIC_API_KEY                 # Delete a secret
```

Reference secrets in config with `${secret:NAME}` syntax.

---

### bc env

```bash
bc env set SHARED_VAR global                           # Workspace env
bc env set --provider claude CLAUDE_CODE_USE_BEDROCK 1 # Provider env
bc env list                                            # All env vars
bc env list --provider claude                          # Provider-specific
bc env get SHARED_VAR                                  # Get value
bc env unset SHARED_VAR                                # Remove
```

---

## Tools & Roles

### bc tool

```bash
bc tool list              # Show all tools with status
bc tool add myagent       # Add a custom tool
bc tool show claude       # Show tool details
bc tool setup claude      # Install and configure
bc tool status claude     # Check installation status
bc tool upgrade claude    # Upgrade an installed tool
bc tool delete mytool     # Remove a custom tool
bc tool run claude        # Run a tool directly
bc tool edit mytool       # Edit tool configuration
```

---

### bc role

```bash
bc role list              # List all roles
bc role show engineer     # Show role details
```

---

## Monitoring & Diagnostics

### bc logs
View the event log.

```bash
bc logs                     # Show all events
bc logs --agent eng-01      # Filter by agent
bc logs --type agent.report # Filter by event type
bc logs --since 1h          # Events from last hour
bc logs --tail 20           # Last N events
bc logs --full              # Show full messages
```

---

### bc doctor
Run health checks.

```bash
bc doctor                          # Full health check
bc doctor check workspace          # Check specific category
bc doctor fix                      # Auto-fix fixable issues
bc doctor fix --dry-run            # Preview fixes
bc doctor fix --category git       # Fix specific category
```

---

### bc version

```bash
bc version       # Show version info
bc --version     # Same as above
bc -V            # Same as above
```

---

### bc mcp
Manage MCP server configurations.

```bash
bc mcp list                                     # List all MCP servers
bc mcp add github --command npx --args "@modelcontextprotocol/server-github"
bc mcp add remote --transport sse --url "https://api.example.com/mcp"
bc mcp show github                              # Show server details
bc mcp remove github                            # Remove a server
bc mcp disable github                           # Disable a server
bc mcp enable github                            # Re-enable a server
bc mcp register                                 # Register bc as MCP server
bc mcp serve                                    # Start bc as MCP server
```

---

## Quick Reference

```bash
# Daily workflow
bc up                                    # Start root agent
bc status                                # Check status
bc agent create eng-01 --role engineer   # Create agent
bc agent send eng-01 "implement X"       # Send work
bc agent peek eng-01                     # Watch output
bc home                                  # Open dashboard
bc down                                  # Stop all

# Communication
bc channel send eng "starting tests"     # Channel message
bc agent broadcast "check status"        # Broadcast to all

# Monitoring
bc logs --tail 20                        # Recent events
bc doctor                                # Health check
bc cost summary                          # Cost overview
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

**For more help:** `bc --help` or `bc <command> --help`
