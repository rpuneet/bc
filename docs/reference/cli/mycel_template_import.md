## mycel template import

Import a template from a file, URL, or the marketplace catalog

### Synopsis

Import a template into the daemon template store.

<source> may be:
  - a path to a local JSON file describing the template
  - an http(s) URL to a template JSON document
  - the name of a template already known to the marketplace catalog

Importing a name that already exists updates it in place when --force is set.

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

