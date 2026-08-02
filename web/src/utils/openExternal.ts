/**
 * openExternal — open a URL in the user's system browser from either the
 * embedded web UI or the Wails desktop app.
 *
 * In the desktop app, `window.open` / `<a target="_blank">` do NOT open the
 * system browser: the Wails webview boots on the daemon's http://127.0.0.1
 * origin, and Wails only injects `window.runtime` (and `BrowserOpenURL`) into
 * pages served through its own asset scheme — never into an external http://
 * origin. So the click silently does nothing (this was the "sign-in link
 * doesn't work" bug). To detect the desktop shell the boot page appends a
 * `?desktop=1` marker to the URL it hands the SPA; when present we ask the
 * daemon to open the link via `POST /api/system/open-url`. The web build has
 * no marker and no `window.runtime`, so it falls back to a plain new-tab open.
 */

/**
 * isDesktop reports whether the UI is running inside the Wails desktop shell.
 * The boot page navigates to `…/?desktop=1`; SPA navigation then drops the
 * query string, so the first sighting is persisted to localStorage.
 */
export function isDesktop(): boolean {
  try {
    if (new URLSearchParams(location.search).has("desktop")) {
      localStorage.setItem("mycel.desktop", "1");
      return true;
    }
    return localStorage.getItem("mycel.desktop") === "1";
  } catch {
    return false;
  }
}

export function openExternal(url: string): void {
  if (!url) return;

  // If Wails ever does inject its runtime (asset-scheme pages), prefer it.
  const wailsOpen = window.runtime?.BrowserOpenURL;
  if (typeof wailsOpen === "function") {
    wailsOpen(url);
    return;
  }

  if (isDesktop()) {
    const fallback = () => window.open(url, "_blank", "noopener,noreferrer");
    fetch("/api/system/open-url", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ url }),
    })
      // fetch only rejects on network failure — a 400/403/502 from the daemon
      // still resolves, so check res.ok explicitly or the failure is silent.
      .then((res) => {
        if (!res.ok) fallback();
      })
      .catch(fallback);
    return;
  }

  window.open(url, "_blank", "noopener,noreferrer");
}

declare global {
  interface Window {
    /** Present only inside the Wails desktop webview. */
    runtime?: {
      BrowserOpenURL?: (url: string) => void;
    };
  }
}
