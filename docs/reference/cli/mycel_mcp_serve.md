## mycel mcp serve

Start bc as an MCP server

### Synopsis

Start bc as an MCP (Model Context Protocol) server.

AI tools like Claude Code and Cursor can connect to bc via MCP to query
workspace state and control agents natively.

Default transport is stdio (newline-delimited JSON on stdin/stdout).
Use --sse to start an HTTP server instead.

Resources exposed:
  bc://workspace/status   Workspace name, path, and config
  bc://agents             All agents with state, role, and worktree info
  bc://channels           All channels with members and message counts
  bc://costs              Workspace and per-agent cost summaries
  bc://roles              Role definitions with capabilities
  bc://tools              Available AI agent tools

Tools available:
  create_agent     Create a new agent in the workspace
  send_message     Send a message to a channel
  report_status    Update an agent's current task
  query_costs      Query cost usage

Examples:
  mycel mcp serve                    # stdio — use in Claude Code settings.json
  mycel mcp serve --sse              # SSE on :8811
  mycel mcp serve --sse --addr :9000 # SSE on custom port

```
mycel mcp serve [flags]
```

### Options

```
      --addr string   Address to listen on (SSE mode only) (default ":8811")
  -h, --help          help for serve
      --sse           Use SSE transport instead of stdio
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel mcp](mycel_mcp.md)	 - Manage MCP server configurations

