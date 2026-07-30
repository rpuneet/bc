# mycel desktop

A native window around the mycel server, in the spirit of Docker Desktop:
launching the app boots the full mycel server **in-process** and shows the
web UI in a window. The server listens on its normal localhost port
(default `127.0.0.1:9374`), so `http://127.0.0.1:9374` keeps working in any
browser while the app is open. Closing the window shuts the server down
gracefully.

Built with [Wails v2](https://wails.io) — pure Go, no Rust/Electron.

## How it works

- **Own Go module.** `desktop/go.mod` declares `github.com/rpuneet/mycel/desktop`
  with a `replace github.com/rpuneet/mycel => ../` directive. Wails' heavy
  dependency tree never touches the main module; the root `go build ./...`
  is unaffected.
- **Same boot path as `mycel up`.** `server.go` resolves the anchor repo
  (`MYCEL_WORKSPACE`, or the enclosing adopted repo) and listen address
  (workspace preferences `server.host`/`server.port`, `--addr` flag
  override, default `127.0.0.1:9374`), publishes `MYCEL_DAEMON_ADDR` +
  `~/.mycel/daemon.addr` for CLI/agent discovery, then calls
  `cmd.RunServerCtx` — the exact code `mycel up` runs, with a
  caller-controlled context instead of signal handling.
- **Lifecycle.** Wails `OnStartup` starts the server goroutine;
  `OnShutdown` (window close / Cmd-Q) cancels the context and waits up to
  15s for the graceful shutdown (HTTP drain, service closers, PID file
  removal).
- **Window → UI handoff.** The webview boots on a tiny embedded page
  (`bootpage.go`) that polls `/api/health` until the server is up, then
  navigates the window to `http://127.0.0.1:<port>` (external-URL path).
  If the webview blocks that cross-scheme navigation, the page falls back
  to a full-window iframe pointing at the same URL after 2 seconds. Either
  way the UI runs against the real HTTP server — SSE and websockets use
  the normal network stack, not the Wails asset scheme.

## Dev

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest

cd desktop
wails dev          # hot-reloading dev build
```

The boot page is generated in Go (`bootpage.go`); `frontend/dist/` holds
only a placeholder so the embed FS is never empty. There is no npm build.

## Build

```bash
make build-local-desktop     # from the repo root (builds web UI first)
# or directly:
cd desktop && wails build
```

Output lands in `desktop/build/bin/`:

- macOS: `mycel.app` (self-signed ad-hoc)
- Linux: `mycel-desktop` binary
- Windows: `mycel-desktop.exe`

The web UI must be built first (`make build-local-web`) — the server
package embeds `server/web/dist` at compile time, and the desktop binary
inherits that embed.

## CI cross-build (follow-up — workflow not added yet)

Wails cannot cross-compile between OSes (native webview toolchains), so CI
should use a GitHub Actions matrix, one job per OS:

```yaml
strategy:
  matrix:
    include:
      - os: macos-latest      # produces mycel.app (universal via -platform darwin/universal)
      - os: ubuntu-latest     # needs libgtk-3-dev libwebkit2gtk-4.0-dev
      - os: windows-latest    # produces mycel-desktop.exe (NSIS installer via -nsis)
runs-on: ${{ matrix.os }}
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-go@v5
  - uses: oven-sh/setup-bun@v2
  - run: make build-local-web
  - run: go install github.com/wailsapp/wails/v2/cmd/wails@latest
  - run: cd desktop && wails build
  - uses: actions/upload-artifact@v4
    with: { path: desktop/build/bin/* }
```

macOS distribution beyond ad-hoc signing needs a Developer ID certificate +
notarization; Linux packaging (deb/rpm/AppImage) and a Windows NSIS
installer are follow-ups.

## Follow-ups

- System tray with Open/Quit — Wails v2 has no tray support; revisit on
  Wails v3 or add a platform tray library.
- Replace the placeholder spore icon (`build/appicon.svg` → `appicon.png`)
  with final branding.
- Release CI matrix (above) + signing/notarization.
