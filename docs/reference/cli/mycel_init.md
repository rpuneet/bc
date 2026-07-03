## mycel init

Initialize a new mycel v2 workspace

### Synopsis

Initialize a new mycel v2 workspace in the specified directory (or current directory).

This creates a .mycel-scoped workspace directory with v2 configuration for managing agents.

v2 workspace structure (runtime state lives outside the project dir):
  ~/.mycel/workspaces/<id>/
    preferences.json  # Workspace configuration
    roles/            # Agent role definitions
      root.md         # Root agent role
    agents/           # Per-agent state files

Examples:
  mycel init                        # Interactive wizard
  mycel init --quick                # Quick init with defaults
  mycel init --preset solo          # Use solo developer preset
  mycel init --preset small-team    # Use small team preset
  mycel init --preset full-team     # Use full team preset
  mycel init ~/Projects/myapp       # Initialize specific directory

```
mycel init [directory] [flags]
```

### Options

```
  -h, --help            help for init
      --preset string   Use preset configuration (solo, small-team, full-team)
      --quick           Quick init with defaults (skip wizard)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator

