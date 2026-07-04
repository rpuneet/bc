## mycel config set

Set a configuration value

### Synopsis

Set a specific configuration value using dot notation.

The value type is automatically inferred (string, number, boolean).

Examples:
  mycel config set providers.default 6
  mycel config set providers.default claude
  mycel config set runtime.backend docker
  mycel config set tools.claude.command "claude --force"

```
mycel config set <key> <value> [flags]
```

### Options

```
  -h, --help   help for set
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel config](mycel_config.md)	 - Manage repo configuration

