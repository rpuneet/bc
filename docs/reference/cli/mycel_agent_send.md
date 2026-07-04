## mycel agent send

Send a message to an agent

### Synopsis

Send a message or command to an agent's session.

Use --preview to see what action will be taken before sending (Intent Preview).
This shows agent details and asks for confirmation.

Examples:
  mycel agent send eng-01 "run the tests"
  mycel agent send coordinator "check status"
  mycel agent send eng-01 "implement login" --preview  # Preview before sending

```
mycel agent send <agent> <message> [flags]
```

### Options

```
  -h, --help      help for send
      --preview   Show preview of action before sending (Intent Preview)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel agent](mycel_agent.md)	 - Manage mycel agents

