## mycel mcp register

Register mycel as an MCP server in agent settings.json

### Synopsis

Automatically add mycel to the Claude Code MCP server configuration.

This writes (or updates) the mcp.servers entry in the workspace
settings.json so that agents automatically have access to mycel MCP tools.

Examples:
  mycel mcp register               # Register with stdio transport
  mycel mcp register --sse         # Register with SSE transport

```
mycel mcp register [flags]
```

### Options

```
      --addr string   SSE server address to register (default ":8811")
  -h, --help          help for register
      --sse           Register SSE transport endpoint
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel mcp](mycel_mcp.md)	 - Manage MCP server configurations

