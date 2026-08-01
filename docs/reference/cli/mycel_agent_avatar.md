## mycel agent avatar

Generate agent AgentCharacter avatar images (for public hosting)

### Synopsis

Generate the deterministic AgentCharacter avatar for one or more agents as
PNG (and optionally SVG) files in an output directory.

With no names, every current agent is rendered (requires a running daemon).
With names, exactly those are rendered offline — no daemon needed, since the
avatar derives purely from the name.

Publish flow (makes avatars fetchable by Slack, which needs a public URL):
  1. mycel agent avatar --out landing/public/avatars
  2. commit landing/public/avatars and deploy the landing site
  3. export MYCEL_AVATAR_PUBLIC_BASE=https://bc-infra.com/avatars
Then whoami.avatar_url and the Slack gateway icon_url resolve to the public PNG.

```
mycel agent avatar [names...] [flags]
```

### Options

```
  -h, --help         help for avatar
      --out string   output directory for avatar files (default "landing/public/avatars")
      --size int     avatar dimension in pixels (default 256)
      --svg          also write an .svg alongside each .png
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel agent](mycel_agent.md)	 - Manage mycel agents

