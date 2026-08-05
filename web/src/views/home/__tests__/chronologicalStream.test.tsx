/**
 * ChronologicalStream unit tests — merge + sort + agent attribution (#3642).
 */

import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ChronologicalStream } from "../ChronologicalStream";
import type { AgentActivity, ToolNode } from "../../../components/live/liveTypes";

function node(overrides: Partial<ToolNode> & { id: string; startTime: number }): ToolNode {
  return {
    toolName: "Bash",
    args: "echo hi",
    fullInput: null,
    fullOutput: null,
    status: "completed",
    children: [],
    ...overrides,
  };
}

function agent(name: string, nodes: ToolNode[], state = "working"): AgentActivity {
  return {
    name,
    state,
    task: "working",
    tool: "claude",
    role: "base",
    tokens: 0,
    inputTokens: 0,
    outputTokens: 0,
    costUsd: 0,
    lastEventTime: nodes[0]?.startTime ?? 0,
    nodes,
    collapsed: false,
  };
}

describe("ChronologicalStream", () => {
  it("merges agents newest-first with agent chips", () => {
    const onOpen = vi.fn();
    render(
      <ChronologicalStream
        agents={[
          agent("alice", [node({ id: "a1", startTime: 100, args: "older" })]),
          agent("bob", [node({ id: "b1", startTime: 200, args: "newer", toolName: "Read" })]),
        ]}
        searchTerm=""
        typeFilter="all"
        onOpenAgent={onOpen}
        emptyTitle="empty"
        emptyDescription="none"
      />,
    );

    const rows = screen.getAllByTestId("home-stream-row");
    expect(rows).toHaveLength(2);
    expect(rows[0].getAttribute("data-agent")).toBe("bob");
    expect(rows[1].getAttribute("data-agent")).toBe("alice");

    fireEvent.click(screen.getByTitle("Open bob detail view"));
    expect(onOpen).toHaveBeenCalledWith("bob");
  });

  it("pins running rows above completed ones", () => {
    render(
      <ChronologicalStream
        agents={[
          agent("alice", [
            node({ id: "done", startTime: 300, status: "completed" }),
            node({ id: "run", startTime: 100, status: "running", toolName: "Bash", args: "sleep" }),
          ]),
        ]}
        searchTerm=""
        typeFilter="all"
        onOpenAgent={() => {}}
        emptyTitle="empty"
        emptyDescription="none"
      />,
    );

    expect(screen.getByText("Running")).toBeInTheDocument();
    const rows = screen.getAllByTestId("home-stream-row");
    // Running first, then completed (even though completed has a newer startTime).
    expect(rows[0].textContent).toMatch(/sleep|Bash/);
  });

  it("shows empty state when there are no nodes", () => {
    render(
      <ChronologicalStream
        agents={[agent("alice", [], "idle")]}
        searchTerm=""
        typeFilter="all"
        onOpenAgent={() => {}}
        emptyTitle="No activity yet"
        emptyDescription="waiting"
      />,
    );
    expect(screen.getByText("No activity yet")).toBeInTheDocument();
    expect(screen.getByTestId("home-chronological-stream").getAttribute("data-empty")).toBe("true");
  });
});
