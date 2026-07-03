## mycel notify

Manage channel subscriptions and gateway notifications

### Synopsis

Manage agent subscriptions to gateway channels (Slack, Telegram, Discord).

Channels deliver external app messages to subscribed agents via tmux send-keys.
Agents respond using the platform's own MCP tools.

Examples:
  mycel notify status                               # Show gateway connection status
  mycel notify list                                  # List all subscriptions
  mycel notify subscribe slack:eng eng-01            # Subscribe agent to channel
  mycel notify unsubscribe slack:eng eng-01          # Unsubscribe agent
  mycel notify activity slack:eng                    # Show delivery activity log

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

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator
* [mycel notify activity](mycel_notify_activity.md)	 - Show delivery activity for a channel
* [mycel notify list](mycel_notify_list.md)	 - List all agent subscriptions
* [mycel notify status](mycel_notify_status.md)	 - Show gateway connection status and subscriptions
* [mycel notify subscribe](mycel_notify_subscribe.md)	 - Subscribe an agent to a channel
* [mycel notify unsubscribe](mycel_notify_unsubscribe.md)	 - Unsubscribe an agent from a channel

