package server

import (
	"embed"
	"io/fs"
)

//go:embed web/dist
var webUI embed.FS

// WebDist returns the embedded web/dist filesystem, or nil when only
// placeholder files are present (i.e. the UI has not been built yet).
func WebDist() fs.FS {
	sub, err := fs.Sub(webUI, "web/dist")
	if err != nil {
		return nil
	}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.Name() != "placeholder.txt" && e.Name() != ".gitkeep" {
			return sub
		}
	}
	return nil
}

// noWebUIPage is served in place of the UI when the running binary was built
// without it (see WebDist). It exists so that the daemon says which of the
// things that look identical in a browser has actually happened: the daemon is
// up and its API is answering on this very address, and only the bundle is
// missing. Kept to plain inline markup because the moment it is needed is the
// moment no asset can be served.
const noWebUIPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>mycel — web UI not built</title>
<style>
  :root { color-scheme: dark }
  body { margin:0; min-height:100vh; display:flex; align-items:center; justify-content:center;
         background:#12100e; color:#e8e3da;
         font:15px/1.65 ui-sans-serif,-apple-system,"Segoe UI",sans-serif }
  main { max-width:33rem; padding:2.5rem }
  h1 { font-size:1.35rem; margin:0 0 .4rem; color:#f0b429 }
  p { margin:0 0 1.15rem; color:#b8b0a4 }
  code { font:13px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;
         background:#1e1b17; color:#f0d9a8; padding:.15em .4em; border-radius:4px }
  pre { background:#1a1714; border:1px solid #2a251f; border-radius:8px;
        padding:.85rem 1rem; overflow-x:auto; margin:0 0 1.15rem }
  pre code { background:none; padding:0; color:#e8e3da }
  a { color:#f0b429 }
</style>
</head>
<body>
<main>
  <h1>The daemon is running. Its web UI is not in this binary.</h1>
  <p>This is not a broken daemon or a wrong port — the API is answering right
     here, which you can confirm at
     <a href="/api/agents">/api/agents</a>. The binary was just built without
     the UI bundle embedded, so there is nothing to serve at this path.</p>
  <p>To get the UI, build with the target that bundles it, then restart:</p>
  <pre><code>make build-local</code></pre>
  <p>Or, if you are working on the UI itself, run the dev server and use that
     instead — it proxies the API through to this daemon:</p>
  <pre><code>cd web &amp;&amp; bun run dev   # http://localhost:9375</code></pre>
  <p>The CLI is unaffected either way: <code>mycel status</code> and every other
     command talk to the API, not to this page.</p>
</main>
</body>
</html>
`
