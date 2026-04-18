## bc notify status

Show adapter connection status and health

### Synopsis

Display the connection status of all gateway adapters, including whether they are connected, any errors, last message timestamps, and total message counts.

### Examples

```bash
bc notify status                # Show all adapter statuses
bc notify status --json         # JSON output
```

### Description

Reports the health and connectivity of every registered gateway adapter. Use this to verify that platform connections are active and diagnose connection failures.

Output columns:
- Gateway name (e.g., `slack`, `telegram:trade`)
- Connection status (connected / disconnected / error)
- Last message received timestamp
- Total messages processed
- Error details (if any)

```
bc notify status [flags]
```

### Options

```
  -h, --help   help for status
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### See Also

- [bc notify](bc_notify.md) - Manage notification subscriptions and sources
