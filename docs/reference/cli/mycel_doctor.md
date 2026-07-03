## mycel doctor

Health checks and diagnostics

### Synopsis

Run health checks on your mycel workspace and dependencies.

Checks workspace config, agent state, databases, tools, and git worktrees.

Categories:
  workspace   state directory, preferences.json, role files
  database    SQLite integrity and table existence
  agents      Running agents, stale sessions, missing worktrees
  tools       tmux, git, and AI provider installations
  git         Worktree validity and orphaned worktrees

Examples:
  mycel doctor                          # Full health check
  mycel doctor check workspace          # Check specific category
  mycel doctor fix                      # Auto-fix fixable issues
  mycel doctor fix --dry-run            # Preview fixes
  mycel doctor fix --category git       # Fix specific category

Exit codes:
  0  All checks passed or only warnings
  1  One or more checks failed

```
mycel doctor [flags]
```

### Options

```
  -h, --help   help for doctor
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator
* [mycel doctor check](mycel_doctor_check.md)	 - Check a specific health category
* [mycel doctor fix](mycel_doctor_fix.md)	 - Auto-fix fixable issues

