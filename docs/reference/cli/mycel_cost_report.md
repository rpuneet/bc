## mycel cost report

Report cost totals across repos

### Synopsis

Report cost totals from the user-global cost ledger (~/.mycel/costs.db).

By default prints per-repo breakdown. Use --by to change grouping:

  mycel cost report                  # per-repo totals
  mycel cost report --by repo        # per-repo totals
  mycel cost report --by project     # per-project totals (repo name grouping)
  mycel cost report --since 30d      # only include records from last 30 days

```
mycel cost report [flags]
```

### Options

```
      --by string      Grouping: repo | project (default "repo")
  -h, --help           help for report
      --since string   Include records since (e.g. 7d, 30d, 2026-01-01)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel cost](mycel_cost.md)	 - Show cost information

