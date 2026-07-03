## mycel workspace remove

Remove a workspace from the registry

### Synopsis

Unregister a workspace from the global registry.

This does not delete the workspace, just removes it from the registry.

Examples:
  mycel workspace remove fe                    # Remove by alias
  mycel workspace remove ~/projects/frontend   # Remove by path

```
mycel workspace remove <alias|path> [flags]
```

### Options

```
  -h, --help   help for remove
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel workspace](mycel_workspace.md)	 - Manage mycel workspaces

