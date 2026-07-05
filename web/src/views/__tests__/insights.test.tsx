import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { Insights } from "../Insights";
import { AppRoutes } from "../../App";
import { HeaderSlotProvider } from "../../context/HeaderSlotContext";
import { ThemeProvider } from "../../context/ThemeContext";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

/** Route-aware API mock for the single-page Stats dashboard. */
function mockApi() {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    // Per-agent cost ledger — the single source of truth for the agents
    // table token/cost columns, Cost by Agent, and Agent Token Breakdown.
    if (u.includes("/api/costs/agents")) {
      return jsonResponse([
        {
          agent_id: "bc-bc-bot-1",
          total_cost_usd: 8.5,
          input_tokens: 800_000,
          output_tokens: 200_000,
          cache_read_tokens: 400_000,
          cache_write_tokens: 50_000,
          total_tokens: 1_000_000,
          record_count: 30,
        },
        {
          agent_id: "bc-bc-bot-2",
          total_cost_usd: 3.84,
          input_tokens: 400_000,
          output_tokens: 100_000,
          cache_read_tokens: 100_000,
          cache_write_tokens: 20_000,
          total_tokens: 500_000,
          record_count: 12,
        },
      ]);
    }
    // Per-model cost ledger — Cost by Model + Model Usage (Tokens).
    if (u.includes("/api/costs/models")) {
      return jsonResponse([
        {
          model: "claude-opus-4-6",
          total_cost_usd: 10.0,
          input_tokens: 900_000,
          output_tokens: 250_000,
          total_tokens: 1_150_000,
          record_count: 24,
        },
        {
          model: "claude-sonnet-4-6",
          total_cost_usd: 2.34,
          input_tokens: 300_000,
          output_tokens: 50_000,
          total_tokens: 350_000,
          record_count: 18,
        },
      ]);
    }
    // Daily cost ledger — Cost Over Time, Token Throughput, Spend/Tokens/Burn.
    if (u.includes("/api/costs/daily")) {
      return jsonResponse([
        { date: "2026-07-04", cost_usd: 5.0, total_tokens: 700_000, input_tokens: 560_000, output_tokens: 140_000, record_count: 20 },
        { date: "2026-07-05", cost_usd: 7.34, total_tokens: 800_000, input_tokens: 640_000, output_tokens: 160_000, record_count: 22 },
      ]);
    }
    // Cost summary — cache efficiency headline + totals.
    if (u.includes("/api/costs")) {
      return jsonResponse({
        input_tokens: 1_200_000,
        output_tokens: 300_000,
        cache_read_tokens: 500_000,
        cache_write_tokens: 70_000,
        total_tokens: 1_500_000,
        total_cost_usd: 12.34,
        record_count: 42,
      });
    }
    if (u.includes("/api/system/info")) {
      return jsonResponse({ hostname: "test-host", os: "darwin", arch: "arm64" });
    }
    if (u.includes("/api/health")) return jsonResponse({ status: "ok" });
    return jsonResponse([]);
  });
}

function renderInsights(entry = "/insights") {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <HeaderSlotProvider>
        <Insights />
      </HeaderSlotProvider>
    </MemoryRouter>,
  );
}

function LocationProbe() {
  const loc = useLocation();
  return <div data-testid="loc">{loc.pathname + loc.search}</div>;
}

beforeEach(() => {
  fetchMock.mockReset();
  window.localStorage?.clear();
  mockApi();
});

describe("Insights", () => {
  it("renders one dashboard with no tab bar", async () => {
    renderInsights();

    // The old Metrics/Costs tab bar is gone.
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();

    // Chart panels arrive once polling settles.
    await waitFor(() => expect(screen.getByText("No CPU data")).toBeInTheDocument());
  });

  it("shows the KPI strip", async () => {
    renderInsights();

    // The dashboard mounts once the first poll settles (a skeleton shows
    // until then), so wait for a KPI tile label to appear.
    await waitFor(() => expect(screen.getByText("Spend (this range)")).toBeInTheDocument());
    expect(screen.getByText("Active agents")).toBeInTheDocument();
    expect(screen.getByText("Burn rate")).toBeInTheDocument();
  });

  it("wires KPIs off the cost ledger", async () => {
    renderInsights();

    // Spend sums the daily ledger (5.00 + 7.34 = 12.34); the top cost driver
    // is the biggest per-agent spender with its "bc-bc-" prefix stripped.
    await waitFor(() => expect(screen.getByText("Top cost driver")).toBeInTheDocument());
    expect(screen.getByText("bot-1")).toBeInTheDocument();
    expect(screen.getByText("$12.34")).toBeInTheDocument();
  });

  it("renders the grouped section headers", async () => {
    renderInsights();

    // Section headers double as anchor-nav targets; the pill labels and
    // the headers share text, so each label appears at least once.
    await waitFor(() => expect(screen.getByText("No CPU data")).toBeInTheDocument());
    for (const label of ["Cost", "Usage", "System", "Activity"]) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
  });
});

describe("Insights redirects", () => {
  function renderApp(entry: string) {
    return render(
      <ThemeProvider>
        <MemoryRouter initialEntries={[entry]}>
          <LocationProbe />
          <AppRoutes />
        </MemoryRouter>
      </ThemeProvider>,
    );
  }

  it("redirects /costs to /insights", async () => {
    renderApp("/costs");
    await waitFor(() =>
      expect(screen.getByTestId("loc")).toHaveTextContent("/insights"),
    );
  });

  it("redirects /stats and /metrics to /insights", async () => {
    const { unmount } = renderApp("/stats");
    await waitFor(() =>
      expect(screen.getByTestId("loc")).toHaveTextContent("/insights"),
    );
    unmount();

    renderApp("/metrics");
    await waitFor(() =>
      expect(screen.getByTestId("loc")).toHaveTextContent("/insights"),
    );
  });
});
