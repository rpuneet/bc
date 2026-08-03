/**
 * The Live tab's empty state has to distinguish four situations that look
 * identical on screen but need opposite responses from the reader: events are
 * coming, this provider can never be captured, the agent isn't running, and the
 * agent is running but has reported nothing for long enough that it is broken.
 *
 * The capture verdict comes from each provider's own declared activity_mode,
 * served by GET /api/providers. It used to come from a list hardcoded in the
 * component, which drifted: cursor was listed as uncapturable long after mycel
 * started ingesting its hooks, so cursor users were sent to the terminal for no
 * reason. These tests pin that wiring, including the two ways it could go wrong
 * quietly — a wrong verdict while the answer is still loading, and a wrong
 * verdict when the lookup fails — plus the silence case, which is what a dead
 * agent actually looks like.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { AgentToolStream } from "../AgentToolStream";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

/** Route the calls this component's hooks make; providers is the one under test. */
function mockApi(
  providers: unknown,
  opts: { providersFail?: boolean; agents?: unknown[] } = {},
) {
  fetchMock.mockImplementation((input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes("/api/providers")) {
      if (opts.providersFail) return Promise.reject(new Error("offline"));
      return jsonResponse(providers);
    }
    // No agents and no recorded activity: the component renders its empty state,
    // which is exactly what these tests are about.
    if (url.includes("/activity")) return jsonResponse([]);
    if (url.includes("/api/agents")) return jsonResponse(opts.agents ?? []);
    return jsonResponse([]);
  });
}

const WAITING = /Waiting for the first event/i;
const UNAVAILABLE = /can.t read activity from/i;
const SILENT = /Nothing reported since this agent started/i;
const NOT_RUNNING = /isn.t running/i;

function renderStream(
  tool?: string,
  props: { agentState?: string; startedAt?: string } = {},
) {
  return render(
    <MemoryRouter>
      <AgentToolStream
        agentName="eng-01"
        agentState={props.agentState ?? "idle"}
        agentTool={tool}
        startedAt={props.startedAt}
      />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("AgentToolStream capture verdict", () => {
  it("waits for events for a hooks-mode provider like cursor", async () => {
    mockApi([{ name: "cursor", activity_mode: "hooks" }]);
    renderStream("cursor");
    await waitFor(() => expect(screen.getByText(WAITING)).toBeTruthy());
    expect(screen.queryByText(UNAVAILABLE)).toBeNull();
  });

  it("waits for events for a transcript-mode provider like codex", async () => {
    mockApi([{ name: "codex", activity_mode: "transcript" }]);
    renderStream("codex");
    await waitFor(() => expect(screen.getByText(WAITING)).toBeTruthy());
  });

  it("says capture is unavailable only when the provider declares none", async () => {
    mockApi([{ name: "openclaw", activity_mode: "none" }]);
    renderStream("openclaw");
    await waitFor(() => expect(screen.getByText(UNAVAILABLE)).toBeTruthy());
    expect(screen.queryByText(WAITING)).toBeNull();
  });

  it("does not flash 'unavailable' before the provider list arrives", () => {
    // Never resolves: the verdict is unknown for the whole of this render.
    fetchMock.mockImplementation(() => new Promise(() => {}));
    renderStream("openclaw");
    // Claiming capture is impossible before asking would be a lie the user acts
    // on, so an unknown verdict must read as "waiting".
    expect(screen.queryByText(UNAVAILABLE)).toBeNull();
    expect(screen.getByText(WAITING)).toBeTruthy();
  });

  it("keeps waiting when the provider lookup fails", async () => {
    mockApi(null, { providersFail: true });
    renderStream("openclaw");
    // A network blip must not be reported to the user as a missing capability.
    await waitFor(() => expect(screen.getByText(WAITING)).toBeTruthy());
    expect(screen.queryByText(UNAVAILABLE)).toBeNull();
  });

  it("gives an unrecognized tool the benefit of the doubt", async () => {
    mockApi([{ name: "claude", activity_mode: "hooks" }]);
    // A custom claude-compatible wrapper is not in the registry but is still
    // captured, so it must not be declared uncapturable.
    renderStream("some-custom-wrapper");
    await waitFor(() => expect(screen.getByText(WAITING)).toBeTruthy());
    expect(screen.queryByText(UNAVAILABLE)).toBeNull();
  });

  it("treats a provider that omits activity_mode as uncapturable", async () => {
    // An older daemon that predates the field reports nothing; "none" is the
    // safe reading, since before this change no such provider was captured.
    mockApi([{ name: "legacy" }]);
    renderStream("legacy");
    await waitFor(() => expect(screen.getByText(UNAVAILABLE)).toBeTruthy());
  });
});

describe("AgentToolStream empty-feed diagnosis", () => {
  const HOURS_AGO = new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString();
  const JUST_NOW = new Date(Date.now() - 5 * 1000).toISOString();

  it("says a long-silent running agent is broken, not starting up", async () => {
    // The case reported as "live doesn't work": the agent runs, its CLI refuses
    // every turn, and nothing ever reaches the feed. Telling the reader to keep
    // waiting sends them to look for a mycel bug that isn't there.
    mockApi([{ name: "pi", activity_mode: "transcript" }]);
    renderStream("pi", { agentState: "idle", startedAt: HOURS_AGO });
    await waitFor(() => expect(screen.getByText(SILENT)).toBeTruthy());
    expect(screen.queryByText(WAITING)).toBeNull();
    // The reason only exists in the terminal, so the way there must be offered.
    expect(screen.getByRole("link")).toHaveAttribute(
      "href",
      "/agents/eng-01/attach",
    );
  });

  it("still says 'waiting' for an agent that only just started", async () => {
    mockApi([{ name: "cursor", activity_mode: "hooks" }]);
    renderStream("cursor", { agentState: "working", startedAt: JUST_NOW });
    await waitFor(() => expect(screen.getByText(WAITING)).toBeTruthy());
    expect(screen.queryByText(SILENT)).toBeNull();
  });

  it("does not diagnose silence before the capture verdict arrives", () => {
    // An uncapturable provider is silent by design. Calling that silence broken
    // before knowing which kind of provider this is would be a false alarm.
    fetchMock.mockImplementation(() => new Promise(() => {}));
    renderStream("openclaw", { agentState: "idle", startedAt: HOURS_AGO });
    expect(screen.queryByText(SILENT)).toBeNull();
    expect(screen.getByText(WAITING)).toBeTruthy();
  });

  it("tells a stopped agent's reader to start it rather than to wait", async () => {
    mockApi([{ name: "cursor", activity_mode: "hooks" }]);
    renderStream("cursor", { agentState: "stopped", startedAt: HOURS_AGO });
    await waitFor(() => expect(screen.getByText(NOT_RUNNING)).toBeTruthy());
    expect(screen.queryByText(SILENT)).toBeNull();
    expect(screen.queryByText(WAITING)).toBeNull();
  });

  it("does not mistake an errored agent for one that is still working", async () => {
    mockApi([{ name: "cursor", activity_mode: "hooks" }]);
    renderStream("cursor", { agentState: "error", startedAt: HOURS_AGO });
    await waitFor(() => expect(screen.getByText(NOT_RUNNING)).toBeTruthy());
  });

  it("explains the silence when only lifecycle events arrived", async () => {
    // The reported symptom, exactly: "I only see state changed events". The
    // agent is in the list so it has an activity entry, but no tool events ever
    // landed, and the stream described that as "No tool events yet for this
    // agent" — true, unhelpful, and indistinguishable from a slow start.
    mockApi([{ name: "pi", activity_mode: "transcript" }], {
      agents: [{ name: "eng-01", state: "working", tool: "pi" }],
    });
    renderStream("pi", { agentState: "working", startedAt: HOURS_AGO });
    await waitFor(() => expect(screen.getByText(SILENT)).toBeTruthy());
    expect(screen.queryByText(/No tool events yet/i)).toBeNull();
  });
});
