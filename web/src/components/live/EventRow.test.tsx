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
import { render, screen } from "@testing-library/react";
import { EventRow, compactPath, eventGlyphKind } from "./EventRow";
import { flattenNodes } from "./liveHelpers";
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
    const { dir, base } = compactPath("/Users/someone/Projects/bc/web/src/components/live/EventRow.tsx");
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
      <EventRow node={node({ toolName: "Read", args: "/Users/x/Projects/bc/web/src/views/Live.tsx" })} />,
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
