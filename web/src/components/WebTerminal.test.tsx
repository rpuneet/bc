/**
 * WebTerminal.test.tsx — verifies the onConnectionStateChange contract
 * plus the AttachTab overlay variants (connecting / stopped / error).
 *
 * jsdom does not have a WebSocket implementation, so we install a
 * controllable mock that lets each test drive the socket lifecycle
 * (open / close / error) by hand.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, act, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import {
  WebTerminal,
  type TerminalConnectionState,
  type TerminalConnectionDetail,
} from "./WebTerminal";

/* ────────────────────────────────────────────────────────────────────
 * MockWebSocket — a minimal WebSocket that records instances in a
 * module-level registry so tests can reach in and trigger lifecycle
 * events.
 * ──────────────────────────────────────────────────────────────────── */

interface MockSocket {
  url: string;
  readyState: number;
  binaryType: string;
  onopen: ((ev?: Event) => void) | null;
  onmessage: ((ev: MessageEvent) => void) | null;
  onclose: ((ev: CloseEvent) => void) | null;
  onerror: ((ev?: Event) => void) | null;
  send: ReturnType<typeof vi.fn>;
  close: ReturnType<typeof vi.fn>;
}

const mockSockets: MockSocket[] = [];

class MockWebSocket implements MockSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  url: string;
  readyState = MockWebSocket.CONNECTING;
  binaryType = "blob";
  onopen: ((ev?: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onerror: ((ev?: Event) => void) | null = null;
  send = vi.fn();
  close = vi.fn(() => {
    this.readyState = MockWebSocket.CLOSED;
  });

  constructor(url: string) {
    this.url = url;
    mockSockets.push(this);
  }
}

// Helpers the tests use to drive the latest mock socket.
function latestSocket(): MockSocket {
  const s = mockSockets[mockSockets.length - 1];
  if (!s) throw new Error("No WebSocket has been constructed yet");
  return s;
}
function openLatest() {
  const s = latestSocket();
  s.readyState = MockWebSocket.OPEN;
  s.onopen?.();
}
function closeLatest(code = 1000, reason = "") {
  const s = latestSocket();
  s.readyState = MockWebSocket.CLOSED;
  s.onclose?.({ code, reason, wasClean: code === 1000 } as CloseEvent);
}
function errorLatest() {
  const s = latestSocket();
  s.onerror?.(new Event("error"));
}

beforeEach(() => {
  mockSockets.length = 0;
  (globalThis as unknown as { WebSocket: typeof MockWebSocket }).WebSocket =
    MockWebSocket;
});

afterEach(() => {
  // Leave the mock installed — all tests in this file expect it.
});

/* ────────────────────────────────────────────────────────────────────
 * WebTerminal: onConnectionStateChange callback contract.
 * ──────────────────────────────────────────────────────────────────── */

describe("WebTerminal — connection state callback", () => {
  it("reports connecting → open as the socket opens", () => {
    const states: Array<[TerminalConnectionState, TerminalConnectionDetail?]> = [];
    const onState = vi.fn((s: TerminalConnectionState, d?: TerminalConnectionDetail) => {
      states.push([s, d]);
    });

    render(<WebTerminal agentName="alice" onConnectionStateChange={onState} />);

    // The component fires "connecting" synchronously as it creates the WS.
    expect(states[0]?.[0]).toBe("connecting");

    act(() => openLatest());
    expect(states[states.length - 1]?.[0]).toBe("open");
  });

  it("reports error + close detail on socket error", () => {
    const states: Array<[TerminalConnectionState, TerminalConnectionDetail?]> = [];
    const onState = vi.fn((s: TerminalConnectionState, d?: TerminalConnectionDetail) => {
      states.push([s, d]);
    });

    render(<WebTerminal agentName="alice" onConnectionStateChange={onState} />);
    act(() => {
      errorLatest();
      closeLatest(1006, "abnormal");
    });

    const last = states[states.length - 1];
    expect(last?.[0]).toBe("error");
    expect(last?.[1]?.code).toBe(1006);
    expect(last?.[1]?.reason).toBe("abnormal");
  });

  it("reports closed (not error) when server closes cleanly", () => {
    const states: Array<[TerminalConnectionState, TerminalConnectionDetail?]> = [];
    render(
      <WebTerminal
        agentName="alice"
        onConnectionStateChange={(s, d) => states.push([s, d])}
      />,
    );
    act(() => openLatest());
    act(() => closeLatest(1000, ""));
    const last = states[states.length - 1];
    expect(last?.[0]).toBe("closed");
    expect(last?.[1]?.code).toBe(1000);
  });

  it("opens a fresh socket when reconnectToken changes", () => {
    const { rerender } = render(
      <WebTerminal agentName="alice" reconnectToken={0} />,
    );
    expect(mockSockets.length).toBe(1);
    rerender(<WebTerminal agentName="alice" reconnectToken={1} />);
    expect(mockSockets.length).toBe(2);
    // The first socket should have been closed cleanly.
    expect(mockSockets[0]?.close).toHaveBeenCalled();
  });
});

/* ────────────────────────────────────────────────────────────────────
 * AttachTab overlay variants.
 * We import AttachTab indirectly by rendering the exported
 * AgentDetail's Attach content via the same overlay component logic.
 * Since AttachTab is not exported from AgentDetail, the fastest focused
 * test is to verify the overlay component itself via AttachTab's
 * contract: render the AgentDetail Attach route and assert the overlay
 * renders the correct state for each agent.state + WS state.
 * ──────────────────────────────────────────────────────────────────── */

// Minimal fetch stub for the AgentDetail loader.
const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

function makeAgent(state: string) {
  return {
    name: "alice",
    role: "engineer",
    tool: "claude",
    state,
    total_cost_usd: 0,
    started_at: "2025-01-01T00:00:00Z",
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-01-01T00:00:00Z",
    stopped_at: state === "stopped" ? "2025-01-01T00:00:00Z" : undefined,
    task: "",
    session: "mycel-alice",
    mcp_servers: [],
  };
}

async function renderAttachAt(agentState: string) {
  fetchMock.mockImplementation((url: string) => {
    if (typeof url === "string" && url.includes("/api/agents/alice")) {
      return jsonResponse(makeAgent(agentState));
    }
    return jsonResponse({});
  });
  const { AgentDetail } = await import("../views/AgentDetail");
  const utils = render(
    <MemoryRouter initialEntries={["/agents/alice/attach"]}>
      <Routes>
        <Route path="/agents/:name" element={<AgentDetail />} />
        <Route path="/agents/:name/*" element={<AgentDetail />} />
      </Routes>
    </MemoryRouter>,
  );
  // Flush the initial fetch + state update.
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  return utils;
}

describe("AttachTab overlay — state variants", () => {
  beforeEach(() => {
    fetchMock.mockReset();
  });

  it("shows the 'stopped' overlay when the agent state is stopped", async () => {
    const { getByTestId, getByText } = await renderAttachAt("stopped");
    const overlay = getByTestId("attach-overlay");
    expect(overlay.getAttribute("data-state")).toBe("stopped");
    expect(getByText(/agent is stopped/i)).toBeInTheDocument();
    expect(getByText(/start agent/i)).toBeInTheDocument();
    // The WS should NOT be constructed when the agent is stopped.
    expect(mockSockets.length).toBe(0);
  });

  it("shows the 'connecting' overlay before the WS opens", async () => {
    const { getByTestId } = await renderAttachAt("running");
    // Running agent mounts the terminal, which constructs a WS.
    expect(mockSockets.length).toBe(1);
    const overlay = getByTestId("attach-overlay");
    expect(overlay.getAttribute("data-state")).toBe("connecting");
    expect(overlay.textContent).toMatch(/connecting to alice/i);
  });

  it("hides the overlay once the socket opens", async () => {
    const { queryByTestId } = await renderAttachAt("running");
    await act(async () => {
      openLatest();
    });
    expect(queryByTestId("attach-overlay")).toBeNull();
  });

  it("shows the 'error' overlay with a Retry button when the WS drops", async () => {
    const { getByTestId, getByRole } = await renderAttachAt("running");
    await act(async () => {
      openLatest();
      errorLatest();
      closeLatest(1006, "lost");
    });
    const overlay = getByTestId("attach-overlay");
    expect(overlay.getAttribute("data-state")).toBe("error");
    expect(overlay.textContent).toMatch(/connection lost/i);
    // The reason + code should appear in the muted text.
    expect(overlay.textContent).toMatch(/1006/);
    // Retry button reopens the socket.
    const retry = getByRole("button", { name: /retry/i });
    const before = mockSockets.length;
    await act(async () => {
      fireEvent.click(retry);
    });
    expect(mockSockets.length).toBe(before + 1);
  });
});
