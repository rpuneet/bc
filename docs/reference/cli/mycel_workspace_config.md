## mycel workspace config

Manage workspace configuration

### Synopsis

Manage workspace configuration (preferences.json).

Examples:
  mycel workspace config show                    # Show full config
  mycel workspace config get providers.default   # Get a value
  mycel workspace config set providers.default claude # Set a value
  mycel workspace config validate                # Validate config
  mycel workspace config edit                    # Open in $EDITOR

```
mycel workspace config [flags]
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

* [mycel workspace](mycel_workspace.md)	 - Manage mycel workspaces
* [mycel workspace config edit](mycel_workspace_config_edit.md)	 - Edit configuration file in $EDITOR
* [mycel workspace config get](mycel_workspace_config_get.md)	 - Get a configuration value
* [mycel workspace config set](mycel_workspace_config_set.md)	 - Set a configuration value
* [mycel workspace config show](mycel_workspace_config_show.md)	 - Show configuration
* [mycel workspace config validate](mycel_workspace_config_validate.md)	 - Validate configuration file

