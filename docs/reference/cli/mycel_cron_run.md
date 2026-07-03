## mycel cron run

Manually trigger a cron job

### Synopsis

Trigger a cron job immediately outside its normal schedule.
The job must be enabled. The daemon (bcd) executes the actual agent interaction;
this command records the trigger and updates run stats.

Examples:
  mycel cron run daily-lint

```
mycel cron run <name> [flags]
```

### Options

```
  -h, --help   help for run
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel cron](mycel_cron.md)	 - Manage scheduled agent tasks

