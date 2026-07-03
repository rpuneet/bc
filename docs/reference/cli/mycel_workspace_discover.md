## mycel workspace discover

Discover and register workspaces

### Synopsis

Scan filesystem for mycel workspaces and add them to the registry.

This updates ~/.bc/workspaces.json with newly found workspaces.

Examples:
  mycel workspace discover                # Scan default locations
  mycel workspace discover --scan ~/work  # Include additional path

```
mycel workspace discover [flags]
```

### Options

```
      --depth int      Maximum scan depth (default 3)
  -h, --help           help for discover
      --scan strings   Additional paths to scan
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel workspace](mycel_workspace.md)	 - Manage mycel workspaces

