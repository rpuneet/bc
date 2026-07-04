## mycel cron logs

Show execution history for a cron job

### Synopsis

Display the execution log for a cron job.

Examples:
  mycel cron logs daily-lint
  mycel cron logs daily-lint --last 5
  mycel cron logs daily-lint --json

```
mycel cron logs <name> [flags]
```

### Options

```
  -h, --help       help for logs
      --json       Output as JSON
      --last int   Number of entries to show (default 20)
```

### Options inherited from parent commands

```
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel cron](mycel_cron.md)	 - Manage scheduled agent tasks

