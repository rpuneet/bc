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
