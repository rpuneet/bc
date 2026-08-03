/**
 * defaultView — when the "Default view" preference may redirect, and when it
 * must keep quiet.
 *
 * The preference decides where the app opens. It must not decide where the app
 * goes once it is open, because the navigation's Home link points at "/": making
 * "/" obey the preference made clicking Home send you to Agents, so Home became
 * unreachable (#3556). The same mistake also broke the back button — returning
 * from Agents to "/" bounced forward to Agents again, with no way out.
 *
 * So the rule is: honor it once, for the entry that opened the app, and never
 * again in that document.
 */

/**
 * The path the document was opened with, read once at module load — before any
 * client-side navigation can change it.
 */
let entryPath = typeof window === "undefined" ? "" : window.location.pathname;

let applied = false;

/**
 * shouldApplyDefaultView reports whether a mount of the root route is the one
 * that opened the app.
 *
 * False for a click on Home, for a back navigation to "/", and for every mount
 * after the first — all of which are the user saying where they want to be.
 */
export function shouldApplyDefaultView(): boolean {
  if (applied) return false;
  return entryPath === "/" || entryPath === "";
}

/** markDefaultViewApplied records that the entry redirect has happened. */
export function markDefaultViewApplied(): void {
  applied = true;
}

/** Test seam: module state outlives a single render tree. */
export function __resetDefaultViewForTests(path = "/"): void {
  applied = false;
  entryPath = path;
}
