/**
 * ToolDetail.test.tsx — the rich per-tool renderers behind expanded
 * stream rows (owner directive: parse tool JSON into a real UI, the
 * error state's structure is the benchmark).
 *
 * Invariants:
 *   - parseBashOutput handles object payloads, JSON-string payloads and
 *     plain strings; noise flags surface ONLY when true.
 *   - Bash input renders `$ command` + muted description, not JSON.
 *   - Bash output shows stdout/stderr blocks only when non-empty.
 *   - Unknown/MCP tools fall back to a key/value table, nested objects
 *     collapse behind a disclosure.
 *   - The `{}` raw toggle restores today's JSON view.
 */

import { describe, it, expect } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { EventRow } from "./EventRow";
import {
  KeyValueView,
  RichToolInput,
  RichToolOutput,
  contentBlocksText,
  normalizePayload,
  parseBashOutput,
} from "./ToolDetail";
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

describe("parseBashOutput", () => {
  it("parses object payloads into stdout/stderr", () => {
    const p = parseBashOutput({ stdout: "hello\n", stderr: "", interrupted: false, isImage: false });
    expect(p).not.toBeNull();
    expect(p!.stdout).toBe("hello\n");
    expect(p!.stderr).toBe("");
    // False flags are noise — suppressed entirely.
    expect(p!.flags).toEqual([]);
  });

  it("surfaces flags only when true", () => {
    const p = parseBashOutput({ stdout: "", stderr: "boom", interrupted: true, noOutputExpected: false });
    expect(p!.flags).toEqual(["interrupted"]);
  });

  it("parses JSON-string payloads", () => {
    const p = parseBashOutput(JSON.stringify({ stdout: "from json", stderr: "" }));
    expect(p!.stdout).toBe("from json");
  });

  it("treats plain strings as stdout", () => {
    const p = parseBashOutput("plain text output");
    expect(p!.stdout).toBe("plain text output");
    expect(p!.flags).toEqual([]);
  });

  it("returns null for unrecognizable shapes (caller falls back)", () => {
    expect(parseBashOutput({ command: "ls", description: "list" })).toBeNull();
    expect(parseBashOutput(null)).toBeNull();
    expect(parseBashOutput(42)).toBeNull();
  });
});

describe("normalizePayload / contentBlocksText", () => {
  it("parses JSON strings and passes other strings through", () => {
    expect(normalizePayload('{"a":1}')).toEqual({ a: 1 });
    expect(normalizePayload("not json {")).toBe("not json {");
  });

  it("joins MCP text content blocks", () => {
    expect(contentBlocksText([{ type: "text", text: "a" }, { type: "text", text: "b" }])).toBe("a\nb");
    expect(contentBlocksText([{ type: "image", data: "x" }])).toBeNull();
    expect(contentBlocksText("nope")).toBeNull();
  });
});

describe("Bash renderers", () => {
  it("renders the command as a $ shell line with the description as caption", () => {
    render(
      <RichToolInput
        toolName="Bash"
        input={{ command: "git status --short", description: "Show working tree status" }}
      />,
    );
    expect(screen.getByText("git status --short")).toBeTruthy();
    expect(screen.getByText("Show working tree status")).toBeTruthy();
    expect(screen.getByText("$")).toBeTruthy();
    // No raw JSON braces from the input object.
    expect(screen.queryByText(/"command"/)).toBeNull();
  });

  it("shows stdout and suppresses false noise flags", () => {
    render(
      <RichToolOutput
        toolName="Bash"
        output={{ stdout: "3 files changed", stderr: "", interrupted: false, isImage: false }}
      />,
    );
    expect(screen.getByText("3 files changed")).toBeTruthy();
    expect(screen.queryByText("interrupted")).toBeNull();
    expect(screen.queryByText("isImage")).toBeNull();
    // Only stdout — no stderr block, no labels needed.
    expect(screen.queryByText("stderr")).toBeNull();
  });

  it("shows stderr as its own labeled block and true flags as chips", () => {
    render(
      <RichToolOutput
        toolName="Bash"
        output={{ stdout: "partial", stderr: "warning: dirty tree", interrupted: true }}
      />,
    );
    expect(screen.getByText("warning: dirty tree")).toBeTruthy();
    expect(screen.getByText("stderr")).toBeTruthy();
    expect(screen.getByText("interrupted")).toBeTruthy();
  });

  it("says 'no output' instead of an empty block", () => {
    render(<RichToolOutput toolName="Bash" output={{ stdout: "", stderr: "" }} />);
    expect(screen.getByText("no output")).toBeTruthy();
  });
});

describe("File tool renderers", () => {
  it("headlines the file path and shows the line range for Read", () => {
    render(
      <RichToolInput
        toolName="Read"
        input={{ file_path: "/Users/x/Projects/bc/web/src/views/Home.tsx", offset: 10, limit: 40 }}
      />,
    );
    expect(screen.getByText("Home.tsx")).toBeTruthy();
    expect(screen.getByText("10–49")).toBeTruthy();
    expect(screen.queryByText(/"file_path"/)).toBeNull();
  });

  it("shows replace/with rows for Edit", () => {
    render(
      <RichToolInput
        toolName="Edit"
        input={{ file_path: "/a/b/c.ts", old_string: "const a = 1", new_string: "const a = 2" }}
      />,
    );
    expect(screen.getByText("const a = 1")).toBeTruthy();
    expect(screen.getByText("const a = 2")).toBeTruthy();
    expect(screen.getByText("replace")).toBeTruthy();
  });

  it("shows the pattern row for Grep", () => {
    render(<RichToolInput toolName="Grep" input={{ pattern: "useAgentActivity", path: "/repo/web" }} />);
    expect(screen.getByText("useAgentActivity")).toBeTruthy();
    expect(screen.getByText("pattern")).toBeTruthy();
  });
});

describe("Unknown / MCP fallback", () => {
  it("renders a key/value table, never a JSON blob", () => {
    render(
      <RichToolInput toolName="mcp__github__create_pr" input={{ title: "Fix bug", draft: true }} />,
    );
    expect(screen.getByText("title")).toBeTruthy();
    expect(screen.getByText("Fix bug")).toBeTruthy();
    expect(screen.getByText("draft")).toBeTruthy();
    expect(screen.getByText("true")).toBeTruthy();
    expect(screen.queryByText(/[{}]/)).toBeNull();
  });

  it("collapses nested objects behind a disclosure", () => {
    render(<KeyValueView value={{ config: { deep: { x: 1 } }, name: "svc" }} />);
    const toggle = screen.getByRole("button", { name: /1 keys/ });
    expect(screen.queryByText(/"deep"/)).toBeNull();
    fireEvent.click(toggle);
    expect(screen.getByText(/"deep"/)).toBeTruthy();
  });

  it("renders MCP text content blocks as plain text", () => {
    render(<RichToolOutput toolName="mcp__bc__send_message" output={[{ type: "text", text: "delivered to #ops" }]} />);
    expect(screen.getByText("delivered to #ops")).toBeTruthy();
  });
});

describe("EventRow integration — raw toggle", () => {
  it("expanded Bash row shows the parsed view; {} toggle restores raw JSON", () => {
    render(
      <EventRow
        node={node({
          toolName: "Bash",
          args: "git log",
          fullInput: { command: "git log --oneline", description: "Recent commits" },
          fullOutput: { stdout: "abc123 fix", stderr: "", interrupted: false },
        })}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Expand Bash event/ }));

    // Parsed views by default.
    expect(screen.getByText("git log --oneline")).toBeTruthy();
    expect(screen.getByText("abc123 fix")).toBeTruthy();
    expect(screen.queryByText(/"command"/)).toBeNull();

    // Toggle the input section to raw JSON.
    fireEvent.click(screen.getByRole("button", { name: "Toggle raw JSON for Bash input" }));
    expect(screen.getByText(/"command"/)).toBeTruthy();

    // Copy buttons remain (one per section).
    expect(screen.getAllByRole("button", { name: "Copy to clipboard" }).length).toBe(2);
  });
});
