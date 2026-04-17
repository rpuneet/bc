## bc workspace

Manage bc workspaces

### Synopsis

Manage bc workspaces: info, config, logs, list.

Examples:
  bc workspace info                   # Show workspace details
  bc workspace status                 # Show agents and health
  bc workspace config show            # Show workspace config
  bc workspace config set KEY VAL     # Set config value
  bc workspace list                   # List discovered workspaces
  bc workspace list --scan ~/Projects # Scan additional paths
  bc workspace discover               # Discover and register new workspaces

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

* [bc](bc.md)	 - A simpler, more controllable agent orchestrator
* [bc workspace add](bc_workspace_add.md)	 - Add a workspace to the registry
* [bc workspace config](bc_workspace_config.md)	 - Manage workspace configuration
* [bc workspace discover](bc_workspace_discover.md)	 - Discover and register workspaces
* [bc workspace info](bc_workspace_info.md)	 - Show workspace information
* [bc workspace list](bc_workspace_list.md)	 - List discovered workspaces
* [bc workspace remove](bc_workspace_remove.md)	 - Remove a workspace from the registry
* [bc workspace stats](bc_workspace_stats.md)	 - Show workspace statistics
* [bc workspace status](bc_workspace_status.md)	 - Show workspace status and agent health
* [bc workspace switch](bc_workspace_switch.md)	 - Switch active workspace
* [bc workspace up](bc_workspace_up.md)	 - Start all roster agents

