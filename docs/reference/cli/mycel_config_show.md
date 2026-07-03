## mycel config show

Show configuration

### Synopsis

Display the current workspace configuration.

If a key is specified, shows only that section. Otherwise shows entire config.

Examples:
  mycel config show                  # Show all config
  mycel config show tools            # Show tools section
  mycel config show tools.claude     # Show specific tool config
  mycel config show --json           # Output as JSON

```
mycel config show [key] [flags]
```

### Options

```
  -h, --help   help for show
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel config](mycel_config.md)	 - Manage workspace configuration

