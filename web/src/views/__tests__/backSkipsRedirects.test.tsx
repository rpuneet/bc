/**
 * Every alias/redirect route (/live, /stats, /metrics, /costs, /notifications,
 * /settings/tools, /providers, /secrets, /setup, ...) must resolve with
 * `<Navigate replace>` so it never becomes a history entry the user can land
 * back on. This suite proves that end-to-end with a real navigation stack:
 * push a page, follow a link to an alias route, let it redirect, then press
 * Back — the user must land on the page *before* the alias, in one step,
 * never on the dead alias route itself (the "phantom page" bug).
 *
 * If a redirect ever regresses to a plain `navigate(x)` (push) or a
 * `<Navigate>` without `replace`, these tests fail because Back would need
 * two presses, or would land back on the alias route.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import { AppRoutes } from "../../App";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

let lastLocation = "";
function LocationSpy() {
  const loc = useLocation();
  lastLocation = loc.pathname + loc.hash;
  return null;
}

/** Stand-in for the browser's Back button, exercised through react-router's
 *  own history stack (equivalent to a real Back press for a MemoryRouter). */
function BackTrigger() {
  const navigate = useNavigate();
  return (
    <button type="button" onClick={() => navigate(-1)}>
      test-back
    </button>
  );
}

function renderStack(entries: string[], initialIndex: number) {
  return render(
    <MemoryRouter initialEntries={entries} initialIndex={initialIndex}>
      <LocationSpy />
      <BackTrigger />
      <AppRoutes />
    </MemoryRouter>,
  );
}

async function pressBackOnceAndExpect(path: string) {
  fireEvent.click(screen.getByText("test-back"));
  await waitFor(() => {
    expect(lastLocation).toBe(path);
  });
}

beforeEach(() => {
  fetchMock.mockReset();
  fetchMock.mockReturnValue(jsonResponse([]));
  lastLocation = "";
});

describe("Back skips redirect/alias routes", () => {
  it("/stats -> /insights: Back from /insights lands on the page before /stats, not on /stats", async () => {
    renderStack(["/home", "/stats"], 1);
    await waitFor(() => expect(lastLocation).toBe("/insights"));

    await pressBackOnceAndExpect("/home");
  });

  it("/metrics -> /insights: Back lands on the prior page in one step", async () => {
    renderStack(["/home", "/metrics"], 1);
    await waitFor(() => expect(lastLocation).toBe("/insights"));

    await pressBackOnceAndExpect("/home");
  });

  it("/costs -> /insights: Back lands on the prior page in one step", async () => {
    renderStack(["/home", "/costs"], 1);
    await waitFor(() => expect(lastLocation).toBe("/insights"));

    await pressBackOnceAndExpect("/home");
  });

  it("/live -> /: Back lands on the page before /live, not on the dead /live entry", async () => {
    renderStack(["/agents", "/live"], 1);
    await waitFor(() => expect(lastLocation).toBe("/"));

    await pressBackOnceAndExpect("/agents");
  });

  it("/notifications -> /apps: Back lands on the prior page in one step", async () => {
    renderStack(["/home", "/notifications"], 1);
    await waitFor(() => expect(lastLocation).toBe("/apps"));

    await pressBackOnceAndExpect("/home");
  });

  it("/settings/tools -> /settings: Back lands on the prior page in one step", async () => {
    renderStack(["/home", "/settings/tools"], 1);
    await waitFor(() => expect(lastLocation).toBe("/settings"));

    await pressBackOnceAndExpect("/home");
  });

  it("/providers -> /settings: Back lands on the prior page in one step", async () => {
    renderStack(["/home", "/providers"], 1);
    await waitFor(() => expect(lastLocation).toBe("/settings"));

    await pressBackOnceAndExpect("/home");
  });

  it("/secrets -> /apps#custom-keys: Back lands on the prior page in one step", async () => {
    renderStack(["/home", "/secrets"], 1);
    await waitFor(() => expect(lastLocation).toBe("/apps#custom-keys"));

    await pressBackOnceAndExpect("/home");
  });

  it("/setup -> /readiness: Back lands on the prior page in one step", async () => {
    renderStack(["/home", "/setup"], 1);
    await waitFor(() => expect(lastLocation).toBe("/readiness"));

    await pressBackOnceAndExpect("/home");
  });
});
