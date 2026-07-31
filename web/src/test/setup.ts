import "@testing-library/jest-dom/vitest";
import { vi, beforeEach } from "vitest";
import { __resetCache } from "../api/cache";

globalThis.fetch = vi.fn();

// The module-level API cache persists across tests (it's shared app state).
// Reset it before each test so a cached read from one test never masks a
// fetch the next test asserts on.
beforeEach(() => {
  __resetCache();
});

class FakeEventSource {
  onopen: (() => void) | null = null;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  close() {}
}
globalThis.EventSource = FakeEventSource as unknown as typeof EventSource;

// xterm (used by WebTerminal) calls window.matchMedia during open();
// jsdom does not provide it, so we install a minimal stub.
if (typeof window !== "undefined" && !window.matchMedia) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (q: string) => ({
      matches: false,
      media: q,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      onchange: null,
      dispatchEvent: () => false,
    }),
  });
}

// jsdom doesn't have ResizeObserver; xterm uses it.
if (typeof globalThis.ResizeObserver === "undefined") {
  class FakeResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  (globalThis as unknown as { ResizeObserver: typeof FakeResizeObserver }).ResizeObserver = FakeResizeObserver;
}
