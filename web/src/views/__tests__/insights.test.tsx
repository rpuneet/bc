import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { Insights, summarizeActivity } from "../Insights";
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

/** UTC day key, matching the ledger's day bucketing. */
function dayKey(offsetDays = 0): string {
  return new Date(Date.now() - offsetDays * 86_400_000).toISOString().slice(0, 10);
}

/** Route-aware API mock for the redesigned Insights page. */
function mockApi() {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    // Per-agent ledger — the "Where it goes" default dimension.
    if (u.includes("/api/costs/agents")) {
      return jsonResponse([
        {
          agent_id: "mycel-ab12cd-bot-1",
          total_cost_usd: 8.5,
          input_tokens: 800_000,
          output_tokens: 200_000,
          cache_read_tokens: 400_000,
          cache_write_tokens: 50_000,
          total_tokens: 1_000_000,
          record_count: 30,
        },
        {
          agent_id: "mycel-ab12cd-bot-2",
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
    // Per-model ledger — the Models dimension.
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
      ]);
    }
    // Daily ledger — spend chart + stat band.
    if (u.includes("/api/costs/daily")) {
      return jsonResponse([
        { date: dayKey(1), cost_usd: 5.0, total_tokens: 700_000, input_tokens: 560_000, output_tokens: 140_000, record_count: 20 },
        { date: dayKey(0), cost_usd: 7.34, total_tokens: 800_000, input_tokens: 640_000, output_tokens: 160_000, record_count: 22 },
      ]);
    }
    // Per-repo rollup — the Repos dimension.
    if (u.includes("/api/global/costs")) {
      return jsonResponse({
        range: { start: "2026-01-01T00:00:00Z" },
        groupBy: "repo",
        rows: [
          { key: "/repos/alpha", label: "alpha", total: 9.0 },
          { key: "/old/alpha", label: "alpha", total: 1.0 },
          { key: "/repos/beta", label: "beta", total: 2.34 },
        ],
      });
    }
    // Recent hook events — the Activity chart.
    if (u.includes("/api/agents/activity")) {
      const now = Date.now();
      return jsonResponse([
        { timestamp: new Date(now).toISOString(), event: "PreToolUse", agent: "bot-1" },
        { timestamp: new Date(now - 60_000).toISOString(), event: "PostToolUse", agent: "bot-1" },
        { timestamp: new Date(now - 120_000).toISOString(), event: "UserPromptSubmit", agent: "bot-2" },
      ]);
    }
    // Period-scoped cost summary — tokens + cache efficiency.
    if (u.includes("/api/costs")) {
      return jsonResponse({
        input_tokens: 1_000_000,
        output_tokens: 300_000,
        cache_read_tokens: 9_000_000,
        cache_write_tokens: 70_000,
        total_tokens: 1_300_000,
        total_cost_usd: 12.34,
        record_count: 42,
      });
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
  it("renders the four-question stat band off the ledger", async () => {
    renderInsights();

    // Spend sums the daily ledger inside the window (5.00 + 7.34).
    await waitFor(() => expect(screen.getByText("$12.34")).toBeInTheDocument());
    expect(screen.getByText(/Spend · last 30d/i)).toBeInTheDocument();
    expect(screen.getByText("Today")).toBeInTheDocument();
    // Today's ledger day (7.34) renders in the Today cell.
    expect(screen.getByText("$7.34")).toBeInTheDocument();
    // Cache hit rate = 9M / (9M + 1M) fresh input; the figure shows in
    // both the stat band and the cache-efficiency module.
    expect(screen.getByText("Cache hit rate")).toBeInTheDocument();
    expect(screen.getAllByText("90.0%").length).toBeGreaterThan(0);
    // Tokens = input + output from the period summary.
    expect(screen.getByText("1.3M")).toBeInTheDocument();
  });

  it("scopes cost queries to the selected period via since", async () => {
    renderInsights();
    await waitFor(() => expect(screen.getByText("$12.34")).toBeInTheDocument());

    const urls = fetchMock.mock.calls.map((c) => String(c[0]));
    const agentsCall = urls.find((u) => u.includes("/api/costs/agents"));
    expect(agentsCall).toMatch(/since=\d{4}-\d{2}-\d{2}/);
    // The daily fetch doubles the window for the period delta.
    const dailyCall = urls.find((u) => u.includes("/api/costs/daily"));
    expect(dailyCall).toContain("days=60");
  });

  it("breaks down spend by agent, model, and repo behind one switch", async () => {
    renderInsights();

    // Default dimension: agents, prefix-stripped, with share of total.
    await waitFor(() => expect(screen.getByText("bot-1")).toBeInTheDocument());
    expect(screen.getByText("bot-2")).toBeInTheDocument();
    expect(screen.getByText("$8.50")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Models" }));
    await waitFor(() => expect(screen.getByText("claude-opus-4-6")).toBeInTheDocument());

    // Repo rows with the same label fold together (9.0 + 1.0), and since
    // that merges two distinct historical paths, the row is flagged with
    // a "×2" badge so the fold isn't silent.
    fireEvent.click(screen.getByRole("button", { name: "Repos" }));
    await waitFor(() => expect(screen.getByText("alpha")).toBeInTheDocument());
    expect(screen.getByText("$10.00")).toBeInTheDocument();
    expect(screen.getByText("×2")).toBeInTheDocument();
    // beta has only one folded path, so it gets no badge.
    expect(screen.getByText("beta")).toBeInTheDocument();
  });

  it("labels the activity window honestly", async () => {
    renderInsights();
    await waitFor(() => expect(screen.getByText(/3 events/)).toBeInTheDocument());
    expect(screen.getByText("Tool calls")).toBeInTheDocument();
  });

  it("drops the old KPI-soup dashboard", async () => {
    renderInsights();
    await waitFor(() => expect(screen.getByText("$12.34")).toBeInTheDocument());
    for (const gone of ["Burn rate", "CPU by Agent (%)", "Notification Activity (Top 10)", "Active agents"]) {
      expect(screen.queryByText(gone)).not.toBeInTheDocument();
    }
  });
});

describe("summarizeActivity", () => {
  it("buckets events by category over the covered window", () => {
    const base = Date.parse("2026-07-30T10:00:00Z");
    const items = [
      { timestamp: new Date(base).toISOString(), event: "PreToolUse", agent: "a" },
      { timestamp: new Date(base + 30_000).toISOString(), event: "PostToolUse", agent: "a" },
      { timestamp: new Date(base + 60_000).toISOString(), event: "UserPromptSubmit", agent: "b" },
      { timestamp: new Date(base + 90_000).toISOString(), event: "SubagentStop", agent: "a" },
    ];
    const s = summarizeActivity(items);
    expect(s.eventCount).toBe(4);
    expect(s.agentCount).toBe(2);
    expect(s.bucketMinutes).toBe(1);
    const totals = s.buckets.reduce(
      (acc, b) => ({ tools: acc.tools + b.tools, prompts: acc.prompts + b.prompts, other: acc.other + b.other }),
      { tools: 0, prompts: 0, other: 0 },
    );
    expect(totals).toEqual({ tools: 2, prompts: 1, other: 1 });
  });

  it("handles an empty feed", () => {
    const s = summarizeActivity([]);
    expect(s.buckets).toEqual([]);
    expect(s.eventCount).toBe(0);
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
