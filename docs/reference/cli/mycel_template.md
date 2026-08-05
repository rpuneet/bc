## mycel template

Manage agent templates

### Synopsis

Manage agent templates — reusable configurations for spawning agents.

Templates are managed by the mycel daemon (same store as the web UI).

Examples:
  mycel template list                    # List all templates
  mycel template show blank              # Show template details
  mycel template create my-template      # Scaffold a new template
  mycel template import ./my-tmpl.json   # Import a template from a file
  mycel template delete my-template      # Delete a template

```
mycel template [flags]
```

### Options

```
  -h, --help   help for template
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator
* [mycel template create](mycel_template_create.md)	 - Create a new template
* [mycel template delete](mycel_template_delete.md)	 - Delete a template
* [mycel template import](mycel_template_import.md)	 - Import a template from a file, URL, or the marketplace catalog
* [mycel template list](mycel_template_list.md)	 - List all templates
* [mycel template show](mycel_template_show.md)	 - Show template details and system prompt

