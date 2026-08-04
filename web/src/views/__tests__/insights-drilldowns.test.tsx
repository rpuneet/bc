/**
 * Round-2 Insights: progressive-disclosure drill-downs.
 *
 * Covers the system tile row (live snapshot + expand/collapse), the
 * token composition panel, breakdown row drill-downs (lazy agent
 * ledger fetch), and URL-hash restore of expanded state.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Insights } from "../Insights";
import { buildTokenSeries } from "../insights/TokenPanel";
import { pushSample, pivotAgentSeries } from "../insights/SystemRow";
import type { SysSample } from "../insights/SystemRow";
import { HeaderSlotProvider } from "../../context/HeaderSlotContext";
import type { SystemStats } from "../../api/client";

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

const SNAPSHOT: SystemStats = {
  hostname: "test-host",
  os: "darwin",
  arch: "arm64",
  cpus: 12,
  cpu_usage_percent: 62.4,
  memory_total_bytes: 24 * 1024 ** 3,
  memory_used_bytes: 9.6 * 1024 ** 3,
  memory_usage_percent: 40,
  disk_total_bytes: 460 * 1024 ** 3,
  disk_used_bytes: 363.4 * 1024 ** 3,
  disk_usage_percent: 79,
  go_version: "go1.25",
  uptime_seconds: 1000,
  goroutines: 70,
};

function mockApi() {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    if (u.includes("/api/stats/system")) return jsonResponse(SNAPSHOT);
    if (u.includes("/api/agents/stats/latest")) return jsonResponse([]);
    if (u.includes("/api/agents/stats/cpu")) {
      const now = Date.now();
      return jsonResponse([
        {
          time: new Date(now - 60_000).toISOString(),
          agent_name: "bot-1",
          role: "engineer", tool: "claude", runtime: "tmux", state: "working",
          cpu_percent: 12.5, mem_used_bytes: 0, mem_limit_bytes: 0, mem_percent: 0,
          net_rx_bytes: 0, net_tx_bytes: 0, disk_read_bytes: 0, disk_write_bytes: 0,
        },
        {
          time: new Date(now).toISOString(),
          agent_name: "bot-1",
          role: "engineer", tool: "claude", runtime: "tmux", state: "working",
          cpu_percent: 20, mem_used_bytes: 0, mem_limit_bytes: 0, mem_percent: 0,
          net_rx_bytes: 0, net_tx_bytes: 0, disk_read_bytes: 0, disk_write_bytes: 0,
        },
      ]);
    }
    if (u.match(/\/api\/agents\/stats\/(mem|net|disk)/)) return jsonResponse([]);
    if (u.includes("/api/agents/activity")) {
      const now = Date.now();
      return jsonResponse([
        { timestamp: new Date(now).toISOString(), event: "PreToolUse", agent: "bot-1" },
        { timestamp: new Date(now - 60_000).toISOString(), event: "PostToolUse", agent: "bot-1" },
        { timestamp: new Date(now - 120_000).toISOString(), event: "UserPromptSubmit", agent: "bot-2" },
      ]);
    }
    if (u.includes("/api/agents") && !u.includes("/api/agents/")) {
      return jsonResponse([
        { name: "bot-1", role: "engineer", tool: "claude", state: "working", total_cost_usd: 8.5, started_at: "", created_at: "", updated_at: "" },
      ]);
    }
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
    if (u.includes("/api/costs/agent/")) {
      return jsonResponse({
        summary: {
          agent_id: "mycel-ab12cd-bot-1",
          total_cost_usd: 42.0,
          input_tokens: 2_000_000,
          output_tokens: 500_000,
          total_tokens: 2_500_000,
          record_count: 90,
        },
        daily: [
          { agent_id: "mycel-ab12cd-bot-1", date: dayKey(1), cost_usd: 4.0, total_tokens: 350_000, record_count: 10, input_tokens: 280_000, output_tokens: 70_000 },
          { agent_id: "mycel-ab12cd-bot-1", date: dayKey(0), cost_usd: 4.5, total_tokens: 380_000, record_count: 12, input_tokens: 300_000, output_tokens: 80_000 },
        ],
      });
    }
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
    if (u.includes("/api/costs/daily")) {
      return jsonResponse([
        { date: dayKey(1), cost_usd: 5.0, total_tokens: 700_000, input_tokens: 560_000, output_tokens: 140_000, record_count: 20 },
        { date: dayKey(0), cost_usd: 7.34, total_tokens: 800_000, input_tokens: 640_000, output_tokens: 160_000, record_count: 22 },
      ]);
    }
    if (u.includes("/api/global/costs")) {
      return jsonResponse({
        range: { start: "2026-01-01T00:00:00Z" },
        groupBy: "repo",
        rows: [
          { key: "/repos/alpha", label: "alpha", total: 9.0 },
          { key: "/old/alpha", label: "alpha", total: 1.0 },
        ],
      });
    }
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
    return jsonResponse([]);
  });
}

function renderInsights() {
  return render(
    <MemoryRouter initialEntries={["/insights"]}>
      <HeaderSlotProvider>
        <Insights />
      </HeaderSlotProvider>
    </MemoryRouter>,
  );
}

function setHash(hash: string) {
  window.history.replaceState(null, "", `/insights${hash}`);
}

beforeEach(() => {
  fetchMock.mockReset();
  window.localStorage?.clear();
  window.sessionStorage?.clear();
  setHash("");
  mockApi();
});

describe("system row", () => {
  it("renders the four vitals tiles from the live snapshot", async () => {
    renderInsights();
    await waitFor(() => expect(screen.getByText("62.4%")).toBeInTheDocument());
    expect(screen.getByText("12 cores")).toBeInTheDocument();
    // Memory used + share of total.
    expect(screen.getByText("9.6 GB")).toBeInTheDocument();
    expect(screen.getByText("40% of 24.0 GB")).toBeInTheDocument();
    // Disk usage.
    expect(screen.getByText("79%")).toBeInTheDocument();
    // No container metrics → the network tile says so instead of faking.
    expect(screen.getByText("no container metrics")).toBeInTheDocument();
  });

  it("expands one tile at a time, lazily fetches the per-agent split, closes on Escape", async () => {
    renderInsights();
    await waitFor(() => expect(screen.getByText("62.4%")).toBeInTheDocument());

    // No per-agent fetch before opening.
    expect(fetchMock.mock.calls.map((c) => String(c[0]))).not.toContainEqual(
      expect.stringContaining("/api/agents/stats/cpu"),
    );

    fireEvent.click(screen.getByRole("button", { name: /^CPU/ }));
    expect(window.location.hash).toBe("#sys=cpu");
    await waitFor(() => expect(screen.getByText("CPU · host")).toBeInTheDocument());
    await waitFor(() =>
      expect(fetchMock.mock.calls.map((c) => String(c[0]))).toContainEqual(
        expect.stringContaining("/api/agents/stats/cpu"),
      ),
    );

    // Switching tiles swaps the panel (one open at a time).
    fireEvent.click(screen.getByRole("button", { name: /^Memory/ }));
    expect(window.location.hash).toBe("#sys=memory");
    await waitFor(() => expect(screen.getByText("Memory · host")).toBeInTheDocument());
    expect(screen.queryByText("CPU · host")).not.toBeInTheDocument();

    // Esc collapses and cleans the hash.
    fireEvent.keyDown(window, { key: "Escape" });
    expect(window.location.hash).toBe("");
  });

  it("restores the expanded tile from the URL hash", async () => {
    setHash("#sys=cpu");
    renderInsights();
    await waitFor(() => expect(screen.getByText("CPU · host")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: /^CPU/ })).toHaveAttribute("aria-expanded", "true");
  });
});

describe("token drill-down", () => {
  it("expands from the Tokens stat with composition legend and cache totals", async () => {
    renderInsights();
    await waitFor(() => expect(screen.getByText("$12.34")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: /Tokens ·/ }));
    expect(window.location.hash).toBe("#tokens=1");
    await waitFor(() => expect(screen.getByText(/Token composition/)).toBeInTheDocument());
    expect(screen.getByText("All tokens processed")).toBeInTheDocument();
    // Cache reads dominate: 9M of 10.37M processed tokens.
    expect(screen.getByText(/86\.8%/)).toBeInTheDocument();

    // Toggle back closed.
    fireEvent.click(screen.getByRole("button", { name: /Tokens ·/ }));
    expect(window.location.hash).toBe("");
  });

  it("maps the daily ledger into a zero-filled stacked series", () => {
    const daily = [
      { date: "2026-07-28", cost_usd: 1, total_tokens: 30, record_count: 1, input_tokens: 20, output_tokens: 10 },
      { date: "2026-07-30", cost_usd: 2, total_tokens: 70, record_count: 2, input_tokens: 50, output_tokens: 20 },
    ];
    const series = buildTokenSeries(daily, ["2026-07-28", "2026-07-29", "2026-07-30"]);
    expect(series).toEqual([
      { date: "2026-07-28", input: 20, output: 10 },
      { date: "2026-07-29", input: 0, output: 0 },
      { date: "2026-07-30", input: 50, output: 20 },
    ]);
  });
});

describe("breakdown drill-down", () => {
  it("expands an agent row, lazily fetching that agent's ledger", async () => {
    renderInsights();
    await waitFor(() => expect(screen.getByText("bot-1")).toBeInTheDocument());

    // No agent-detail fetch before the row is opened.
    expect(fetchMock.mock.calls.map((c) => String(c[0]))).not.toContainEqual(
      expect.stringContaining("/api/costs/agent/mycel-ab12cd-bot-1"),
    );

    fireEvent.click(screen.getByRole("button", { name: "Expand bot-1" }));
    expect(window.location.hash).toBe("#row=agent%3Amycel-ab12cd-bot-1");

    await waitFor(() =>
      expect(fetchMock.mock.calls.map((c) => String(c[0]))).toContainEqual(
        expect.stringContaining("/api/costs/agent/mycel-ab12cd-bot-1"),
      ),
    );
    // Period-scoped stats from the breakdown fetch + the link row.
    await waitFor(() => expect(screen.getByText("Estimated spend over time")).toBeInTheDocument());
    expect(screen.getByRole("link", { name: /Open agent/ })).toHaveAttribute(
      "href",
      "/agents/bot-1",
    );
  });

  it("shows a model row's period token split without any extra fetch", async () => {
    renderInsights();
    await waitFor(() => expect(screen.getByText("bot-1")).toBeInTheDocument());
    const before = fetchMock.mock.calls.length;

    fireEvent.click(screen.getByRole("button", { name: "Models" }));
    await waitFor(() => expect(screen.getByText("claude-opus-4-6")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Expand claude-opus-4-6" }));
    expect(window.location.hash).toBe("#row=model%3Aclaude-opus-4-6");

    await waitFor(() => expect(screen.getByText("Per call")).toBeInTheDocument());
    // 24 calls over $10.00.
    expect(screen.getByText("$0.42")).toBeInTheDocument();
    expect(fetchMock.mock.calls.length).toBe(before);
  });

  it("restores an expanded breakdown row from the URL hash", async () => {
    setHash("#row=agent%3Amycel-ab12cd-bot-1");
    renderInsights();
    await waitFor(() => expect(screen.getByText("Estimated spend over time")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Collapse bot-1" })).toBeInTheDocument();
  });
});

describe("system sampling helpers", () => {
  it("pushSample appends and trims to the 15-minute window", () => {
    const now = Date.now();
    const old: SysSample = {
      t: now - 16 * 60_000, cpu: 1, memPct: 1, memUsed: 1, memTotal: 2, diskPct: 1, diskUsed: 1, diskTotal: 2,
    };
    const recent: SysSample = { ...old, t: now - 60_000 };
    const next = pushSample([old, recent], SNAPSHOT, now);
    expect(next.map((p) => p.t)).toEqual([recent.t, now]);
    expect(next[1]).toMatchObject({ cpu: 62.4, memTotal: SNAPSHOT.memory_total_bytes });
  });

  it("pivotAgentSeries buckets per-agent samples by time and zero-fills", () => {
    const t0 = Date.parse("2026-07-30T10:00:00Z");
    const t1 = t0 + 30_000;
    const base = {
      role: "", tool: "", runtime: "tmux", state: "working",
      mem_used_bytes: 0, mem_limit_bytes: 0, mem_percent: 0,
      net_rx_bytes: 0, net_tx_bytes: 0, disk_read_bytes: 0, disk_write_bytes: 0,
    };
    const series = pivotAgentSeries(
      [
        { ...base, time: new Date(t0).toISOString(), agent_name: "a", cpu_percent: 10 },
        { ...base, time: new Date(t1).toISOString(), agent_name: "a", cpu_percent: 20 },
        { ...base, time: new Date(t1).toISOString(), agent_name: "b", cpu_percent: 5 },
      ],
      "cpu",
    );
    expect(series.agents).toEqual(["a", "b"]);
    expect(series.data).toEqual([
      { t: t0, a: 10, b: 0 },
      { t: t1, a: 20, b: 5 },
    ]);
  });
});
