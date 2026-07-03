## mycel workspace list

List discovered workspaces

### Synopsis

List all mycel workspaces on this machine.

Searches:
  - Global registry (~/.mycel/workspaces.json)
  - Common directories (~/Projects, ~/Developer, ~/dev, ~/code, ~/repos, ~/src)
  - Additional paths specified with --scan

Examples:
  mycel workspace list                    # List all workspaces
  mycel workspace list --json             # Output as JSON
  mycel workspace list --scan ~/work      # Include additional path
  mycel workspace list --no-cache         # Skip registry, scan only

```
mycel workspace list [flags]
```

### Options

```
      --depth int      Maximum scan depth (default 3)
  -h, --help           help for list
      --no-cache       Skip registry, scan filesystem only
      --scan strings   Additional paths to scan
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel workspace](mycel_workspace.md)	 - Manage mycel workspaces

