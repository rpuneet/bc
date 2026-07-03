## mycel cost usage

Show Claude Code token usage via ccusage

### Synopsis

Show Claude Code token usage and cost analytics via ccusage.

Wraps the ccusage tool (https://github.com/ryoppippi/ccusage) to display
detailed token usage, per-model cost breakdown, and cache analytics from
Claude Code's local JSONL session files.

Requires npx (Node.js) to be available on the system.

Examples:
  mycel cost usage                        # Daily usage report
  mycel cost usage --monthly              # Monthly summary
  mycel cost usage --session              # Per-session breakdown
  mycel cost usage --since 20260301       # Usage since date (YYYYMMDD)
  mycel cost usage --until 20260301       # Usage until date (YYYYMMDD)
  mycel cost usage --json                 # Raw JSON output

```
mycel cost usage [flags]
```

### Options

```
  -h, --help           help for usage
      --monthly        Show monthly summary
      --refresh        Force refresh cached data
      --session        Show per-session breakdown
      --since string   Filter from date (YYYYMMDD)
      --until string   Filter until date (YYYYMMDD)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel cost](mycel_cost.md)	 - Show cost information

