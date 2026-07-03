## mycel tool add

Add a tool to the workspace

### Synopsis

Add a custom tool provider to the workspace.

Examples:
  mycel tool add mytool --command "mytool --yes" --install "pip install mytool"
  mycel tool add mytool --command "mytool" --slash-cmds "/help,/quit"

```
mycel tool add <name> [flags]
```

### Options

```
      --command string      Command to run the tool (required)
  -h, --help                help for add
      --install string      Command to install the tool
      --json                Output as JSON
      --slash-cmds string   Comma-separated list of slash commands (e.g. /help,/quit)
      --upgrade string      Command to upgrade the tool
```

### Options inherited from parent commands

```
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel tool](mycel_tool.md)	 - Manage AI tool providers

