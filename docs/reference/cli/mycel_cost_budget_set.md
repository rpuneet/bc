## mycel cost budget set

Set a cost budget

### Synopsis

Set a cost budget for the repo, agent, or team.

Examples:
  mycel cost budget set 100.00                          # Set the fleet-wide budget to $100
  mycel cost budget set 50.00 --agent engineer-01       # Set agent budget
  mycel cost budget set 500.00 --team engineering       # Set team budget
  mycel cost budget set 100.00 --period weekly          # Weekly budget
  mycel cost budget set 100.00 --alert-at 0.9           # Alert at 90%
  mycel cost budget set 100.00 --hard-stop              # Stop when limit reached

```
mycel cost budget set <amount> [flags]
```

### Options

```
      --agent string     Set budget for specific agent
      --alert-at float   Alert when usage reaches this percentage (0.0-1.0) (default 0.8)
      --hard-stop        Stop operations when budget is exceeded
  -h, --help             help for set
      --period string    Budget period (daily, weekly, monthly) (default "monthly")
      --team string      Set budget for specific team
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel cost budget](mycel_cost_budget.md)	 - Manage cost budgets

