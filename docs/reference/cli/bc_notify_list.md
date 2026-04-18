## bc notify list

List all agent subscriptions

### Synopsis

Display all agent subscriptions across all notification channels. Each subscription maps an agent to a channel (format: `platform:channel_name`) with an optional `@mention only` filter.

### Examples

```bash
bc notify list                  # List all subscriptions
bc notify list --json           # JSON output
```

### Description

Lists every agent subscription. The output includes:

- Channel name (format: `platform:channel_name`, e.g., `slack:engineering`)
- Subscribed agent name
- Whether `@mention only` filtering is enabled

```
bc notify list [flags]
```

### Options

```
      --json   Output as JSON
  -h, --help   help for list
```

### Options inherited from parent commands

```
  -v, --verbose   Enable verbose output
```

### See Also

- [bc notify](bc_notify.md) - Manage notification subscriptions and sources
