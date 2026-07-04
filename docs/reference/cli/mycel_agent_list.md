## mycel agent list

List all agents

### Synopsis

List all agents with their status, role, and current task.

Examples:
  mycel agent list          # List all agents
  mycel agent list --json   # Output as JSON
  mycel agent list --role engineer  # Filter by role

```
mycel agent list [flags]
```

### Options

```
      --full            Include full agent data including prompts (with --json)
  -h, --help            help for list
      --json            Output as JSON (compact by default)
      --role string     Filter by role
      --status string   Filter by status (running, stopped, error)
```

### Options inherited from parent commands

```
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel agent](mycel_agent.md)	 - Manage mycel agents

