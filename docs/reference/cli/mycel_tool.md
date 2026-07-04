## mycel tool

Manage AI tool providers

### Synopsis

Add, remove, and inspect AI tool providers for agent spawning.

Examples:
  mycel tool list              # Show all tools with status
  mycel tool add myagent       # Add a custom tool
  mycel tool show claude       # Show tool details
  mycel tool setup claude      # Install and configure a tool
  mycel tool status claude     # Check installation status
  mycel tool upgrade claude    # Upgrade an installed tool
  mycel tool delete mytool     # Remove a custom tool
  mycel tool run claude --help # Run a tool directly

### Options

```
  -h, --help   help for tool
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator
* [mycel tool add](mycel_tool_add.md)	 - Add a tool to the repo
* [mycel tool delete](mycel_tool_delete.md)	 - Remove a tool from the repo
* [mycel tool edit](mycel_tool_edit.md)	 - Edit a tool's configuration
* [mycel tool list](mycel_tool_list.md)	 - List all configured tools and their status
* [mycel tool run](mycel_tool_run.md)	 - Run a tool directly
* [mycel tool setup](mycel_tool_setup.md)	 - Install and configure a tool
* [mycel tool show](mycel_tool_show.md)	 - Show detailed information about a tool
* [mycel tool status](mycel_tool_status.md)	 - Check installation status of a tool
* [mycel tool upgrade](mycel_tool_upgrade.md)	 - Upgrade an installed tool

