## mycel workspace up

Start all roster agents

### Synopsis

Start all agents defined in [roster] of .bc/settings.json.

Agents that are already running are skipped. Missing role files are
created from built-in defaults automatically.

Examples:
  mycel workspace up          # Start roster agents
  bc ws up                 # Short alias

```
mycel workspace up [flags]
```

### Options

```
  -h, --help   help for up
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel workspace](mycel_workspace.md)	 - Manage mycel workspaces

