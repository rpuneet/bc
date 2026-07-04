## mycel template

Manage agent templates

### Synopsis

Manage agent templates — reusable configurations for spawning agents.

Templates are stored in ~/.mycel/templates/ (user-global) and each workspace
may override a template by placing a file with the same name under its
state dir (~/.mycel/workspaces/<id>/templates/). List/show/edit see the
union; create defaults to writing the user-global copy.

Examples:
  mycel template list                    # List all templates (global + workspace overrides)
  mycel template show feature-dev        # Show template details
  mycel template create my-template      # Scaffold a new user-global template
  mycel template create my-template --workspace   # Workspace-local override
  mycel template delete my-template      # Delete (prefers workspace scope)
  mycel template delete my-template --global      # Delete user-global

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
* [mycel template list](mycel_template_list.md)	 - List all templates
* [mycel template show](mycel_template_show.md)	 - Show template details and system prompt

