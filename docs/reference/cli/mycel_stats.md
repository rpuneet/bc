## mycel stats

Show workspace statistics

### Synopsis

Display statistics about the current repo's agents including work
item metrics, agent utilization, and completion rates.

Examples:
  mycel stats             # human-readable summary
  mycel stats --json      # JSON output for scripting
  mycel stats --save      # save stats snapshot to the state dir

```
mycel stats [flags]
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

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator

