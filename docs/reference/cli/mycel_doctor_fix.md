## mycel doctor fix

Auto-fix fixable issues

### Synopsis

Attempt to automatically repair fixable issues found by 'mycel doctor'.

Fixable issues include:
  - Orphaned git worktrees
  - Missing ~/.mycel directories

Use --dry-run to preview actions without making changes.

Examples:
  mycel doctor fix                      # Fix all fixable issues
  mycel doctor fix --dry-run            # Preview fixes
  mycel doctor fix --category git       # Fix specific category

```
mycel doctor fix [flags]
```

### Options

```
      --category string   Fix only the specified category
      --dry-run           Preview fixes without making changes
  -h, --help              help for fix
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel doctor](mycel_doctor.md)	 - Health checks and diagnostics

