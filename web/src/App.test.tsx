/**
 * App.test.tsx — HomeGate first-run flash fix (#2674).
 *
 * HomeGate used to render the full Home dashboard immediately and only
 * redirect once the onboarding probe resolved — a visible flash of the
 * empty dashboard on a fresh install. It should instead show a
 * blank/skeleton frame while the probe is in flight, then land on either
 * Home or Settings (first-run setup is a progressive reveal there now —
 * there's no separate /welcome wizard) once it resolves.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { HomeGate } from "./App";
import { __resetDefaultViewForTests } from "./utils/defaultView";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

function renderGate() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <Routes>
        <Route path="/" element={<HomeGate />} />
        <Route path="/settings" element={<div>Settings Page</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("HomeGate", () => {
  it("shows a blank/skeleton frame while the onboarding probe is in flight, not Home", async () => {
    let resolveOnboarding!: (v: Response) => void;
    fetchMock.mockImplementation((url: string) => {
      if (url.includes("/onboarding/state")) {
        return new Promise<Response>((resolve) => {
          resolveOnboarding = resolve;
        });
      }
      return jsonResponse([]);
    });

    renderGate();

    // While pending: no live-stream chrome from Home should be present.
    expect(screen.queryByTestId("live-state-badge")).not.toBeInTheDocument();
    expect(screen.getByText(/loading/i)).toBeInTheDocument();

    // Resolve as a returning (non-first-run) install: lands on Home.
    resolveOnboarding(jsonResponse({ firstRun: false }) as unknown as Response);
    await waitFor(() => {
      expect(screen.queryByText(/loading/i)).not.toBeInTheDocument();
    });
  });

  it("redirects to /settings on first run without ever rendering Home", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url.includes("/onboarding/state")) return jsonResponse({ firstRun: true });
      return jsonResponse([]);
    });

    renderGate();

    await waitFor(() => {
      expect(screen.getByText("Settings Page")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("live-state-badge")).not.toBeInTheDocument();
  });

  it("falls back to Home when the onboarding probe fails", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url.includes("/onboarding/state")) return Promise.reject(new Error("network down"));
      return jsonResponse([]);
    });

    renderGate();

    await waitFor(() => {
      expect(screen.queryByText(/loading/i)).not.toBeInTheDocument();
    });
    expect(screen.queryByText("Settings Page")).not.toBeInTheDocument();
  });
});

/**
 * "Default view" was saved, displayed, and never read: pick Agents or Insights
 * and you still landed on Home every time (#3474).
 */
describe("HomeGate default view", () => {
  beforeEach(() => {
    // The entry decision is per document, so it outlives a render tree.
    __resetDefaultViewForTests("/");
  });

  function renderIndex(defaultView: string, firstRun = false) {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.includes("/api/onboarding")) return jsonResponse({ firstRun });
      if (u.includes("/api/settings")) {
        return jsonResponse({ ui: { theme: "dark", mode: "auto", default_view: defaultView } });
      }
      return jsonResponse({});
    });
    return render(
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route path="/" element={<HomeGate honorDefaultView />} />
          <Route path="/agents" element={<div>Agents Page</div>} />
          <Route path="/insights" element={<div>Insights Page</div>} />
          <Route path="/settings" element={<div>Settings Page</div>} />
        </Routes>
      </MemoryRouter>,
    );
  }

  it("lands on Agents when that is the chosen default", async () => {
    renderIndex("agents");
    await waitFor(() => expect(screen.getByText("Agents Page")).toBeTruthy());
  });

  it("lands on Insights when that is the chosen default", async () => {
    renderIndex("insights");
    await waitFor(() => expect(screen.getByText("Insights Page")).toBeTruthy());
  });

  it("treats an unrecognized value as Home rather than a dead end", async () => {
    // "dashboard" is an older spelling still sitting in some prefs files.
    renderIndex("dashboard");
    await waitFor(() => expect(screen.queryByText("Agents Page")).toBeNull());
    expect(screen.queryByText("Insights Page")).toBeNull();
  });

  it("still sends a fresh install to setup, whatever the preference says", async () => {
    // Being dropped into an unfinished setup matters more than a view choice.
    renderIndex("agents", true);
    await waitFor(() => expect(screen.getByText("Settings Page")).toBeTruthy());
  });

  it("does not override an explicit request for /home", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.includes("/api/onboarding")) return jsonResponse({ firstRun: false });
      if (u.includes("/api/settings")) {
        return jsonResponse({ ui: { theme: "dark", mode: "auto", default_view: "agents" } });
      }
      return jsonResponse({});
    });
    render(
      <MemoryRouter initialEntries={["/home"]}>
        <Routes>
          <Route path="/home" element={<HomeGate />} />
          <Route path="/agents" element={<div>Agents Page</div>} />
        </Routes>
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.queryByText("Agents Page")).toBeNull());
  });
});

/**
 * The preference decides where the app opens, not where it goes once open. The
 * navigation's Home link points at "/", so honoring the preference on every
 * mount of the root route made clicking Home land on Agents — Home became
 * unreachable, and the back button bounced you forward again (#3556).
 */
describe("HomeGate default view only redirects the entry navigation", () => {
  function mockPrefs(defaultView: string) {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.includes("/api/onboarding")) return jsonResponse({ firstRun: false });
      if (u.includes("/api/settings")) {
        return jsonResponse({ ui: { theme: "dark", mode: "auto", default_view: defaultView } });
      }
      return jsonResponse({});
    });
  }

  function renderRoot() {
    return render(
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route path="/" element={<HomeGate honorDefaultView />} />
          <Route path="/agents" element={<div>Agents Page</div>} />
          <Route path="/settings" element={<div>Settings Page</div>} />
        </Routes>
      </MemoryRouter>,
    );
  }

  it("leaves Home alone when the app was opened somewhere else", async () => {
    // Someone opened /agents directly and then clicked Home. Nothing about that
    // is an app launch.
    __resetDefaultViewForTests("/agents");
    mockPrefs("agents");

    renderRoot();

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(screen.queryByText("Agents Page")).toBeNull();
  });

  it("redirects once, then lets Home be reached", async () => {
    __resetDefaultViewForTests("/");
    mockPrefs("agents");

    const first = renderRoot();
    await waitFor(() => expect(screen.getByText("Agents Page")).toBeTruthy());
    first.unmount();

    // This is the Home click, and the back button: same route, second visit.
    renderRoot();
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(screen.queryByText("Agents Page")).toBeNull();
  });

  it("counts the decision as made even when the preference is Home", async () => {
    __resetDefaultViewForTests("/");
    mockPrefs("home");

    const first = renderRoot();
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    first.unmount();

    renderRoot();
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(screen.queryByText("Agents Page")).toBeNull();
  });
});
