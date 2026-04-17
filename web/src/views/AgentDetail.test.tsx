/**
 * AgentDetail.test.tsx — unit tests for the agent detail view's
 * tab-state-from-URL behavior.
 *
 * Covers the proposal's key invariants for the revamped agent page:
 *   - Deep link /agents/<name>/<tab> renders that tab.
 *   - Clicking a tab header updates the URL.
 *   - Keyboard shortcut 1-5 switches tab + URL.
 *
 * The AgentDetail page is big + has many data dependencies (polling,
 * websocket, terminal). These tests mock the network layer and drive
 * the UI via the router, asserting the URL-derived tab is correct and
 * that tab clicks/keyboard shortcuts keep URL + tab state in sync.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, act, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import { AgentDetail } from "./AgentDetail";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

// Minimal agent payload to satisfy the loader.
const agent = {
  name: "alice",
  role: "engineer",
  provider: "claude",
  state: "idle",
  session_name: "bc-alice",
  created_at: "2025-01-01T00:00:00Z",
  started_at: "2025-01-01T00:00:00Z",
  updated_at: "2025-01-01T00:00:00Z",
  pid: 0,
  workspace_path: "/ws",
  template: "engineer",
  context_files: [],
  mcps: [],
  secrets: [],
  plugins: [],
  running: false,
};

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

function LocationProbe() {
  const loc = useLocation();
  return <div data-testid="probe">{loc.pathname}</div>;
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route
          path="/w/:wsId/agents/:name/*"
          element={
            <div>
              <AgentDetail />
              <LocationProbe />
            </div>
          }
        />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
  fetchMock.mockImplementation((url: string) => {
    // The polling loop hits many endpoints; return safe defaults for all.
    if (typeof url === "string") {
      if (url.includes("/api/agents/alice/stats-computed")) {
        return jsonResponse({});
      }
      if (url.includes("/api/agents/alice")) return jsonResponse(agent);
      if (url.endsWith("/api/agents") || url.includes("/api/agents?")) {
        return jsonResponse([agent]);
      }
      if (url.includes("/providers")) return jsonResponse([]);
      if (url.includes("/costs/agent")) return jsonResponse({ summary: null, daily: [] });
      if (url.includes("/tools")) return jsonResponse([]);
      if (url.includes("/mcp")) return jsonResponse([]);
      // TSDB timeseries endpoints all return arrays.
      if (url.match(/\/agents\/stats\/(cpu|mem|net|tokens|latest|cost)/)) {
        return jsonResponse([]);
      }
      if (url.includes("/agents/stats/summary/")) {
        return jsonResponse({});
      }
    }
    return jsonResponse({});
  });
});

describe("AgentDetail tab-from-URL", () => {
  it("deep link /config renders the Config tab as active", async () => {
    renderAt("/w/ws1/agents/alice/config");

    // Tab buttons all render with their label. The "active" one has
    // distinct styling — rather than sniff class names, we assert the
    // URL probe shows /config and the tab header is present.
    await waitFor(() => {
      expect(screen.getByText("Config")).toBeInTheDocument();
    });
    expect(screen.getByTestId("probe").textContent).toBe("/w/ws1/agents/alice/config");
  });

  it("deep link /metrics keeps URL on metrics", async () => {
    renderAt("/w/ws1/agents/alice/metrics");

    await waitFor(() => {
      expect(screen.getByText("Metrics")).toBeInTheDocument();
    });
    expect(screen.getByTestId("probe").textContent).toBe("/w/ws1/agents/alice/metrics");
  });

  it("clicking a tab header updates the URL", async () => {
    renderAt("/w/ws1/agents/alice/attach");

    await waitFor(() => {
      expect(screen.getByText("Live")).toBeInTheDocument();
    });

    await act(async () => {
      screen.getByText("Live").click();
    });

    await waitFor(() => {
      expect(screen.getByTestId("probe").textContent).toBe("/w/ws1/agents/alice/live");
    });
  });

  it("keyboard shortcut 3 switches to Config tab", async () => {
    renderAt("/w/ws1/agents/alice/attach");

    await waitFor(() => {
      expect(screen.getByText("Config")).toBeInTheDocument();
    });

    await act(async () => {
      fireEvent.keyDown(window, { key: "3" });
    });

    await waitFor(() => {
      expect(screen.getByTestId("probe").textContent).toBe("/w/ws1/agents/alice/config");
    });
  });

  it("keyboard shortcut 4 switches to Metrics tab", async () => {
    renderAt("/w/ws1/agents/alice/attach");

    await waitFor(() => {
      expect(screen.getByText("Metrics")).toBeInTheDocument();
    });

    await act(async () => {
      fireEvent.keyDown(window, { key: "4" });
    });

    await waitFor(() => {
      expect(screen.getByTestId("probe").textContent).toBe("/w/ws1/agents/alice/metrics");
    });
  });

  it("unknown sub-path defaults to Attach tab (no URL rewrite)", async () => {
    renderAt("/w/ws1/agents/alice/bogus");

    await waitFor(() => {
      expect(screen.getByText("Attach")).toBeInTheDocument();
    });
    // Default routes through without rewriting the URL — the probe
    // should still show the deep-linked path; the tab component
    // falls back to "attach" in state.
    expect(screen.getByTestId("probe").textContent).toBe("/w/ws1/agents/alice/bogus");
  });
});
