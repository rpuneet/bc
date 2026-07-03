## mycel workspace add

Add a workspace to the registry

### Synopsis

Register a workspace in the global registry for quick access.

Examples:
  mycel workspace add .                        # Add current directory
  mycel workspace add ~/projects/frontend      # Add by path
  mycel workspace add . --alias fe             # Add with short alias
  mycel workspace add ~/api --alias backend    # Add with alias

```
mycel workspace add <path> [flags]
```

### Options

```
      --alias string   Short alias for quick access
  -h, --help           help for add
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel workspace](mycel_workspace.md)	 - Manage mycel workspaces

