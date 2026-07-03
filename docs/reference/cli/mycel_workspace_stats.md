## mycel workspace stats

Show workspace statistics

### Synopsis

Display statistics about the current workspace including work item
metrics, agent utilization, and completion rates.

Examples:
  mycel workspace stats             # human-readable summary
  mycel workspace stats --json      # JSON output for scripting
  mycel workspace stats --save      # save stats snapshot to .bc/stats.json

```
mycel workspace stats [flags]
```

### Options

```
  -h, --help   help for stats
      --json   Output as JSON
      --save   Save stats snapshot to disk
```

### Options inherited from parent commands

```
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel workspace](mycel_workspace.md)	 - Manage mycel workspaces

