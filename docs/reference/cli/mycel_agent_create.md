## mycel agent create

Create a new agent

### Synopsis

Create and start a new agent.

If no name is provided, a random memorable name is generated (e.g., swift-falcon).
Agents are configured via templates (markdown files at ~/.mycel/templates/).
Use --copy to clone settings from an existing agent.

Examples:
  mycel agent create                              # Random name, base template
  mycel agent create worker-01                    # Explicit name, base template
  mycel agent create eng-01 --template engineer   # Use engineer template
  mycel agent create qa-01 --tool cursor          # Base template with Cursor
  mycel agent create clone-01 --copy swift-hawk   # Copy config from swift-hawk

```
mycel agent create [name] [flags]
```

### Options

```
      --copy string       Copy settings from an existing agent
      --env string        Path to env file (KEY=VALUE per line)
  -h, --help              help for create
      --parent string     Parent agent ID
      --role string       Agent role (default: base)
      --runtime string    Runtime backend override: tmux or docker
      --task string       Initial task recorded on the agent and delivered after spawn
      --team string       Team name (alphanumeric)
      --template string   Template name from ~/.mycel/templates/ (e.g. base, engineer)
      --tool string       Agent tool (agy, claude, codex, cursor, pi)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel agent](mycel_agent.md)	 - Manage mycel agents

