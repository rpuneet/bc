## mycel

A simpler, more controllable agent orchestrator

### Synopsis

mycel is a multi-agent orchestration system for AI coding assistants.

Coordinate multiple AI agents with predictable behavior and cost awareness.
Supports Claude Code, Cursor, Codex, and other AI coding tools.

Getting Started:
  mycel up                                   # Start the server (bootstraps ~/.mycel)
  mycel agent create eng-01 --role engineer  # Create engineer agent
  mycel status                               # View agent status

Common Workflows:
  Start working:    mycel up && mycel status
  Monitor agents:   mycel status --activity
  Send message:     mycel channel send eng "message"
  Debug agent:      mycel logs --agent eng-01 --tail 50
  Cost check:       mycel cost show

Command Groups (with short aliases):
  agent                        Manage agents
  channel (ch)                 Communication channels
  cost (co)                    Cost tracking and budgets
  config                       Configuration management
  doctor (dr)                  Health checks

Key Features:
  • Coordinate multiple AI coding agents in parallel
  • Isolated git worktrees per agent
  • Channel-based agent communication
  • Cost tracking and limits
  • Hierarchical agent roles (product-manager, manager, engineer)

Environment Variables:
  MYCEL_AGENT_ID       Current agent name (set automatically in agent sessions)
  MYCEL_AGENT_ROLE     Current agent role
  MYCEL_WORKSPACE      Path to the agent's repo root
  MYCEL_AGENT_WORKTREE Path to agent's worktree
  MYCEL_BIN            Path to mycel binary (default: mycel in PATH)
  MYCEL_ROOT           Override the mycel home root directory
  NO_COLOR          Disable colored output

Documentation: https://github.com/rpuneet/mycel
Full CLI reference: https://github.com/rpuneet/mycel/docs/cli.md

```
mycel [flags]
```

### Options

```
  -h, --help      help for mycel
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
  -V, --version   Print version information
```

### SEE ALSO

* [mycel agent](mycel_agent.md)	 - Manage mycel agents
* [mycel app](mycel_app.md)	 - Manage app (gateway plugin) integrations
* [mycel channel](mycel_channel.md)	 - Manage communication channels
* [mycel completion](mycel_completion.md)	 - Generate shell completion scripts
* [mycel config](mycel_config.md)	 - Manage repo configuration
* [mycel cost](mycel_cost.md)	 - Show cost information
* [mycel doctor](mycel_doctor.md)	 - Health checks and diagnostics
* [mycel down](mycel_down.md)	 - Stop mycel services
* [mycel logs](mycel_logs.md)	 - Show the event log
* [mycel mcp](mycel_mcp.md)	 - Manage MCP server configurations
* [mycel notify](mycel_notify.md)	 - Manage channel subscriptions and gateway notifications
* [mycel secret](mycel_secret.md)	 - Manage encrypted secrets
* [mycel stats](mycel_stats.md)	 - Show repo statistics
* [mycel status](mycel_status.md)	 - Show agent status
* [mycel template](mycel_template.md)	 - Manage agent templates
* [mycel tool](mycel_tool.md)	 - Manage AI tool providers
* [mycel up](mycel_up.md)	 - Start mycel server
* [mycel version](mycel_version.md)	 - Print version information

