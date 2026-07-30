## mycel cost

Show cost information

### Synopsis

Commands for viewing API cost information.

Shows Claude Code token usage, costs, and budget management.

Examples:
  mycel cost                              # Show cost records (default)
  mycel cost show eng-01                  # Show costs for specific agent
  mycel cost usage                        # Claude Code usage via ccusage
  mycel cost usage --monthly              # Monthly summary
  mycel cost budget show                  # Show budget status

See Also:
  Web UI (http://localhost:9374)  Costs dashboard
  mycel status         Agent status (includes cost info)

```
mycel cost [flags]
```

### Options

```
  -h, --help   help for cost
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator
* [mycel cost agent](mycel_cost_agent.md)	 - Show per-agent cost breakdown
* [mycel cost budget](mycel_cost_budget.md)	 - Manage cost budgets
* [mycel cost daily](mycel_cost_daily.md)	 - Show daily cost totals
* [mycel cost dashboard](mycel_cost_dashboard.md)	 - Show rich cost dashboard
* [mycel cost model](mycel_cost_model.md)	 - Show per-model cost breakdown
* [mycel cost report](mycel_cost_report.md)	 - Report cost totals across repos
* [mycel cost show](mycel_cost_show.md)	 - Show cost records
* [mycel cost summary](mycel_cost_summary.md)	 - Show repo cost overview
* [mycel cost usage](mycel_cost_usage.md)	 - Show Claude Code token usage via ccusage

