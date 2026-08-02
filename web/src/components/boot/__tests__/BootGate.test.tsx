import { describe, it, expect, vi, beforeEach } from "vitest";
import { useEffect, useState } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route, Navigate } from "react-router-dom";
import { BootGate } from "../BootGate";
import type { SplashTimings } from "../BootSplash";
import { api } from "../../../api/client";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown, ok = true, status = 200) {
  return Promise.resolve({
    ok,
    status,
    statusText: ok ? "OK" : "ERR",
    json: () => Promise.resolve(body),
  } as Response);
}

// Collapse every phase so the sequence completes near-instantly in tests.
const FAST: SplashTimings = {
  drawMs: 0,
  minStreamMs: 0,
  riseMs: 0,
  fadeMs: 0,
  boot: { pollMs: 5, paceMs: 0 },
};

/** Mirrors App.tsx's HomeGate: probe onboarding, redirect fresh installs. */
function GateProbe() {
  const [toSettings, setToSettings] = useState(false);
  useEffect(() => {
    let cancelled = false;
    api
      .getOnboardingState()
      .then((s) => {
        if (!cancelled && s.firstRun) setToSettings(true);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);
  return toSettings ? <Navigate to="/settings" replace /> : <div data-testid="home">home</div>;
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("BootGate", () => {
  it("shows the splash, then hands off to the app once the daemon is healthy", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.includes("/api/health")) return jsonResponse({ status: "ok" });
      if (u.includes("/api/doctor")) return jsonResponse({ categories: [] });
      if (u.includes("/api/logs")) return jsonResponse([]);
      return jsonResponse([]);
    });

    render(
      <MemoryRouter initialEntries={["/"]}>
        <BootGate timings={FAST}>
          <div data-testid="app">the app</div>
        </BootGate>
      </MemoryRouter>,
    );

    // Splash renders first.
    expect(screen.getByLabelText("Starting mycel")).toBeInTheDocument();

    // Then hands off to the wrapped app.
    await waitFor(() => expect(screen.getByTestId("app")).toBeInTheDocument());
  });

  it("streams REAL readiness lines derived from /api/doctor (no fake logs)", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.includes("/api/health")) return jsonResponse({ status: "ok" });
      if (u.includes("/api/doctor"))
        return jsonResponse({
          categories: [
            { name: "Tools", items: [{ name: "git", message: "git 2.44", severity: "ok" }] },
          ],
        });
      if (u.includes("/api/logs")) return jsonResponse([]);
      return jsonResponse([]);
    });

    render(
      <MemoryRouter initialEntries={["/"]}>
        {/* Slow rise so the console is still visible when we assert. */}
        <BootGate timings={{ ...FAST, minStreamMs: 50, riseMs: 300 }}>
          <div data-testid="app">the app</div>
        </BootGate>
      </MemoryRouter>,
    );

    // The real git readiness line (label "git", detail "git 2.44") streams in.
    await waitFor(() => expect(screen.getByText("git 2.44")).toBeInTheDocument());
    expect(screen.getByText("daemon online")).toBeInTheDocument();
  });

  it("first-run hand-off lands on /settings via the existing HomeGate", async () => {
    // The boot splash does not itself route; the app's HomeGate reads
    // /api/onboarding/state and redirects a fresh install to /settings,
    // where setup is a progressive reveal (no separate wizard route).
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.includes("/api/onboarding/state"))
        return jsonResponse({
          firstRun: true,
          hasAgents: false,
          prefsValid: false,
          completed: [],
          step: "",
        });
      if (u.includes("/api/health")) return jsonResponse({ status: "ok" });
      if (u.includes("/api/doctor")) return jsonResponse({ categories: [] });
      if (u.includes("/api/logs")) return jsonResponse([]);
      return jsonResponse([]);
    });

    render(
      <MemoryRouter initialEntries={["/"]}>
        <BootGate timings={FAST}>
          <Routes>
            <Route index element={<GateProbe />} />
            <Route path="settings" element={<div data-testid="settings">settings, revealing</div>} />
          </Routes>
        </BootGate>
      </MemoryRouter>,
    );

    await waitFor(() => expect(screen.getByTestId("settings")).toBeInTheDocument());
  });
});
