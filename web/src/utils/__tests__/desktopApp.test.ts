import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

/**
 * The desktop app's version reaches the UI only through the handoff URL, and SPA
 * navigation drops the query string moments later. These tests pin the capture,
 * because losing it silently reverts the page to showing one number that a user
 * reasonably reads as the app's when it is the daemon's.
 */

const STORAGE_KEY = "mycel.desktopAppVersion";

// The module captures at load, so each case needs a fresh import.
async function loadFresh() {
  vi.resetModules();
  return import("../desktopApp");
}

function setSearch(search: string) {
  // location is not writable in jsdom; replace just what the module reads.
  Object.defineProperty(window, "location", {
    value: { ...window.location, search },
    writable: true,
    configurable: true,
  });
}

/**
 * installStorage — this jsdom setup does not provide localStorage (other suites
 * guard it with `window.localStorage?.`), so each case supplies its own. That
 * also lets the failure case be a store whose writes throw, which is what
 * private browsing does.
 */
function installStorage(opts: { throwOnWrite?: boolean } = {}) {
  const map = new Map<string, string>();
  const stub = {
    getItem: (k: string) => map.get(k) ?? null,
    setItem: (k: string, v: string) => {
      if (opts.throwOnWrite) throw new Error("QuotaExceededError");
      map.set(k, v);
    },
    removeItem: (k: string) => void map.delete(k),
    clear: () => map.clear(),
  };
  Object.defineProperty(globalThis, "localStorage", {
    value: stub,
    writable: true,
    configurable: true,
  });
  return map;
}

const realLocation = window.location;

beforeEach(() => {
  setSearch("");
});

afterEach(() => {
  Object.defineProperty(window, "location", {
    value: realLocation,
    writable: true,
    configurable: true,
  });
});

describe("desktopAppVersion", () => {
  it("captures the version from the handoff URL", async () => {
    installStorage();
    setSearch("?desktop=1&app_version=0.4.5-dev.12.g1a2b3c4");
    const { desktopAppVersion } = await loadFresh();
    expect(desktopAppVersion()).toBe("0.4.5-dev.12.g1a2b3c4");
  });

  it("survives the router dropping the query string", async () => {
    const store = installStorage();
    setSearch("?desktop=1&app_version=0.4.5-dev.12.g1a2b3c4");
    await loadFresh();
    expect(store.get(STORAGE_KEY)).toBe("0.4.5-dev.12.g1a2b3c4");

    // A later page load with no query string left, as after SPA navigation.
    setSearch("");
    const { desktopAppVersion } = await loadFresh();
    expect(desktopAppVersion()).toBe("0.4.5-dev.12.g1a2b3c4");
  });

  it("returns empty in a plain browser tab", async () => {
    installStorage();
    const { desktopAppVersion } = await loadFresh();
    expect(desktopAppVersion()).toBe("");
  });

  it("prefers the URL over a stale stored value", async () => {
    // An app updated between launches must not keep reporting the version it
    // wrote last time.
    const store = installStorage();
    store.set(STORAGE_KEY, "0.4.4");
    setSearch("?desktop=1&app_version=0.4.5-dev.12.g1a2b3c4");
    const { desktopAppVersion } = await loadFresh();
    expect(desktopAppVersion()).toBe("0.4.5-dev.12.g1a2b3c4");
  });

  it("still reports the version when storage writes throw", async () => {
    installStorage({ throwOnWrite: true });
    setSearch("?desktop=1&app_version=0.4.5-dev.12.g1a2b3c4");
    const { desktopAppVersion } = await loadFresh();
    expect(desktopAppVersion()).toBe("0.4.5-dev.12.g1a2b3c4");
  });

  it("returns empty rather than throwing when storage is absent entirely", async () => {
    // Which is this jsdom setup's actual default, and any environment where the
    // bare `localStorage` reference is a ReferenceError.
    Reflect.deleteProperty(globalThis, "localStorage");
    setSearch("");
    const { desktopAppVersion } = await loadFresh();
    expect(desktopAppVersion()).toBe("");
  });
});
