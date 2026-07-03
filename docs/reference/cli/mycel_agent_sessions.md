## mycel agent sessions

List session history for an agent

### Synopsis

Show stored session IDs for an agent.

The current session ID (if captured) is listed first, followed by archived
session IDs from previous runs.

Examples:
  mycel agent sessions eng-01       # List session IDs
  mycel agent sessions eng-01 --json

```
mycel agent sessions <agent> [flags]
```

### Options

```
  -h, --help   help for sessions
      --json   Output as JSON
```

### Options inherited from parent commands

```
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel agent](mycel_agent.md)	 - Manage mycel agents

