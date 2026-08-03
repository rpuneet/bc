/**
 * desktopApp — the version of the Wails desktop shell hosting this UI, when
 * there is one.
 *
 * The desktop app attaches to an already-running daemon when it finds one
 * (desktop/server.go), and the window is then a pure client of it. Everything
 * the page loads — including /api/health, which is where every version on the
 * About page comes from — is served by that daemon, so an app can be newer than
 * the daemon answering it and the UI has no way to tell. Someone who updates the
 * app then sees the old daemon's version and concludes the update did not take.
 *
 * The boot page therefore passes the app's own version through the handoff URL,
 * the only channel that exists. Capture happens at module load rather than on
 * first use because SPA navigation drops the query string: by the time a view
 * asks, the marker may be several routes in the past.
 *
 * The marker is held in sessionStorage, whose lifetime — this tab, this visit —
 * is exactly the claim being made: *this window is hosted by that app*. It was
 * localStorage, which outlives the window and so outlived the app: any browser
 * that had ever been handed off went on reporting that version forever, so a
 * plain tab announced a "Desktop app" that was not running and might not still
 * be installed, and About warned of a mismatch against a build that no longer
 * existed (#3562).
 */

const STORAGE_KEY = "mycel.desktopAppVersion";

/** Read the handoff marker before the router can drop it. */
function capture(): string {
  try {
    const fromURL = new URLSearchParams(location.search).get("app_version");
    if (fromURL) {
      sessionStorage.setItem(STORAGE_KEY, fromURL);
      return fromURL;
    }
    return sessionStorage.getItem(STORAGE_KEY) ?? "";
  } catch {
    // Private-mode storage throws on write; the URL value is still good for
    // this page load, so prefer it over reporting nothing.
    try {
      return new URLSearchParams(location.search).get("app_version") ?? "";
    } catch {
      return "";
    }
  }
}

const captured = capture();

/**
 * desktopAppVersion returns the hosting desktop app's version, or "" when the UI
 * is not running inside one (a plain browser tab, where the daemon's version is
 * the only one that exists and nothing needs disambiguating).
 */
export function desktopAppVersion(): string {
  return captured;
}
