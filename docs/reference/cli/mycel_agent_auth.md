## mycel agent auth

Authenticate an agent for Docker containers

### Synopsis

Run OAuth login for a specific agent. Each agent has its own isolated
credentials directory. Opens a browser for authentication.

Usage:
  mycel agent auth my-agent        # Login for a specific agent
  mycel agent auth my-agent status # Check auth status

```
mycel agent auth <agent-name> [flags]
```

### Options

```
  -h, --help   help for auth
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel agent](mycel_agent.md)	 - Manage mycel agents

