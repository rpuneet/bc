/**
 * Agents.peekItemToNode.test.tsx — unit tests for the Peek Activity Feed's
 * historical-row normalization (#3423).
 *
 * The Agents list row's "peek" preview used to build its row title from the
 * server's flat `message` string ("Bash: cd /repo && grep -rn foo ."),
 * which only ever carried the raw command. Bash tool calls carry a
 * human-written `tool_input.description` (e.g. "Check rebuild completion")
 * that was being ignored. `peekItemToNode` now delegates to the shared
 * `activityItemToNode` (the same normalization the Live/Raw stream uses)
 * so a row reads identically everywhere and prefers the description.
 */

import { describe, it, expect } from "vitest";
import { peekItemToNode } from "./Agents";
import type { AgentActivityItem } from "../api/client";

describe("peekItemToNode", () => {
  it("prefers tool_input.description as the row title for a Bash call", () => {
    const item: AgentActivityItem = {
      timestamp: "2026-08-01T10:00:00.000Z",
      event: "PreToolUse",
      message: "Bash: cd /repo && grep -rn foo .",
      data: {
        tool_name: "Bash",
        tool_input: {
          command: "cd /repo && grep -rn foo .",
          description: "Check rebuild completion",
        },
      },
    };

    const node = peekItemToNode(item, 0);

    expect(node.toolName).toBe("Bash");
    expect(node.args).toBe("Check rebuild completion");
    // The raw command is not lost — it stays available in the expanded detail.
    expect(node.fullInput).toEqual({
      command: "cd /repo && grep -rn foo .",
      description: "Check rebuild completion",
    });
  });

  it("falls back to the raw command when no description is present", () => {
    const item: AgentActivityItem = {
      timestamp: "2026-08-01T10:00:00.000Z",
      event: "PreToolUse",
      message: "Bash: ls -la",
      data: { tool_name: "Bash", tool_input: { command: "ls -la" } },
    };

    const node = peekItemToNode(item, 0);

    expect(node.toolName).toBe("Bash");
    expect(node.args).toBe("ls -la");
  });

  it("degrades gracefully to the message when there is no structured tool_input", () => {
    const item: AgentActivityItem = {
      timestamp: "2026-08-01T10:00:00.000Z",
      event: "Stop",
      message: "Turn complete",
    };

    const node = peekItemToNode(item, 0);

    // No data.tool_name is present, so the title is derived from the
    // message (same as the Live/Raw stream's historical hydration path),
    // and the message doubles as the row summary.
    expect(node.toolName).toBe("Turn");
    expect(node.args).toBe("Turn complete");
  });

  it("re-keys the id per index so peek rows stay stable within the feed", () => {
    const item: AgentActivityItem = {
      timestamp: "2026-08-01T10:00:00.000Z",
      event: "PreToolUse",
      data: { tool_name: "Read", tool_input: { file_path: "/repo/main.go" } },
    };

    const node = peekItemToNode(item, 3);
    expect(node.id).toBe(`peek-${item.timestamp}-3`);
  });
});
