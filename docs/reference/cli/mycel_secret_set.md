## mycel secret set

Create or update a secret

### Synopsis

Create or update an encrypted secret.

The value can be provided via --value, --from-env, or --from-file.
If none are specified, reads from stdin.

Note: --value appears in shell history. For sensitive values, prefer:
  mycel secret set API_KEY --from-env API_KEY
  echo "sk-abc123" | mycel secret set API_KEY

Examples:
  mycel secret set API_KEY --value "sk-abc123"
  mycel secret set API_KEY --from-env API_KEY
  mycel secret set API_KEY --from-file /path/to/key
  echo "sk-abc123" | mycel secret set API_KEY

```
mycel secret set <name> [flags]
```

### Options

```
      --desc string        Secret description
      --from-env string    Import value from environment variable
      --from-file string   Import value from file
  -h, --help               help for set
      --value string       Secret value (visible in shell history — prefer --from-env or stdin)
      --workspace          Write as a workspace-scoped override (default: user-global vault)
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel secret](mycel_secret.md)	 - Manage encrypted secrets

