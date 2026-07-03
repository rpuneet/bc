## mycel workspace

Manage mycel workspaces

### Synopsis

Manage mycel workspaces: info, config, logs, list.

Examples:
  mycel workspace info                   # Show workspace details
  mycel workspace status                 # Show agents and health
  mycel workspace config show            # Show workspace config
  mycel workspace config set KEY VAL     # Set config value
  mycel workspace list                   # List discovered workspaces
  mycel workspace list --scan ~/Projects # Scan additional paths
  mycel workspace discover               # Discover and register new workspaces

### Options

```
  -h, --help   help for workspace
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator
* [mycel workspace add](mycel_workspace_add.md)	 - Add a workspace to the registry
* [mycel workspace config](mycel_workspace_config.md)	 - Manage workspace configuration
* [mycel workspace discover](mycel_workspace_discover.md)	 - Discover and register workspaces
* [mycel workspace info](mycel_workspace_info.md)	 - Show workspace information
* [mycel workspace list](mycel_workspace_list.md)	 - List discovered workspaces
* [mycel workspace remove](mycel_workspace_remove.md)	 - Remove a workspace from the registry
* [mycel workspace stats](mycel_workspace_stats.md)	 - Show workspace statistics
* [mycel workspace status](mycel_workspace_status.md)	 - Show workspace status and agent health
* [mycel workspace switch](mycel_workspace_switch.md)	 - Switch active workspace
* [mycel workspace up](mycel_workspace_up.md)	 - Start all roster agents

