/**
 * Desktop boot session — when to show the branded BootSplash.
 *
 * The splash is desktop-app only (real daemon readiness/logs). The browser
 * SPA must not run it. The Wails shell hands off with `?desktop=1`; SPA
 * navigations drop the query string, so we remember the handoff for this
 * tab via sessionStorage (not localStorage — that would bleed into Chrome
 * on the same origin after using the desktop app).
 *
 * After the splash completes once, `mycel.boot.done` prevents replaying on
 * HMR / remount within the same tab.
 */

const DESKTOP_SESSION = "mycel.showBootSplash";
const BOOT_DONE = "mycel.boot.done";

export function isDesktopBootSession(): boolean {
  try {
    if (new URLSearchParams(location.search).has("desktop")) {
      sessionStorage.setItem(DESKTOP_SESSION, "1");
      return true;
    }
    return sessionStorage.getItem(DESKTOP_SESSION) === "1";
  } catch {
    return false;
  }
}

/** True when this tab already finished the branded splash. */
export function isBootSplashDone(): boolean {
  try {
    return sessionStorage.getItem(BOOT_DONE) === "1";
  } catch {
    return false;
  }
}

export function markBootSplashDone(): void {
  try {
    sessionStorage.setItem(BOOT_DONE, "1");
  } catch {
    /* ignore quota / private mode */
  }
}

/**
 * Default skip for BootGate: skip on web, skip if already done this tab,
 * show only for an active desktop handoff session.
 */
export function shouldSkipBootSplash(explicitSkip?: boolean): boolean {
  if (explicitSkip !== undefined) return explicitSkip;
  if (isBootSplashDone()) return true;
  return !isDesktopBootSession();
}
