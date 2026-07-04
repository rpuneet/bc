## mycel agent stop

Stop an agent

### Synopsis

Stop a specific agent and its tmux session.

Examples:
  mycel agent stop eng-01       # Stop eng-01
  mycel agent stop eng-01 --force  # Force stop

```
mycel agent stop <agent> [flags]
```

### Options

```
      --force   Force stop without cleanup
  -h, --help    help for stop
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel agent](mycel_agent.md)	 - Manage mycel agents

