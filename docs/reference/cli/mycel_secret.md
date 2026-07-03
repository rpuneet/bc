## mycel secret

Manage encrypted secrets

### Synopsis

Manage encrypted secrets for the workspace.

Secrets store API keys and tokens used by tools, MCP servers, and agents.
Values are encrypted at rest with AES-256-GCM. The API never exposes
secret values in list/show operations.

Other configs reference secrets with ${secret:NAME} syntax:
  [tools.claude-code]
  env = { ANTHROPIC_API_KEY = "${secret:ANTHROPIC_API_KEY}" }

Examples:
  mycel secret set ANTHROPIC_API_KEY                    # Prompt for value
  mycel secret set ANTHROPIC_API_KEY --value "sk-..."   # Set directly
  mycel secret set GITHUB_TOKEN --from-env GITHUB_TOKEN # Import from env var
  mycel secret list                                     # List names (no values)
  mycel secret show ANTHROPIC_API_KEY                   # Show metadata
  mycel secret show ANTHROPIC_API_KEY --reveal          # Show actual value
  mycel secret delete ANTHROPIC_API_KEY                 # Delete a secret

### Options

```
  -h, --help   help for secret
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator
* [mycel secret delete](mycel_secret_delete.md)	 - Delete a secret
* [mycel secret get](mycel_secret_get.md)	 - Get a secret value (prints to stdout)
* [mycel secret list](mycel_secret_list.md)	 - List secrets (names and metadata only)
* [mycel secret set](mycel_secret_set.md)	 - Create or update a secret
* [mycel secret show](mycel_secret_show.md)	 - Show secret metadata

