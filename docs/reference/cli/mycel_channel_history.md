## mycel channel history

Show channel message history

### Synopsis

Display the history of messages sent to a channel.

Examples:
  mycel channel history eng                       # Last 50 messages (default)
  mycel channel history eng --limit 10            # Last 10 messages
  mycel channel history eng --since 1h            # Messages from last hour
  mycel channel history eng --agent agent-core    # Messages from agent-core only
  mycel channel history eng --from 2026-03-01     # Messages from date
  mycel channel history eng --from 2026-03-01 --to 2026-03-05  # Date range
  mycel channel history eng --limit 20 --offset 20  # Page 2 of 20

```
mycel channel history <channel> [flags]
```

### Options

```
      --agent string   Filter messages by sender agent
      --from string    Show messages from timestamp (RFC3339 or 2006-01-02)
  -h, --help           help for history
      --last int       Show last N messages (alias for --limit)
      --limit int      Maximum number of messages to show (default 50)
      --offset int     Number of messages to skip
      --since string   Show messages since duration (e.g., 1h, 30m)
      --to string      Show messages until timestamp (RFC3339 or 2006-01-02)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel channel](mycel_channel.md)	 - Manage communication channels

