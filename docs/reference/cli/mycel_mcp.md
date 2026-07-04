## mycel mcp

Manage MCP server configurations

### Synopsis

Manage Model Context Protocol (MCP) server configurations.

MCP servers provide tools and resources to AI agents. Configurations are
stored per-workspace and can be referenced by roles.

Examples:
  mycel mcp list                                     # List all MCP servers
  mycel mcp add github --command npx --args "@modelcontextprotocol/server-github"
  mycel mcp add sqlite --command npx --args "@modelcontextprotocol/server-sqlite,/path/to/db"
  mycel mcp add remote --transport sse --url "https://api.example.com/mcp"
  mycel mcp add github --command npx --env "GITHUB_TOKEN=tok_123"
  mycel mcp show github                              # Show server details
  mycel mcp remove github                            # Remove a server
  mycel mcp disable github                           # Disable a server
  mycel mcp enable github                            # Re-enable a server

### Options

```
  -h, --help   help for mcp
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator
* [mycel mcp add](mycel_mcp_add.md)	 - Add an MCP server configuration
* [mycel mcp disable](mycel_mcp_disable.md)	 - Disable an MCP server configuration
* [mycel mcp enable](mycel_mcp_enable.md)	 - Enable an MCP server configuration
* [mycel mcp list](mycel_mcp_list.md)	 - List MCP server configurations
* [mycel mcp register](mycel_mcp_register.md)	 - Register bc as an MCP server in agent settings.json
* [mycel mcp remove](mycel_mcp_remove.md)	 - Remove an MCP server configuration
* [mycel mcp serve](mycel_mcp_serve.md)	 - Start bc as an MCP server
* [mycel mcp show](mycel_mcp_show.md)	 - Show MCP server configuration details

