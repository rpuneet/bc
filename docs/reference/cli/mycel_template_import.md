## mycel template import

Import a template from a file, URL, or the marketplace catalog

### Synopsis

Import a template into the global template store (~/.mycel/templates/).

<source> may be:
  - a path to a local JSON file describing the template
  - an http(s) URL to a template JSON document
  - the name of a template already known to the marketplace catalog
    (mycel marketplace list --type template)

The JSON document has the same shape as 'mycel template show', plus an
optional "system_prompt" string field carrying the system prompt text:

  {
    "name": "my-template",
    "description": "...",
    "mcps": ["mycel"],
    "system_prompt": "You are..."
  }

Importing a name that already exists in the store updates it in place;
pass --force to allow the overwrite.

```
mycel template import <source> [flags]
```

### Options

```
      --force         overwrite an existing template with the same name
  -h, --help          help for import
      --name string   override the imported template's name
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel template](mycel_template.md)	 - Manage agent templates

