## mycel cron

Manage scheduled agent tasks

### Synopsis

Manage cron jobs that trigger agent prompts or shell commands on a schedule.

Cron expressions use standard 5-field format:
  ┌────── minute (0-59)
  │ ┌──── hour (0-23)
  │ │ ┌── day of month (1-31)
  │ │ │ ┌ month (1-12)
  │ │ │ │ ┌ day of week (0-6, 0=Sun)
  * * * * *

Examples:
  mycel cron add daily-lint --schedule "0 9 * * *" --agent qa-01 --prompt "Run make lint"
  mycel cron list                          # List all cron jobs
  mycel cron show daily-lint               # Show job details
  mycel cron enable daily-lint             # Enable a disabled job
  mycel cron disable daily-lint            # Disable without deleting
  mycel cron run daily-lint                # Trigger manually
  mycel cron logs daily-lint --last 10     # Show last 10 executions
  mycel cron remove daily-lint             # Delete a job

### Options

```
  -h, --help   help for cron
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator
* [mycel cron add](mycel_cron_add.md)	 - Add a new cron job
* [mycel cron disable](mycel_cron_disable.md)	 - Disable a cron job
* [mycel cron enable](mycel_cron_enable.md)	 - Enable a cron job
* [mycel cron list](mycel_cron_list.md)	 - List all cron jobs
* [mycel cron logs](mycel_cron_logs.md)	 - Show execution history for a cron job
* [mycel cron remove](mycel_cron_remove.md)	 - Remove a cron job
* [mycel cron run](mycel_cron_run.md)	 - Manually trigger a cron job
* [mycel cron show](mycel_cron_show.md)	 - Show cron job details

