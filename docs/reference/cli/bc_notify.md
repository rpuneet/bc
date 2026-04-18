## bc notify

Manage notification subscriptions and sources

### Synopsis

Manage how agents receive notifications from external platforms (Slack, Telegram, GitHub, and others).

Notification sources are platform channels discovered by connected gateway adapters.
Agents subscribe to sources and receive inbound events in their tmux/Docker sessions.

Examples:
  bc notify list                                       # List all notification sources
  bc notify subscribe slack:engineering eng-01          # Subscribe agent to Slack channel
  bc notify subscribe --mention-only github:bc lead-01  # Only @mentions
  bc notify unsubscribe slack:engineering eng-01        # Remove subscription
  bc notify status                                     # Show adapter connection health
  bc notify history slack:engineering --last 20         # Recent notifications

Notification Flow:
  External platform -> Gateway adapter -> Notify service -> Subscribed agents

Sources use the format `platform:channel`:
  slack:engineering     Slack #engineering channel
  telegram:bc-dev       Telegram bc-dev group
  github:bc             GitHub bc repository events

See Also:
  bc agent send       Send message directly to an agent
  bc agent broadcast  Send to all agents
  bc status           View agents and notification activity

### Options

```
  -h, --help   help for notify
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [bc](bc.md)	 - A simpler, more controllable agent orchestrator
* [bc notify history](bc_notify_history.md)	 - Show recent notifications for a source
* [bc notify list](bc_notify_list.md)	 - List all notification sources
* [bc notify status](bc_notify_status.md)	 - Show adapter connection status
* [bc notify subscribe](bc_notify_subscribe.md)	 - Subscribe an agent to a notification source
* [bc notify unsubscribe](bc_notify_unsubscribe.md)	 - Unsubscribe an agent from a notification source
