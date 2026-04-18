## bc status

Show agent status

### Synopsis

Show the status of all bc agents.

Examples:
  bc status                   # Show all agents
  bc status --json            # Output as JSON
  bc status --activity        # Show recent notification activity

Output:
  AGENT     ROLE      STATE    UPTIME    TASK
  eng-01    engineer  working  2h 15m    Implementing feature X
  eng-02    engineer  idle     1h 30m    -

Agent States:
  working   Agent is actively processing
  idle      Agent is waiting for input
  done      Agent has completed task
  error     Agent encountered an error
  stopped   Agent is not running

See Also:
  bc agent list   List agents with more detail
  bc logs         View agent event logs
  bc home         Open TUI dashboard

```
bc status [flags]
```

### Options

```
      --activity   Show recent notification activity
  -h, --help       help for status
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [bc](bc.md)	 - A simpler, more controllable agent orchestrator

