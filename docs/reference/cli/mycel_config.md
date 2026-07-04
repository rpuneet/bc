## mycel config

Manage repo configuration

### Synopsis

Commands for managing repo configuration (preferences.json).

Configuration uses a hierarchical key structure with dot notation:
  workspace.name
  providers.claude.command
  providers.default

Examples:
  mycel config show                        # Show all config
  mycel config get providers.default           # Get a specific value
  mycel config set providers.default 6      # Set a value
  mycel config list                        # List all config keys
  mycel config edit                        # Open config in editor
  mycel config validate                    # Validate config file
  mycel config reset                       # Reset to defaults

```
mycel config [flags]
```

### Options

```
  -h, --help   help for config
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator
* [mycel config edit](mycel_config_edit.md)	 - Edit configuration file
* [mycel config get](mycel_config_get.md)	 - Get a configuration value
* [mycel config list](mycel_config_list.md)	 - List all configuration keys
* [mycel config reset](mycel_config_reset.md)	 - Reset configuration to defaults
* [mycel config set](mycel_config_set.md)	 - Set a configuration value
* [mycel config show](mycel_config_show.md)	 - Show configuration
* [mycel config user](mycel_config_user.md)	 - Manage user-level configuration (~/.bcrc)
* [mycel config validate](mycel_config_validate.md)	 - Validate configuration file

