## mycel mcp add

Add an MCP server configuration

### Synopsis

Add a new MCP server configuration to the workspace.

For stdio transport (default), specify --command and optionally --args.
For SSE transport, specify --transport sse and --url.

Environment variables can be passed with --env as KEY=VALUE pairs.

Examples:
  mycel mcp add github --command npx --args "@modelcontextprotocol/server-github"
  mycel mcp add db --command npx --args "@modelcontextprotocol/server-sqlite,/tmp/test.db"
  mycel mcp add remote --transport sse --url "https://api.example.com/mcp"
  mycel mcp add github --command npx --env 'GITHUB_TOKEN=${secret:GITHUB_TOKEN}' --env "OWNER=me"

Use ${secret:NAME} references for sensitive values (see 'mycel secret set').

```
mycel mcp add <name> [flags]
```

### Options

```
      --args string        Comma-separated arguments
      --command string     Command to run (for stdio transport)
      --env stringArray    Environment variables (KEY=VALUE, repeatable)
  -h, --help               help for add
      --transport string   Transport type (stdio or sse) (default "stdio")
      --url string         Server URL (for sse transport)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel mcp](mycel_mcp.md)	 - Manage MCP server configurations

