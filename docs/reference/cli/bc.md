## bc

A simpler, more controllable agent orchestrator

### Synopsis

bc is a multi-agent orchestration system for AI coding assistants.

Coordinate multiple AI agents with predictable behavior and cost awareness.
Supports Claude Code, Cursor, Codex, and other AI coding tools.

Getting Started:
  bc init                                 # Initialize workspace
  bc up                                   # Start root agent
  bc agent create eng-01 --role engineer  # Create engineer agent
  bc status                               # View agent status
  bc home                                 # Open TUI dashboard

Common Workflows:
  Start working:    bc up && bc status
  Monitor agents:   bc status --activity
  Notifications:    bc notify list
  Debug agent:      bc logs --agent eng-01 --tail 50
  Cost check:       bc cost show

Command Groups (with short aliases):
  agent                        Manage agents
  notify (nt)                  Notification subscriptions
  cost (co)                    Cost tracking and budgets
  workspace (ws)               Workspace management
  doctor (dr)                  Health checks
  demon (cr/cron)              Scheduled tasks

Key Features:
  • Coordinate multiple AI coding agents in parallel
  • Isolated git worktrees per agent
  • Platform notification routing to agents
  • Cost tracking and limits
  • Hierarchical agent roles (product-manager, manager, engineer)

Environment Variables:
  BC_AGENT_ID       Current agent name (set automatically in agent sessions)
  BC_AGENT_ROLE     Current agent role
  BC_WORKSPACE      Path to workspace root
  BC_AGENT_WORKTREE Path to agent's worktree
  BC_BIN            Path to bc binary (default: bc in PATH)
  BC_ROOT           Workspace root directory
  NO_COLOR          Disable colored output

Documentation: https://github.com/rpuneet/bc
Full CLI reference: https://github.com/rpuneet/bc/docs/cli.md

```
bc [flags]
```

### Options

```
  -h, --help      help for bc
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
  -V, --version   Print version information
```

### SEE ALSO

* [bc agent](bc_agent.md)	 - Manage bc agents
* [bc notify](bc_notify.md)	 - Manage notification subscriptions and sources
* [bc completion](bc_completion.md)	 - Generate shell completion scripts
* [bc config](bc_config.md)	 - Manage workspace configuration
* [bc cost](bc_cost.md)	 - Show cost information
* [bc cron](bc_cron.md)	 - Manage scheduled agent tasks
* [bc daemon](bc_daemon.md)	 - Manage workspace processes and the bcd server
* [bc doctor](bc_doctor.md)	 - Health checks and diagnostics
* [bc down](bc_down.md)	 - Stop bc agents
* [bc env](bc_env.md)	 - Manage workspace environment variables
* [bc home](bc_home.md)	 - Open the bc TUI dashboard
* [bc init](bc_init.md)	 - Initialize a new bc v2 workspace
* [bc logs](bc_logs.md)	 - Show the event log
* [bc mcp](bc_mcp.md)	 - Manage MCP server configurations
* [bc role](bc_role.md)	 - Manage agent roles in the workspace
* [bc secret](bc_secret.md)	 - Manage encrypted secrets
* [bc status](bc_status.md)	 - Show agent status
* [bc tool](bc_tool.md)	 - Manage AI tool providers
* [bc up](bc_up.md)	 - Start bc agents
* [bc version](bc_version.md)	 - Print version information
* [bc workspace](bc_workspace.md)	 - Manage bc workspaces

