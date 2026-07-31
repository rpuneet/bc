/**
 * homeModules.test.tsx — the Home page's dense grid modules and the
 * /live→/ redirect.
 *
 * Covers:
 *   - /live redirects to the Home index ("/").
 *   - OverviewStrip renders live counts from mocked endpoints.
 *   - ActivityFeed renders channel messages as compact rows.
 *   - CostCharts fetches costs scoped to today via `?since=<today>`.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { AppRoutes } from "../../../App";
import { OverviewStrip } from "../OverviewStrip";
import { ActivityFeed } from "../ActivityFeed";
import { CostCharts } from "../CostCharts";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

function wrap(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

const fleet = { working: 2, idle: 1, stuck: 0, stopped: 3, total: 6 };

beforeEach(() => {
  fetchMock.mockReset();
});

// ── /live → / redirect ───────────────────────────────────────────────

let lastLocation = "";
function LocationSpy() {
  const loc = useLocation();
  lastLocation = loc.pathname;
  return null;
}

describe("Home routing", () => {
  it("/live redirects to the Home index", async () => {
    fetchMock.mockReturnValue(jsonResponse([]));
    render(
      <MemoryRouter initialEntries={["/live"]}>
        <LocationSpy />
        <AppRoutes />
      </MemoryRouter>,
    );
    await waitFor(() => {
      expect(lastLocation).toBe("/");
    });
  });
});

// ── OverviewStrip ────────────────────────────────────────────────────

describe("OverviewStrip", () => {
  it("renders live counts from mocked endpoints", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.includes("/apps/channels")) {
        return jsonResponse([
          { name: "slack:eng", description: "", members: [], member_count: 3 },
          { name: "telegram:ops", description: "", members: [], member_count: 1 },
        ]);
      }
      if (u.includes("/apps")) {
        return jsonResponse({
          catalog: [],
          instances: [
            { name: "slack", app: "slack", enabled: true, connected: true },
            { name: "telegram", app: "telegram", enabled: true, connected: false },
          ],
        });
      }
      if (u.includes("/costs/daily")) {
        return jsonResponse([{ date: new Date().toISOString().slice(0, 10), cost_usd: 4.2, total_tokens: 0, record_count: 0, input_tokens: 0, output_tokens: 0 }]);
      }
      return jsonResponse([]);
    });

    wrap(<OverviewStrip summary={fleet} eventCount={0} connected />);

    // Agents cell: active (working+idle+stuck) over total.
    const agents = await screen.findByTestId("overview-agents");
    expect(agents.textContent).toContain("3");
    expect(agents.textContent).toContain("6");

    // Apps: one connected of two instances.
    await waitFor(() => {
      expect(screen.getByTestId("overview-apps").textContent).toContain("1");
    });
    // Channels: two gateway sources.
    expect(screen.getByTestId("overview-channels").textContent).toContain("2");
    // Spend today from the daily ledger.
    await waitFor(() => {
      expect(screen.getByTestId("overview-spend").textContent).toContain("$4.20");
    });
  });
});

// ── ActivityFeed ─────────────────────────────────────────────────────

describe("ActivityFeed", () => {
  it("renders recent channel messages as compact rows", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.includes("/apps/channels/") && u.includes("/history")) {
        return jsonResponse([
          { id: 1, sender: "alice", content: "deploy is green", created_at: new Date().toISOString() },
        ]);
      }
      if (u.includes("/apps/channels")) {
        return jsonResponse([{ name: "slack:eng", description: "", members: [], member_count: 2 }]);
      }
      if (u.includes("/stats/channels")) {
        return jsonResponse([{ name: "slack:eng", message_count: 5, member_count: 2, last_activity: new Date().toISOString(), top_senders: [] }]);
      }
      return jsonResponse([]);
    });

    wrap(<ActivityFeed />);

    expect(await screen.findByText("deploy is green")).toBeInTheDocument();
    expect(screen.getByText("alice")).toBeInTheDocument();
    // Header links to the full activity page.
    expect(screen.getByRole("link", { name: /view all/ })).toHaveAttribute("href", "/apps/activity");
  });
});

// ── CostCharts ───────────────────────────────────────────────────────

describe("CostCharts", () => {
  it("fetches top agents scoped to today via ?since=<today>", async () => {
    const today = new Date().toISOString().slice(0, 10);
    const seen: string[] = [];
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      seen.push(u);
      if (u.includes("/costs/agents")) {
        return jsonResponse([
          { agent_id: "mycel-bc-zen-zebra", total_cost_usd: 12.5, input_tokens: 0, output_tokens: 0, total_tokens: 0, record_count: 1 },
        ]);
      }
      if (u.includes("/costs/daily")) {
        return jsonResponse([{ date: today, cost_usd: 12.5, total_tokens: 0, record_count: 1, input_tokens: 0, output_tokens: 0 }]);
      }
      return jsonResponse([]);
    });

    wrap(<CostCharts />);

    await waitFor(() => {
      expect(seen.some((u) => u.includes("/costs/agents") && u.includes(`since=${today}`))).toBe(true);
    });
    // The top-agent bar list renders the stripped agent name + cost.
    expect(await screen.findByText("zen-zebra")).toBeInTheDocument();
    expect(screen.getByText("$12.50")).toBeInTheDocument();
  });
});
