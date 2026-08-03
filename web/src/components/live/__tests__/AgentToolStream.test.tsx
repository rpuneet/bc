/**
 * The Live tab's empty state has to say one of two very different things: "no
 * activity yet, events are coming" or "this provider cannot be captured, go use
 * Attach". It used to choose from a provider list hardcoded in this component,
 * which drifted: cursor was listed as uncapturable long after mycel started
 * ingesting its hooks, so cursor users were sent to the terminal for no reason.
 *
 * The verdict now comes from each provider's own declared activity_mode, served
 * by GET /api/providers. These tests pin that wiring, including the two ways it
 * could go wrong quietly — a wrong verdict while the answer is still loading,
 * and a wrong verdict when the lookup fails.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
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
function mockApi(providers: unknown, opts: { providersFail?: boolean } = {}) {
  fetchMock.mockImplementation((input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes("/api/providers")) {
      if (opts.providersFail) return Promise.reject(new Error("offline"));
      return jsonResponse(providers);
    }
    // No agents and no recorded activity: the component renders its empty state,
    // which is exactly what these tests are about.
    if (url.includes("/activity")) return jsonResponse([]);
    if (url.includes("/api/agents")) return jsonResponse([]);
    return jsonResponse([]);
  });
}

const WAITING = /No activity yet/i;
const UNAVAILABLE = /Live capture isn.t available/i;

function renderStream(tool?: string) {
  return render(
    <AgentToolStream agentName="eng-01" agentState="idle" agentTool={tool} />,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
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
