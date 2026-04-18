## bc notify subscribe

Subscribe an agent to a notification source

### Synopsis

Subscribe an agent to receive notifications from an external platform source. The agent receives inbound events in its tmux or Docker session as JSON payloads.

### Examples

```bash
bc notify subscribe slack:engineering eng-01                  # All messages
bc notify subscribe --mention-only slack:engineering eng-01   # Only @mentions
bc notify subscribe github:bc lead-01                        # GitHub events
bc notify subscribe telegram:bc-dev eng-02                   # Telegram group
```

### Description

Creates a subscription linking an agent to a notification source. Once subscribed, the agent receives platform events dispatched by the notification service.

Use `--mention-only` to filter notifications so the agent only receives events where it is explicitly @mentioned. This is useful for noisy channels where the agent should only respond when addressed directly.

The source must be a valid `platform:channel` identifier discovered by a connected gateway adapter. Use `bc notify list` to see available sources.

```
bc notify subscribe <source> <agent> [flags]
```

### Options

```
  -h, --help           help for subscribe
      --mention-only   Only notify when the agent is @mentioned
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### See Also

- [bc notify](bc_notify.md) - Manage notification subscriptions and sources
