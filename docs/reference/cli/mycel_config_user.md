## mycel config user

Manage user-level configuration (~/.bcrc)

### Synopsis

Manage user-level configuration stored in ~/.bcrc.

User configuration provides defaults that apply across all mycel workspaces:
  - Your nickname for channel messages
  - Default role for new agents
  - Preferred AI tools

Workspace config (.bc/settings.json) takes precedence over user config.

Examples:
  mycel config user init   # Create ~/.bcrc with guided prompts
  mycel config user show   # Show user config
  mycel config user path   # Show user config path

```
mycel config user [flags]
```

### Options

```
  -h, --help   help for user
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel config](mycel_config.md)	 - Manage workspace configuration
* [mycel config user init](mycel_config_user_init.md)	 - Create user configuration file (~/.bcrc)
* [mycel config user path](mycel_config_user_path.md)	 - Show user configuration file path
* [mycel config user show](mycel_config_user_show.md)	 - Show user configuration

