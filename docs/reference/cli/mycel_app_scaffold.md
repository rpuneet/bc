## mycel app scaffold

Generate a new app/gateway plugin skeleton

### Synopsis

Generate a new app plugin skeleton under pkg/gateway/<name>/.

Adding an app today = one new plugin package under pkg/gateway/<name>/
plus one import line in pkg/app/builtin/builtin.go. This command
generates the plugin package (a plugin.go implementing app.Plugin and
a <name>.go adapter implementing gateway.NotificationAdapter) so
contributors can fill in the TODOs and wire it up.

Examples:
  mycel app scaffold linear             # pkg/gateway/linear/
  mycel app scaffold telegram --multi   # allow labeled instances

```
mycel app scaffold <name> [flags]
```

### Options

```
      --dir string   parent directory to generate the plugin package under (default "pkg/gateway")
  -h, --help         help for scaffold
      --multi        allow labeled instances (e.g. telegram:alerts)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel app](mycel_app.md)	 - Manage app (gateway plugin) integrations

