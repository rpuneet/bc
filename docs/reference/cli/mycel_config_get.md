## mycel config get

Get a configuration value

### Synopsis

Get a specific configuration value using dot notation.

Examples:
  mycel config get workspace.name
  mycel config get providers.default
  mycel config get providers.claude.command
  mycel config get tools.claude.command

```
mycel config get <key> [flags]
```

### Options

```
  -h, --help   help for get
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel config](mycel_config.md)	 - Manage workspace configuration

