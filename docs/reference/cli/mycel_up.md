## mycel up

Start mycel server

### Synopsis

Start the mycel server (API, web UI, MCP, agent management).

Bootstraps everything on first run — no separate init step:
  - inside a git repo: the repo root is adopted as the anchor repo
    (state lives under ~/.mycel, the repo stays pristine) and new
    agents default their repo to it
  - outside a git repo: the server boots against MycelHome only

By default the server runs in the foreground (for Docker/Railway).
Use -d to run as a background daemon.

Examples:
  mycel up                              # Foreground (Docker/Railway)
  mycel up -d                           # Background daemon
  mycel up --addr 0.0.0.0:9374         # Custom listen address
  mycel up --workspace /path/to/ws     # Explicit workspace

```
mycel up [flags]
```

### Options

```
      --addr string          Listen address (host:port) (default "127.0.0.1:9374")
      --api-key string       API key for Bearer token auth (or set MYCEL_API_KEY)
      --cors-origin string   CORS allowed origin (default "*")
  -d, --daemon               Run as background daemon
  -h, --help                 help for up
      --workspace string     Workspace directory (defaults to current workspace)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator

