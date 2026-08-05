## mycel notify prune

Remove leftover catch-all auto-copied subscriptions

### Synopsis

List (and optionally delete) per-channel subscriptions that look like
copies of a platform catch-all ("{platform}:*") row.

Before #3464, catch-all delivery wrote permanent rows onto every real channel.
Those rows have no provenance, so prune uses a heuristic: same agent and
mention_only as an existing catch-all subscription. Deliberate subscriptions
that happen to match are included — review the list before confirming.

Examples:
  mycel notify prune --dry-run
  mycel notify prune --platform gmail
  mycel notify prune --yes

```
mycel notify prune [flags]
```

### Options

```
      --dry-run           List candidates without deleting
  -h, --help              help for prune
      --json              Output candidates as JSON
      --platform string   Only consider channels for this platform (e.g. gmail)
      --yes               Delete without interactive confirmation
```

### Options inherited from parent commands

```
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel notify](mycel_notify.md)	 - Manage channel subscriptions and gateway notifications

