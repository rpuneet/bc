/**
 * AgentTimeline.test.tsx — the persisted, scrollable lifecycle history tab
 * (#3423). Built from GET /api/agents/{name}/activity; unlike the Live tab
 * it is read-only history with no WebSocket subscription.
 *
 * Invariants:
 *   - Renders activity rows fetched from the endpoint via the shared
 *     EventRow/activityItemToNode path (same look as Live).
 *   - "Load older" pages backwards using before=<id> and appends rows.
 *   - An agent with no history shows the honest empty state.
 */

import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { AgentTimeline } from "../AgentTimeline";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

function activityItem(id: number, event: string, message: string) {
  return {
    id,
    timestamp: new Date(2026, 0, 1, 12, 0, id).toISOString(),
    event,
    message,
    data: { tool_name: event },
  };
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("AgentTimeline", () => {
  it("shows the empty state when there is no recorded activity", async () => {
    fetchMock.mockImplementationOnce(() => jsonResponse([]));

    render(<AgentTimeline agentName="eng-01" />);

    await waitFor(() => {
      expect(screen.getByText("No recorded activity yet")).toBeInTheDocument();
    });
  });

  it("renders activity rows fetched from the per-agent endpoint", async () => {
    const items = [
      activityItem(3, "Bash", "Bash: ls -la"),
      activityItem(2, "Read", "Read"),
      activityItem(1, "agent.spawned", "spawned"),
    ];
    fetchMock.mockImplementationOnce(() => jsonResponse(items));

    render(<AgentTimeline agentName="eng-01" />);

    await waitFor(() => {
      expect(screen.getByText("Bash")).toBeInTheDocument();
    });
    expect(screen.getAllByText("Read").length).toBeGreaterThan(0);

    const url = fetchMock.mock.calls[0]?.[0] as string;
    expect(url).toContain("/agents/eng-01/activity");
  });

  it("pages backwards through history with load older", async () => {
    const firstPage = Array.from({ length: 50 }, (_, i) =>
      activityItem(100 - i, "Bash", `cmd ${100 - i}`),
    );
    const secondPage = [activityItem(49, "Read", "older read")];

    fetchMock
      .mockImplementationOnce(() => jsonResponse(firstPage))
      .mockImplementationOnce(() => jsonResponse(secondPage));

    render(<AgentTimeline agentName="eng-01" />);

    await waitFor(() => {
      expect(screen.getByText("Load older")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Load older"));

    await waitFor(() => {
      expect(screen.getByText("older read")).toBeInTheDocument();
    });

    const secondUrl = fetchMock.mock.calls[1]?.[0] as string;
    expect(secondUrl).toContain("before=51");
  });
});
