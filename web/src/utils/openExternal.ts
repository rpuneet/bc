/**
 * openExternal — open a URL in the user's system browser from either the
 * embedded web UI or the Wails desktop app.
 *
 * In the desktop app, `window.open` / `<a target="_blank">` do NOT open
 * the system browser — Wails intercepts navigation inside its webview, so
 * the click silently does nothing (this was the "sign-in link doesn't
 * work" bug). The Wails runtime injects `window.runtime.BrowserOpenURL`
 * into the page at startup; when present, it is the only reliable way to
 * hand a URL to the OS browser. The web build never has `window.runtime`,
 * so it falls back to a plain new-tab open there.
 */
export function openExternal(url: string): void {
  if (!url) return;
  const wailsOpen = window.runtime?.BrowserOpenURL;
  if (typeof wailsOpen === "function") {
    wailsOpen(url);
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
