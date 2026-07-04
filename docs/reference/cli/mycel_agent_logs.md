## mycel agent logs

Show agent event history

### Synopsis

Show the event log history for a specific agent.

Examples:
  mycel agent logs eng-01               # Show all events
  mycel agent logs eng-01 --since 1h    # Show events from last hour

```
mycel agent logs <agent> [flags]
```

### Options

```
  -h, --help           help for logs
      --since string   Show events since duration (e.g., 1h, 30m)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel agent](mycel_agent.md)	 - Manage mycel agents

