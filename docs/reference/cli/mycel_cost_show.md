## mycel cost show

Show cost records

### Synopsis

Show cost records, optionally filtered by agent.

You can specify the agent either as a positional argument or using --agent flag.

Examples:
  mycel cost show
  mycel cost show engineer-01
  mycel cost show --agent engineer-01

```
mycel cost show [agent] [flags]
```

### Options

```
      --agent string   Filter by agent (alternative to positional argument)
  -h, --help           help for show
  -n, --limit int      Number of records to show (default 20)
      --offset int     Number of records to skip (for pagination)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel cost](mycel_cost.md)	 - Show cost information

