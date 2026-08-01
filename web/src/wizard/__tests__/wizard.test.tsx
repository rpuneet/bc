/**
 * Setup wizard — first-run routing + shell.
 *
 *   - HomeGate redirects "/" to /welcome when the daemon reports firstRun.
 *   - HomeGate stays on Home when firstRun is false (the common case).
 *   - The wizard resumes at the saved step and renders its chrome.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { AppRoutes } from "../../App";
import { Welcome } from "../Welcome";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

const SETTINGS = {
  version: 2,
  user: { name: "" },
  server: { host: "127.0.0.1", port: 9374, cors_origin: "*" },
  runtime: { default: "docker", docker: {}, tmux: {} },
  providers: { default: "claude", providers: {} },
  storage: { default: "sqlite", sqlite: { path: ".mycel" } },
  logs: { path: "", max_bytes: 0 },
  ui: { theme: "dark", mode: "auto", default_view: "dashboard" },
};

/** Route fetches by URL so onboarding/settings/agents each get sane data. */
function mockApi(onboarding: Record<string, unknown>) {
  fetchMock.mockImplementation((url: string) => {
    if (url.includes("/onboarding/state")) return jsonResponse(onboarding);
    if (url.includes("/settings")) return jsonResponse(SETTINGS);
    return jsonResponse([]);
  });
}

let lastLocation = "";
function LocationSpy() {
  const loc = useLocation();
  lastLocation = loc.pathname;
  return null;
}

beforeEach(() => {
  fetchMock.mockReset();
  lastLocation = "";
});

describe("first-run routing", () => {
  it("redirects home to /welcome on a fresh install", async () => {
    mockApi({ firstRun: true, hasAgents: false, prefsValid: true, completed: [], step: "" });
    render(
      <MemoryRouter initialEntries={["/"]}>
        <LocationSpy />
        <AppRoutes />
      </MemoryRouter>,
    );
    await waitFor(() => expect(lastLocation).toBe("/welcome"));
  });

  it("stays on Home when not a first run", async () => {
    mockApi({ firstRun: false, hasAgents: true, prefsValid: true, completed: ["done"], step: "done" });
    render(
      <MemoryRouter initialEntries={["/"]}>
        <LocationSpy />
        <AppRoutes />
      </MemoryRouter>,
    );
    // Give the gate's probe time to resolve, then assert it never left "/".
    await new Promise((r) => setTimeout(r, 20));
    expect(lastLocation).toBe("/");
  });
});

describe("wizard shell", () => {
  it("renders the welcome step and its progress chrome", async () => {
    mockApi({ firstRun: true, hasAgents: false, prefsValid: true, completed: [], step: "" });
    render(
      <MemoryRouter initialEntries={["/welcome"]}>
        <Welcome />
      </MemoryRouter>,
    );
    expect(await screen.findByRole("heading", { name: /welcome to mycel/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /get started/i })).toBeInTheDocument();
    expect(screen.getByText(/skip setup/i)).toBeInTheDocument();
  });

  it("resumes at the saved step", async () => {
    mockApi({ firstRun: true, hasAgents: false, prefsValid: true, completed: ["welcome"], step: "runtime" });
    render(
      <MemoryRouter initialEntries={["/welcome"]}>
        <Welcome />
      </MemoryRouter>,
    );
    expect(await screen.findByRole("heading", { name: /where your agents run/i })).toBeInTheDocument();
  });
});
