## bc notify unsubscribe

Unsubscribe an agent from a notification source

### Synopsis

Remove an agent's subscription to a notification source. The agent stops receiving inbound events from that source immediately.

### Examples

```bash
bc notify unsubscribe slack:engineering eng-01       # Remove subscription
bc notify unsubscribe github:bc lead-01              # Stop GitHub events
```

### Description

Deletes the subscription record for an agent on the specified notification source. The agent no longer receives dispatched events from that source. Existing notification history is retained in the notification log.

```
bc notify unsubscribe <source> <agent> [flags]
```

### Options

```
  -h, --help   help for unsubscribe
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### See Also

- [bc notify](bc_notify.md) - Manage notification subscriptions and sources
