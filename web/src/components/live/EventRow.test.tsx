/**
 * EventRow.test.tsx — unit tests for the shared hook-event row used by
 * the Live page agent cards and the agent-detail Activity tab (#3267).
 *
 * Invariants:
 *   - eventGlyphKind maps tool names to monochrome glyph kinds
 *     (lifecycle beats the Task* prefix; MCP tools detected by name).
 *   - compactPath keeps the basename intact and shortens long dirs.
 *   - flattenNodes lifts subagent children into the flat stream.
 *   - EventRow renders name + summary without emoji; failed rows lead
 *     with the error text.
 */

import { describe, it, expect } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { EventRow, compactPath, eventGlyphKind } from "./EventRow";
import { RunningSection } from "./LiveRenderers";
import { activityItemToNode, flattenNodes, partitionRunning } from "./liveHelpers";
import type { ToolNode } from "./liveTypes";

function node(overrides: Partial<ToolNode>): ToolNode {
  return {
    id: "n-1",
    toolName: "Bash",
    args: "",
    fullInput: null,
    fullOutput: null,
    status: "completed",
    startTime: Date.now() - 5000,
    endTime: Date.now() - 4000,
    children: [],
    ...overrides,
  };
}

describe("eventGlyphKind", () => {
  it("maps tool names to glyph kinds", () => {
    expect(eventGlyphKind("Bash")).toBe("terminal");
    expect(eventGlyphKind("Edit")).toBe("edit");
    expect(eventGlyphKind("Write")).toBe("edit");
    expect(eventGlyphKind("Read")).toBe("read");
    expect(eventGlyphKind("Grep")).toBe("search");
    expect(eventGlyphKind("WebFetch")).toBe("web");
    expect(eventGlyphKind("Agent")).toBe("agent");
    expect(eventGlyphKind("Agent: sub-1")).toBe("agent");
    expect(eventGlyphKind("mcp__github__create_pr")).toBe("mcp");
    expect(eventGlyphKind("SomethingElse")).toBe("tool");
  });

  it("classifies lifecycle events before the Task* prefix", () => {
    expect(eventGlyphKind("TaskCompleted")).toBe("lifecycle");
    expect(eventGlyphKind("TaskCreate")).toBe("task");
    expect(eventGlyphKind("SessionStart")).toBe("lifecycle");
    expect(eventGlyphKind("Stop")).toBe("lifecycle");
    expect(eventGlyphKind("UserPromptSubmit")).toBe("prompt");
    expect(eventGlyphKind("PermissionRequest")).toBe("permission");
  });
});

describe("compactPath", () => {
  it("splits dir and basename", () => {
    expect(compactPath("src/views/Live.tsx")).toEqual({ dir: "src/views/", base: "Live.tsx" });
  });

  it("compacts long directories to the last two segments", () => {
    const { dir, base } = compactPath("/Users/someone/Projects/mycel/web/src/components/live/EventRow.tsx");
    expect(base).toBe("EventRow.tsx");
    expect(dir).toBe("…/components/live/");
  });

  it("passes through non-paths", () => {
    expect(compactPath("README")).toEqual({ dir: "", base: "README" });
  });
});

describe("flattenNodes", () => {
  it("lifts subagent children into the flat stream", () => {
    const child = node({ id: "c-1", toolName: "Read", args: "/tmp/a.txt" });
    const parent = node({ id: "p-1", toolName: "Agent", children: [child] });
    const flat = flattenNodes([parent]);
    expect(flat.map((n) => n.id)).toEqual(["p-1", "c-1"]);
  });
});

describe("EventRow", () => {
  it("renders file paths with the basename emphasized and no emoji", () => {
    const { container } = render(
      <EventRow node={node({ toolName: "Read", args: "/Users/x/Projects/mycel/web/src/views/Live.tsx" })} />,
    );
    expect(screen.getByText("Live.tsx")).toBeTruthy();
    // Monochrome glyphs only — no emoji anywhere in the row.
    expect(container.textContent).not.toMatch(/[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}]/u);
  });

  it("leads with the error text on failed events", () => {
    render(
      <EventRow node={node({ toolName: "Bash", status: "failed", error: "command not found: foo", args: "run build" })} />,
    );
    expect(screen.getByText("command not found: foo")).toBeTruthy();
  });

  it("uses human phrasing for lifecycle events", () => {
    render(<EventRow node={node({ toolName: "UserPromptSubmit", args: "fix the login bug" })} />);
    expect(screen.getByText("Prompt")).toBeTruthy();
    expect(screen.getByText("fix the login bug")).toBeTruthy();
  });

  it("shows the MCP server chip and function name", () => {
    render(<EventRow node={node({ toolName: "mcp__github__create_pr", args: "open PR" })} />);
    expect(screen.getByText("github")).toBeTruthy();
    expect(screen.getByText("create_pr")).toBeTruthy();
  });
});

/* ── Fix 1: pinned running section ─────────────────────────────────── */

describe("partitionRunning", () => {
  it("splits running rows from the rest, preserving order", () => {
    const nodes = [
      node({ id: "a", status: "completed" }),
      node({ id: "b", status: "running", endTime: undefined }),
      node({ id: "c", status: "failed" }),
      node({ id: "d", status: "running", endTime: undefined }),
    ];
    const { running, rest } = partitionRunning(nodes);
    expect(running.map((n) => n.id)).toEqual(["b", "d"]);
    expect(rest.map((n) => n.id)).toEqual(["a", "c"]);
  });

  it("moves a row out of the running bucket once it completes", () => {
    const running = node({ id: "x", toolName: "Bash", status: "running", endTime: undefined });
    expect(partitionRunning([running]).running.map((n) => n.id)).toEqual(["x"]);
    // Same node id, now completed — it leaves the pinned bucket.
    const done = { ...running, status: "completed" as const, endTime: Date.now() };
    const after = partitionRunning([done]);
    expect(after.running).toEqual([]);
    expect(after.rest.map((n) => n.id)).toEqual(["x"]);
  });
});

describe("RunningSection", () => {
  it("renders the pinned Running header with a count when rows are running", () => {
    render(
      <RunningSection
        nodes={[node({ id: "r1", toolName: "Bash", status: "running", endTime: undefined })]}
      />,
    );
    expect(screen.getByText("Running")).toBeTruthy();
    expect(screen.getByText("1")).toBeTruthy();
  });

  it("renders nothing when there are no running rows", () => {
    const { container } = render(<RunningSection nodes={[]} />);
    expect(container.firstChild).toBeNull();
  });
});

/* ── Fix 2: historical rows expand with their available details ────── */

describe("activityItemToNode + historical expansion", () => {
  it("carries tool_input from the persisted event data", () => {
    const n = activityItemToNode({
      timestamp: "2026-07-30T10:00:00.000Z",
      event: "PreToolUse",
      message: "Bash: git status",
      data: { tool_name: "Bash", tool_input: { command: "git status", description: "Show status" } },
    });
    expect(n.toolName).toBe("Bash");
    // The description is the richer title (#3423) — the raw command still
    // rides along in fullInput for the expanded detail view.
    expect(n.args).toBe("Show status");
    expect(n.fullInput).toEqual({ command: "git status", description: "Show status" });
    expect(n.status).toBe("completed");
    // Duration is genuinely unknown for historical rows.
    expect(n.endTime).toBeUndefined();
  });

  it("falls back to the raw command when a Bash tool_input has no description", () => {
    const n = activityItemToNode({
      timestamp: "2026-07-30T10:00:00.000Z",
      event: "PreToolUse",
      message: "Bash: git status",
      data: { tool_name: "Bash", tool_input: { command: "git status" } },
    });
    expect(n.args).toBe("git status");
  });

  it("marks a historical row failed when the event recorded an error", () => {
    const n = activityItemToNode({
      timestamp: "2026-07-30T10:00:00.000Z",
      event: "PostToolUseFailure",
      message: "Bash",
      data: { tool_name: "Bash", tool_input: { command: "false" }, error: "exit status 1" },
    });
    expect(n.status).toBe("failed");
    expect(n.error).toBe("exit status 1");
  });

  it("degrades gracefully when only a message is stored (no data)", () => {
    const n = activityItemToNode({
      timestamp: "2026-07-30T10:00:00.000Z",
      event: "Stop",
      message: "Turn complete",
    });
    expect(n.fullInput).toBeNull();
    expect(n.status).toBe("completed");
  });

  it("expands a historical row to show its input, a copy control, and no Output section", () => {
    const n = activityItemToNode({
      timestamp: "2026-07-30T10:00:00.000Z",
      event: "PreToolUse",
      message: "Bash: git status --short",
      data: { tool_name: "Bash", tool_input: { command: "git status --short", description: "Show status" } },
    });
    render(<EventRow node={n} />);
    // Chevron affordance present (has details) — expand it.
    fireEvent.click(screen.getByRole("button", { name: /Expand Bash event/ }));
    expect(screen.getByText("git status --short")).toBeTruthy();
    expect(screen.getByText("Input")).toBeTruthy();
    // Copy control present inside the expanded row.
    expect(screen.getAllByRole("button", { name: "Copy to clipboard" }).length).toBeGreaterThan(0);
    // This row's persisted event data has no tool_response → no Output section.
    expect(screen.queryByText("Output")).toBeNull();
  });

  it("carries tool_response from the persisted event data and renders an Output section", () => {
    const n = activityItemToNode({
      timestamp: "2026-07-30T10:00:00.000Z",
      event: "PostToolUse",
      message: "Bash: echo hi",
      data: {
        tool_name: "Bash",
        tool_input: { command: "echo hi" },
        tool_response: { stdout: "hi\n", stderr: "" },
      },
    });
    expect(n.fullOutput).toEqual({ stdout: "hi\n", stderr: "" });

    render(<EventRow node={n} />);
    fireEvent.click(screen.getByRole("button", { name: /Expand Bash event/ }));
    expect(screen.getByText("Input")).toBeTruthy();
    expect(screen.getByText("Output")).toBeTruthy();
    expect(screen.getByText("hi")).toBeTruthy();
  });
});
