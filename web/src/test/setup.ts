import "@testing-library/jest-dom/vitest";
import { vi } from "vitest";

globalThis.fetch = vi.fn();

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
