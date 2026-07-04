/**
 * Live.test.tsx — unit tests for the Live view's "show stopped" toggle.
 *
 * Covers the real-time activity stream filter that hides stopped/errored
 * agents by default so the stream isn't swamped by idle noise.
 *
 * Invariants exercised here:
 *   - Default: stopped/errored agents are hidden; active ones render.
 *   - Badge shows "N active" and "M stopped (show)" counts.
 *   - Clicking "(show)" reveals the stopped agents.
 *   - The toggle persists across renders via localStorage
 *     (key "bc-live-show-stopped").
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, act } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Live, SHOW_STOPPED_STORAGE_KEY } from "./Live";
import { HeaderSlotProvider, useHeaderSlotContext } from "../context/HeaderSlotContext";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

function agent(name: string, state: string) {
  return {
    name,
    role: "engineer",
    tool: "claude",
    state,
    total_cost_usd: 0,
    started_at: "",
    created_at: "",
    updated_at: "",
  };
}

function mockAgentsApi(list: ReturnType<typeof agent>[]) {
  fetchMock.mockImplementation((url: string) => {
    if (url.includes("/agents")) return jsonResponse(list);
    return jsonResponse([]);
  });
}

/** Renders the header-slot content the way Layout's full-width bar does —
 *  Live's presence line + controls now live there, not in the page body. */
function HeaderHost() {
  const { slot } = useHeaderSlotContext();
  return (
    <div data-testid="header-host">
      {slot.title}
      {slot.actions}
    </div>
  );
}

function renderLive() {
  return render(
    <MemoryRouter>
      <HeaderSlotProvider>
        <HeaderHost />
        <Live />
      </HeaderSlotProvider>
    </MemoryRouter>,
  );
}

/**
 * Returns true iff an AgentCard for `name` is currently rendered. Cards render
 * a wrapper button with `title="Open <name> detail view"`, which disambiguates
 * the name from its appearance inside the agent-filter <select> dropdown.
 */
function agentCardVisible(name: string): boolean {
  return screen.queryByTitle(`Open ${name} detail view`) !== null;
}

async function waitForAgentCard(name: string) {
  await waitFor(() => {
    expect(agentCardVisible(name)).toBe(true);
  });
}

// vitest 4's jsdom doesn't plumb localStorage methods onto window by default;
// install a minimal in-memory shim so persistence tests can run.
function installLocalStorageShim() {
  const current = (window as unknown as { localStorage?: { setItem?: unknown } }).localStorage;
  if (current && typeof current.setItem === "function") return;
  const store = new Map<string, string>();
  const fake: Storage = {
    getItem(key: string) {
      return store.has(key) ? store.get(key)! : null;
    },
    setItem(key: string, value: string) {
      store.set(key, String(value));
    },
    removeItem(key: string) {
      store.delete(key);
    },
    clear() {
      store.clear();
    },
    key(index: number) {
      return Array.from(store.keys())[index] ?? null;
    },
    get length() {
      return store.size;
    },
  };
  Object.defineProperty(window, "localStorage", {
    configurable: true,
    value: fake,
  });
}

beforeEach(() => {
  fetchMock.mockReset();
  installLocalStorageShim();
  window.localStorage.clear();
});

describe("Live — show-stopped toggle", () => {
  it("hides stopped and error agents by default, shows active ones", async () => {
    mockAgentsApi([
      agent("alice", "working"),
      agent("bob", "idle"),
      agent("carol", "stopped"),
      agent("dave", "error"),
    ]);

    renderLive();

    await waitForAgentCard("alice");

    // Active agents render.
    expect(agentCardVisible("alice")).toBe(true);
    expect(agentCardVisible("bob")).toBe(true);

    // Stopped/error agents are filtered out of the card list.
    expect(agentCardVisible("carol")).toBe(false);
    expect(agentCardVisible("dave")).toBe(false);

    // The presence line reads as a sentence; stopped agents surface as a
    // "N stopped (hidden)" toggle woven into it (#3254 calm redesign).
    const badge = screen.getByTestId("live-state-badge");
    expect(badge.textContent).toContain("working");
    const toggle = screen.getByTestId("toggle-show-stopped");
    expect(toggle.textContent).toContain("stopped");
    expect(toggle.textContent).toContain("(hidden)");
    expect(toggle.textContent).toContain("2");
    expect(toggle.getAttribute("aria-pressed")).toBe("false");
  });

  it("toggle reveals stopped agents when clicked", async () => {
    mockAgentsApi([
      agent("alice", "working"),
      agent("carol", "stopped"),
      agent("dave", "error"),
    ]);

    renderLive();

    await waitForAgentCard("alice");
    expect(agentCardVisible("carol")).toBe(false);

    const toggle = screen.getByTestId("toggle-show-stopped");
    fireEvent.click(toggle);

    // After toggling, the stopped+error agents appear.
    await waitForAgentCard("carol");
    expect(agentCardVisible("dave")).toBe(true);

    // Toggle now announces the pressed state via aria-pressed.
    expect(screen.getByTestId("toggle-show-stopped").getAttribute("aria-pressed")).toBe("true");
  });

  it("persists the toggle state in localStorage", async () => {
    mockAgentsApi([
      agent("alice", "working"),
      agent("carol", "stopped"),
    ]);

    const { unmount } = renderLive();

    await waitForAgentCard("alice");

    // Flip it ON.
    act(() => {
      fireEvent.click(screen.getByTestId("toggle-show-stopped"));
    });

    await waitForAgentCard("carol");
    expect(window.localStorage.getItem(SHOW_STOPPED_STORAGE_KEY)).toBe("1");

    // Remount: the setting should survive.
    unmount();
    renderLive();

    await waitForAgentCard("alice");
    // Stopped agent visible immediately on second mount — no toggle click needed.
    expect(agentCardVisible("carol")).toBe(true);
    expect(screen.getByTestId("toggle-show-stopped").getAttribute("aria-pressed")).toBe("true");
  });

  it("reads existing localStorage value on first mount", async () => {
    window.localStorage.setItem(SHOW_STOPPED_STORAGE_KEY, "1");
    mockAgentsApi([
      agent("alice", "working"),
      agent("carol", "stopped"),
    ]);

    renderLive();

    await waitForAgentCard("alice");
    // Stopped agent is visible because localStorage said so.
    expect(agentCardVisible("carol")).toBe(true);
  });
});
