## mycel agent rename

Rename an agent

### Synopsis

Rename an agent to a new name.

This updates the agent's name and channel memberships.
By default, running agents cannot be renamed (use --force to override).

Examples:
  mycel agent rename eng-01 engineer-01
  mycel agent rename eng-01 eng-02 --force  # Rename running agent

```
mycel agent rename <old-name> <new-name> [flags]
```

### Options

```
      --force   Rename even if agent is running
  -h, --help    help for rename
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel agent](mycel_agent.md)	 - Manage mycel agents

