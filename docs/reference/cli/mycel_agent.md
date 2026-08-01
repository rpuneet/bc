## mycel agent

Manage mycel agents

### Synopsis

Manage mycel agent lifecycle: create, list, attach, peek, stop, send.

Examples:
  mycel agent list                          # List all agents
  mycel agent create eng-01 --template engineer # Create new agent
  mycel agent attach eng-01                 # Attach to agent session
  mycel agent peek eng-01                   # View recent output
  mycel agent send eng-01 "run tests"       # Send message to agent
  mycel agent stop eng-01                   # Stop agent
  mycel agent broadcast "check status"      # Send to all agents
  mycel agent send-pattern "eng-*" "test"   # Send to matching agents
  mycel agent                               # List all agents (same as mycel agent list)
  mycel agent send-pattern "eng-*" "hello"  # Send to matching agents

```
mycel agent [flags]
```

### Options

```
  -h, --help   help for agent
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator
* [mycel agent attach](mycel_agent_attach.md)	 - Attach to an agent's session
* [mycel agent auth](mycel_agent_auth.md)	 - Authenticate an agent for Docker containers
* [mycel agent avatar](mycel_agent_avatar.md)	 - Generate agent AgentCharacter avatar images (for public hosting)
* [mycel agent broadcast](mycel_agent_broadcast.md)	 - Send a message to all running agents
* [mycel agent cost](mycel_agent_cost.md)	 - Show per-agent cost breakdown
* [mycel agent create](mycel_agent_create.md)	 - Create a new agent
* [mycel agent delete](mycel_agent_delete.md)	 - Permanently delete an agent
* [mycel agent health](mycel_agent_health.md)	 - Check agent health status
* [mycel agent list](mycel_agent_list.md)	 - List all agents
* [mycel agent logs](mycel_agent_logs.md)	 - Show agent event history
* [mycel agent peek](mycel_agent_peek.md)	 - Show recent output from an agent
* [mycel agent rename](mycel_agent_rename.md)	 - Rename an agent
* [mycel agent report](mycel_agent_report.md)	 - Report agent state (called by agents)
* [mycel agent send](mycel_agent_send.md)	 - Send a message to an agent
* [mycel agent send-pattern](mycel_agent_send-pattern.md)	 - Send a message to agents matching a pattern
* [mycel agent sessions](mycel_agent_sessions.md)	 - List session history for an agent
* [mycel agent show](mycel_agent_show.md)	 - Show agent details
* [mycel agent start](mycel_agent_start.md)	 - Start a stopped agent
* [mycel agent stats](mycel_agent_stats.md)	 - Show Docker resource stats for an agent
* [mycel agent stop](mycel_agent_stop.md)	 - Stop an agent

