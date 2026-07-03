## mycel workspace switch

Switch active workspace

### Synopsis

Set the active workspace for cross-workspace operations.

Examples:
  mycel workspace switch fe                    # Switch by alias
  mycel workspace switch ~/projects/frontend   # Switch by path
  mycel workspace switch --clear               # Clear active workspace

```
mycel workspace switch <alias|path> [flags]
```

### Options

```
      --clear   Clear active workspace
  -h, --help    help for switch
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel workspace](mycel_workspace.md)	 - Manage mycel workspaces

