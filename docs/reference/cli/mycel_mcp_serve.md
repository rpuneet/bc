## mycel mcp serve

Start mycel as an MCP server

### Synopsis

Start mycel as an MCP (Model Context Protocol) server.

AI tools like Claude Code and Cursor can connect to mycel via MCP to query
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
  send_message     Send a message to a channel
  send_file        Upload a file to a channel
  list_channels    List all channels
  read_channel     Read recent messages from a channel
  list_agents      List agents in the workspace
  whoami           Show the calling agent's identity

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

