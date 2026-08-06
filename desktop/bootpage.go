package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// bootPage is a silent bridge the Wails webview loads once. It waits for
// /api/health, then hands the window to the real UI at the localhost URL.
//
// Intentionally NOT a branded splash (#3673): the SPA BootSplash (desktop
// handoff only) owns the mushroom + real daemon readiness/log stream. A
// second static mushroom here made every desktop launch feel like two
// start screens.
//
// Preferred handoff: navigate the webview to TARGET+HANDOFF. Fallback: if
// the webview blocks that navigation, embed the UI in a full-window iframe.
const bootPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>mycel</title>
<style>
  html,body{height:100%%;margin:0;background:#14100b}
  /* Visually empty — SPA BootSplash draws the branded sequence after handoff. */
  #msg{position:absolute;width:1px;height:1px;padding:0;margin:-1px;
       overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
  iframe{position:fixed;inset:0;width:100vw;height:100vh;border:0}
</style>
</head>
<body>
<p id="msg" role="status" aria-live="polite">Starting mycel…</p>
<script>
  var TARGET = %q;
  var HANDOFF = %q;
  var msg = document.getElementById("msg");
  var attempts = 0;

  function healthy() {
    return fetch(TARGET + "/api/health", {cache: "no-store"})
      .then(function (r) { return r.ok; })
      .catch(function () { return false; });
  }

  function handoff() {
    msg.textContent = "Opening…";
    // ?desktop=1 marks the SPA for desktop-only BootSplash + openExternal.
    // app_version is the app's own version (daemon may be an older attach).
    window.location.replace(TARGET + HANDOFF);
    setTimeout(function () {
      var f = document.createElement("iframe");
      f.src = TARGET + HANDOFF;
      document.body.appendChild(f);
    }, 2000);
  }

  (function poll() {
    healthy().then(function (ok) {
      // After ~20s of failed probes, hand off anyway.
      if (ok || ++attempts > 40) { handoff(); return; }
      setTimeout(poll, 500);
    });
  })();
</script>
</body>
</html>
`

// handoffPath builds the path the boot page navigates to, carrying the desktop
// marker and the app's own version. appVersion is URL-encoded because a source
// build's version is not a bare identifier ("0.4.5-dev.12.g1a2b3c4.dirty") and
// nothing guarantees a future one will stay query-safe.
func handoffPath(appVersion string) string {
	q := url.Values{"desktop": {"1"}}
	if appVersion != "" {
		q.Set("app_version", appVersion)
	}
	return "/?" + q.Encode()
}

// renderBootPage bakes the server URL and the handoff path into the boot page.
func renderBootPage(serverURL, appVersion string) string {
	return fmt.Sprintf(bootPage, serverURL, handoffPath(appVersion))
}

// bootMiddleware intercepts the document requests ("/", "/index.html")
// and serves the boot page with the live server URL; everything else
// falls through to the embedded assets.
func bootMiddleware(serverURL, appVersion string) func(http.Handler) http.Handler {
	page := []byte(renderBootPage(serverURL, appVersion))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimSuffix(r.URL.Path, "index.html")
			if r.Method == http.MethodGet && (path == "" || path == "/") {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(page) //nolint:errcheck // best-effort response write
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
