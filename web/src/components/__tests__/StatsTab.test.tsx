import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { StatsTab } from "../StatsTab";
import type { Agent } from "../../api/client";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

const baseAgent: Agent = {
  name: "stats-agent",
  role: "engineer",
  tool: "claude",
  state: "working",
  total_cost_usd: 0,
  started_at: "2026-08-01T00:00:00Z",
  created_at: "2026-08-01T00:00:00Z",
  updated_at: "2026-08-01T00:00:00Z",
};

const emptyComputed = {
  total_events: 0,
  tool_calls: 0,
  tool_breakdown: {},
  session_duration_sec: 0,
  last_active: "",
  input_tokens: 0,
  output_tokens: 0,
  tokens: 0,
  cost_usd: 0,
  disk_bytes: 0,
  channel_sent: 0,
  channel_received: 0,
};

function mockApi(opts: { costByAgent?: unknown[] } = {}) {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    if (u.includes("/api/costs/agents")) return jsonResponse(opts.costByAgent ?? []);
    if (u.includes("/api/costs/models")) return jsonResponse([]);
    if (u.includes("/api/costs/agent/")) return jsonResponse({ summary: null, daily: [] });
    if (u.includes("/api/agents/stats/summary/")) return jsonResponse(null);
    if (u.includes("/api/agents/stats/")) return jsonResponse([]);
    if (u.includes("/stats-computed")) return jsonResponse(emptyComputed);
    return jsonResponse([]);
  });
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("StatsTab", () => {
  it("never fabricates an 80/20 split — shows the combined total and a bare dash for the split", async () => {
    mockApi();
    const agent: Agent = { ...baseAgent, total_tokens: 100_000 };
    render(<StatsTab agent={agent} />);

    // Combined total (from agent.total_tokens, the only source available)
    // renders, but no invented in/out split does.
    await waitFor(() => expect(screen.getAllByText("100.0K").length).toBeGreaterThan(0));
    expect(screen.getAllByText("—").length).toBeGreaterThan(0);
    expect(screen.queryByText(/80\.0K in/)).not.toBeInTheDocument();
    expect(screen.queryByText(/20\.0K out/)).not.toBeInTheDocument();
  });

  it("surfaces the real in/out split from the fleet cost ledger when it exists", async () => {
    mockApi({
      costByAgent: [
        {
          agent_id: "stats-agent",
          total_cost_usd: 5,
          input_tokens: 30_000,
          output_tokens: 5_000,
          total_tokens: 35_000,
          record_count: 3,
        },
      ],
    });
    const agent: Agent = { ...baseAgent, total_tokens: 35_000 };
    render(<StatsTab agent={agent} />);

    await waitFor(() => expect(screen.getAllByText(/30\.0K in/).length).toBeGreaterThan(0));
    expect(screen.getAllByText(/5\.0K out/).length).toBeGreaterThan(0);
  });
});
