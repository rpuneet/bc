## bc notify history

Show recent notifications for a source

### Synopsis

Display the history of notifications received from a specific notification source. Shows inbound platform events with timestamps and delivery status.

### Examples

```bash
bc notify history slack:engineering                    # Last 50 notifications (default)
bc notify history slack:engineering --limit 10         # Last 10
bc notify history slack:engineering --since 1h         # From the last hour
bc notify history telegram:bc-dev --limit 20           # Telegram group history
```

### Description

Retrieves recent notifications from the notification log for a given source. Each entry shows the sender, timestamp, and a preview of the notification content. Delivery status indicates whether the notification was successfully dispatched to subscribed agents.

```
bc notify history <source> [flags]
```

### Options

```
  -h, --help           help for history
      --last int       Show last N notifications (alias for --limit)
      --limit int      Maximum number of notifications to show (default 50)
      --since string   Show notifications since duration (e.g., 1h, 30m)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### See Also

- [bc notify](bc_notify.md) - Manage notification subscriptions and sources
