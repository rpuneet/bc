## mycel agent peek

Show recent output from an agent

### Synopsis

Capture and display recent output from an agent's session.

Examples:
  mycel agent peek eng-01              # Show last 500 lines
  mycel agent peek eng-01 --lines 100  # Show last 100 lines
  mycel agent peek eng-01 --follow     # Stream live output (Ctrl+C to stop)

```
mycel agent peek <agent> [flags]
```

### Options

```
  -f, --follow      Stream live output (like tail -f)
  -h, --help        help for peek
      --lines int   Number of lines to show (default 500)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel agent](mycel_agent.md)	 - Manage mycel agents

