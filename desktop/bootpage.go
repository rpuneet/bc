package main

import (
	"fmt"
	"net/http"
	"strings"
)

// bootPage is the only page the Wails webview ever loads from the
// embedded assets. It waits for the in-process server to answer
// /api/health, then hands the window over to the real UI at the
// localhost URL — first by navigating the webview there directly
// (external-URL path), and if the webview blocks that navigation,
// by filling the window with an iframe to the same URL (fallback).
// Either way the UI talks to the real HTTP server, so SSE and
// websockets bypass the Wails asset scheme entirely.
const bootPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>mycel</title>
<style>
  html,body{height:100%%;margin:0}
  body{display:flex;align-items:center;justify-content:center;
       background:#101312;color:#e8e4d8;
       font:15px/1.5 -apple-system,"Segoe UI",Ubuntu,sans-serif}
  .boot{text-align:center}
  .mark{width:96px;height:96px;margin:0 auto 20px;display:block}
  .cap{animation:breathe 2.4s ease-in-out infinite;transform-origin:50%% 60%%}
  @keyframes breathe{0%%,100%%{transform:scale(1)}50%%{transform:scale(1.04)}}
  h1{font-size:20px;font-weight:600;margin:0 0 6px;letter-spacing:.04em}
  p{margin:0;color:#9aa39b}
  iframe{position:fixed;inset:0;width:100vw;height:100vh;border:0}
</style>
</head>
<body>
<div class="boot" id="boot">
  <svg class="mark" viewBox="0 0 128 128" aria-hidden="true">
    <g class="cap">
      <path d="M24 66 C24 38 42 24 64 24 C86 24 104 38 104 66 C104 72 98 74 90 74 L38 74 C30 74 24 72 24 66 Z" fill="#e8e4d8"/>
      <path d="M56 74 L72 74 L70 98 C70 103 66.5 106 64 106 C61.5 106 58 103 58 98 Z" fill="#c8c2b0"/>
      <circle cx="47" cy="48" r="5" fill="#101312" opacity=".25"/>
      <circle cx="70" cy="40" r="4" fill="#101312" opacity=".25"/>
      <circle cx="84" cy="56" r="3.5" fill="#101312" opacity=".25"/>
    </g>
    <circle cx="22" cy="96" r="2.5" fill="#8fae72"/>
    <circle cx="34" cy="110" r="2" fill="#8fae72" opacity=".7"/>
    <circle cx="102" cy="102" r="2.5" fill="#8fae72"/>
    <circle cx="110" cy="88" r="2" fill="#8fae72" opacity=".7"/>
  </svg>
  <h1>mycel</h1>
  <p id="msg">starting the server&hellip;</p>
</div>
<script>
  var TARGET = %q;
  var msg = document.getElementById("msg");
  var attempts = 0;

  function healthy() {
    return fetch(TARGET + "/api/health", {cache: "no-store"})
      .then(function (r) { return r.ok; })
      .catch(function () { return false; });
  }

  function handoff() {
    msg.textContent = "opening…";
    // Preferred: navigate the webview straight to the localhost URL.
    window.location.replace(TARGET + "/");
    // Fallback: if the webview refused the cross-scheme navigation,
    // we are still here after 2s — embed the UI in a full-window iframe.
    setTimeout(function () {
      var f = document.createElement("iframe");
      f.src = TARGET + "/";
      document.body.appendChild(f);
      document.getElementById("boot").remove();
    }, 2000);
  }

  (function poll() {
    healthy().then(function (ok) {
      // After ~20s of failed probes, hand off anyway: if fetch is
      // blocked by the webview but the server is fine, the UI still
      // loads; if the server truly failed, the user sees the browser
      // error instead of an eternal spinner.
      if (ok || ++attempts > 40) { handoff(); return; }
      setTimeout(poll, 500);
    });
  })();
</script>
</body>
</html>
`

// renderBootPage bakes the server URL into the boot page.
func renderBootPage(serverURL string) string {
	return fmt.Sprintf(bootPage, serverURL)
}

// bootMiddleware intercepts the document requests ("/", "/index.html")
// and serves the boot page with the live server URL; everything else
// falls through to the embedded assets.
func bootMiddleware(serverURL string) func(http.Handler) http.Handler {
	page := []byte(renderBootPage(serverURL))
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
