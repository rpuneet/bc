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

- macOS: `mycel.app`
- Linux: `mycel-desktop` binary
- Windows: `mycel-desktop.exe`

The web UI must be built first (`make build-local-web`) — the server
package embeds `server/web/dist` at compile time, and the desktop binary
inherits that embed.

## Installing the desktop app

Release builds are ad-hoc / self-signed unless the maintainer's signing
secrets are configured (see below), so a fresh download will trip the OS
first-run gate. To open it:

**macOS** — Gatekeeper blocks an unnotarized `.app` on first launch. Either:

- Right-click `mycel.app` → **Open**, then confirm **Open** in the dialog; or
- Clear the quarantine flag from a terminal:

  ```bash
  xattr -dr com.apple.quarantine /Applications/mycel.app
  ```

**Windows** — SmartScreen shows "Windows protected your PC" for an
unsigned `.exe`. Click **More info → Run anyway**.

Once the maintainer procures a Developer ID certificate (macOS) and an EV /
OV code-signing certificate (Windows), signed + notarized builds open with
no warning and these steps become unnecessary.

## Release CI

Desktop apps are built by the `release-desktop` job in
`.github/workflows/release.yml` — Wails cannot cross-compile between OSes
(native webview toolchains), so it uses a matrix, one job per target.

Architectures covered:

| OS      | arm64                          | amd64                                    |
|---------|--------------------------------|------------------------------------------|
| macOS   | native (`macos-latest`)        | cross-built on Apple Silicon (`-platform darwin/amd64`) |
| Linux   | native (`ubuntu-24.04-arm`)    | native (`ubuntu-latest`)                 |
| Windows | —                              | native (`windows-latest`)                |

Linux arm64 relies on GitHub's `ubuntu-24.04-arm` hosted runners. Windows
arm64 is not built yet (no native runner + webview toolchain); it is the
one deferred target.

## Code-signing & notarization (release secrets)

The macOS legs of `release-desktop` sign and notarize `mycel.app` **only
when the signing secrets are present**. On fork PRs or any run without the
secrets the app ships ad-hoc and the signing steps are skipped — the
workflow never fails for missing secrets.

To enable signed + notarized macOS builds, the owner adds these repository
secrets (Settings → Secrets and variables → Actions):

| Secret | What it is / how to obtain it |
|--------|-------------------------------|
| `MACOS_CERTIFICATE` | Your **Developer ID Application** certificate exported as a `.p12`, then base64-encoded. In Keychain Access export the cert+key to `cert.p12`, then `base64 -i cert.p12 \| pbcopy`. |
| `MACOS_CERTIFICATE_PWD` | The password you set when exporting the `.p12`. |
| `MACOS_SIGNING_IDENTITY` | The identity string `codesign` matches, e.g. `Developer ID Application: Your Name (TEAMID)`. Find it with `security find-identity -v -p codesigning`. |
| `APPLE_ID` | The Apple ID email of your Apple Developer account (used by `notarytool`). |
| `APPLE_TEAM_ID` | Your 10-character Team ID, from the [Apple Developer membership page](https://developer.apple.com/account#MembershipDetailsCard). |
| `APPLE_APP_PASSWORD` | An **app-specific password** for that Apple ID, generated at [appleid.apple.com](https://appleid.apple.com) → Sign-In and Security → App-Specific Passwords. Not your account password. |

Windows code-signing is a separate certificate (EV/OV code-signing cert
from a CA) and is **not wired up yet** — a future addition once the cert is
procured.

## Follow-ups

- System tray with Open/Quit — Wails v2 has no tray support; revisit on
  Wails v3 or add a platform tray library.
- Replace the placeholder spore icon (`build/appicon.svg` → `appicon.png`)
  with final branding.
- Windows code-signing (needs an EV/OV cert) and Windows arm64 build.
- Linux packaging (deb/rpm/AppImage).
