## mycel status

Show agent status

### Synopsis

Show the status of all bc agents.

Examples:
  mycel status                   # Show all agents
  mycel status --json            # Output as JSON
  mycel status --activity        # Show recent channel activity

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
  mycel agent list   List agents with more detail
  mycel logs         View agent event logs
  mycel home         Open TUI dashboard

```
mycel status [flags]
```

### Options

```
      --activity   Show recent channel activity
  -h, --help       help for status
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator

