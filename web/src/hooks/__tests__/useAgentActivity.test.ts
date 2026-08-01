/**
 * useAgentActivity.test.ts — coverage for the code-review fixes to the
 * live activity stream (#2674):
 *
 *   1. Pause actually buffers events (and counts them) instead of being
 *      decorative, and resume flushes the buffer in order.
 *   2. Event kinds that used to fall through the event→node switch with no
 *      rendered row (e.g. Notification) now produce a node.
 *   3. ElicitationResult finalizes the "waiting for permission" running
 *      node instead of leaving it pinned forever.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useAgentActivity } from "../useAgentActivity";
import { FLUSH_INTERVAL } from "../../components/live/liveTypes";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

/** Captures every EventSource the hook constructs so tests can push
 *  messages onto it directly, mirroring the real SSE stream. */
class TestEventSource {
  onopen: (() => void) | null = null;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(_url: string) {
    instances.push(this);
  }
  close() {
    /* no-op */
  }
}
let instances: TestEventSource[] = [];

function emit(type: string, data: Record<string, unknown>) {
  const es = instances[instances.length - 1];
  es?.onmessage?.({ data: JSON.stringify({ type, data }) } as MessageEvent);
}

beforeEach(() => {
  instances = [];
  (globalThis as unknown as { EventSource: typeof EventSource }).EventSource =
    TestEventSource as unknown as typeof EventSource;
  fetchMock.mockReset();
  fetchMock.mockImplementation(() => jsonResponse([]));
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

async function tick(ms = FLUSH_INTERVAL) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

describe("useAgentActivity — pause/resume", () => {
  it("buffers events and counts them while paused, then flushes on resume", async () => {
    const { result, rerender } = renderHook(
      ({ paused }: { paused: boolean }) => useAgentActivity(undefined, { paused }),
      { initialProps: { paused: false } },
    );
    await tick(0);

    // Pause the stream.
    rerender({ paused: true });
    await tick(0);

    act(() => {
      emit("agent.hook", { agent: "alice", event: "PreToolUse", tool_name: "Bash", command: "echo hi" });
    });
    await tick();

    // Buffered, not applied: no activity entry yet, but pausedCount ticked up.
    expect(result.current.activities.get("alice")).toBeUndefined();
    expect(result.current.pausedCount).toBeGreaterThan(0);

    // Resume flushes the buffer through the normal pipeline.
    rerender({ paused: false });
    await tick();

    const alice = result.current.activities.get("alice");
    expect(alice).toBeDefined();
    expect(alice!.nodes.some((n) => n.toolName === "Bash")).toBe(true);
    expect(result.current.pausedCount).toBe(0);
  });

  it("resumes and flushes buffered state_changed events too", async () => {
    const { result, rerender } = renderHook(
      ({ paused }: { paused: boolean }) => useAgentActivity(undefined, { paused }),
      { initialProps: { paused: false } },
    );
    await tick(0);

    // Seed an existing agent so the state_changed update has something to merge into.
    act(() => {
      emit("agent.hook", { agent: "bob", event: "UserPromptSubmit", prompt: "do work" });
    });
    await tick();
    expect(result.current.activities.get("bob")).toBeDefined();

    rerender({ paused: true });
    await tick(0);

    act(() => {
      emit("agent.state_changed", { name: "bob", state: "stuck", task: "waiting" });
    });

    expect(result.current.activities.get("bob")!.state).not.toBe("stuck");
    expect(result.current.pausedCount).toBeGreaterThan(0);

    rerender({ paused: false });
    await tick();

    expect(result.current.activities.get("bob")!.state).toBe("stuck");
    expect(result.current.pausedCount).toBe(0);
  });
});

describe("useAgentActivity — full event coverage", () => {
  it("renders a node for event kinds that previously had no switch case", async () => {
    const { result } = renderHook(() => useAgentActivity());
    await tick(0);

    act(() => {
      emit("agent.hook", { agent: "carol", event: "Notification", message: "heads up" });
    });
    await tick();

    const carol = result.current.activities.get("carol");
    expect(carol).toBeDefined();
    expect(carol!.nodes.some((n) => n.toolName === "Notification" && n.args.includes("heads up"))).toBe(true);
  });

  it("renders a compact row for a totally unhandled-but-known event via the default case", async () => {
    const { result } = renderHook(() => useAgentActivity());
    await tick(0);

    act(() => {
      emit("agent.hook", { agent: "dave", event: "TeammateIdle" });
    });
    await tick();

    const dave = result.current.activities.get("dave");
    expect(dave).toBeDefined();
    expect(dave!.nodes.some((n) => n.toolName === "TeammateIdle")).toBe(true);
  });
});

describe("useAgentActivity — ElicitationResult clears the running node", () => {
  it("finalizes a pending Elicitation node when its result arrives", async () => {
    const { result } = renderHook(() => useAgentActivity());
    await tick(0);

    act(() => {
      emit("agent.hook", { agent: "erin", event: "Elicitation", tool_name: "AskUser" });
    });
    await tick();

    let erin = result.current.activities.get("erin")!;
    let node = erin.nodes.find((n) => n.toolName === "Elicitation");
    expect(node?.status).toBe("running");

    act(() => {
      emit("agent.hook", { agent: "erin", event: "ElicitationResult", message: "approved" });
    });
    await tick();

    erin = result.current.activities.get("erin")!;
    node = erin.nodes.find((n) => n.toolName === "Elicitation");
    expect(node?.status).toBe("completed");
  });
});
