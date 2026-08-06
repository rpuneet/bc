import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  isBootSplashDone,
  isDesktopBootSession,
  markBootSplashDone,
  shouldSkipBootSplash,
} from "../desktopBoot";

const DESKTOP_SESSION = "mycel.showBootSplash";
const BOOT_DONE = "mycel.boot.done";

describe("desktopBoot", () => {
  beforeEach(() => {
    sessionStorage.clear();
    window.history.replaceState({}, "", "/");
  });

  afterEach(() => {
    sessionStorage.clear();
    window.history.replaceState({}, "", "/");
  });

  it("skips splash in the browser (no desktop handoff)", () => {
    expect(isDesktopBootSession()).toBe(false);
    expect(shouldSkipBootSplash()).toBe(true);
  });

  it("shows splash on ?desktop=1 handoff and remembers the tab session", () => {
    window.history.replaceState({}, "", "/?desktop=1&app_version=0.4.7");
    expect(isDesktopBootSession()).toBe(true);
    expect(shouldSkipBootSplash()).toBe(false);
    expect(sessionStorage.getItem(DESKTOP_SESSION)).toBe("1");

    // Query string drops on SPA nav — session still counts as desktop boot.
    window.history.replaceState({}, "", "/agents");
    expect(isDesktopBootSession()).toBe(true);
    expect(shouldSkipBootSplash()).toBe(false);
  });

  it("skips after the splash has completed once this tab", () => {
    window.history.replaceState({}, "", "/?desktop=1");
    expect(shouldSkipBootSplash()).toBe(false);
    markBootSplashDone();
    expect(isBootSplashDone()).toBe(true);
    expect(sessionStorage.getItem(BOOT_DONE)).toBe("1");
    expect(shouldSkipBootSplash()).toBe(true);
  });

  it("honors explicit skip override", () => {
    window.history.replaceState({}, "", "/?desktop=1");
    expect(shouldSkipBootSplash(true)).toBe(true);
    expect(shouldSkipBootSplash(false)).toBe(false);
  });
});
