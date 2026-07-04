## mycel agent stats

Show Docker resource stats for an agent

### Synopsis

Display recorded Docker CPU and memory stats for an agent.

Stats are collected every 30 s by bcd while the agent is running with a
Docker runtime backend. They are stored in .bc/bc.db.

Examples:
  mycel agent stats eng-01              # Human-readable table
  mycel agent stats eng-01 --json       # JSON output
  mycel agent stats eng-01 --limit 50   # Show more records

```
mycel agent stats <name> [flags]
```

### Options

```
  -h, --help        help for stats
      --json        Output as JSON
      --limit int   Number of records to show (default 20)
```

### Options inherited from parent commands

```
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel agent](mycel_agent.md)	 - Manage mycel agents

