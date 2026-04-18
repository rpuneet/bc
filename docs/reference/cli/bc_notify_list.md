## bc notify list

List all notification sources with subscriber counts

### Synopsis

Display all notification sources discovered by connected gateway adapters, along with the number of agents subscribed to each source.

Sources are grouped by platform and show their subscriber count. Use `--json` for machine-readable output.

### Examples

```bash
bc notify list                  # List all sources
bc notify list --json           # JSON output
```

### Description

Lists every notification source across all connected gateways. Each source represents a channel or event stream on an external platform. The output includes:

- Source name (format: `platform:channel`)
- Platform (slack, telegram, github, etc.)
- Number of subscribed agents
- Gateway connection status

```
bc notify list [flags]
```

### Options

```
  -h, --help   help for list
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### See Also

- [bc notify](bc_notify.md) - Manage notification subscriptions and sources
