import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { Layout } from "../Layout";
import { ThemeProvider } from "../../context/ThemeContext";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

/** Health reports a degraded runtime → the degraded banner renders. */
function mockApi() {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    if (u.includes("/api/health")) {
      return jsonResponse({
        status: "degraded",
        degraded: { runtime: "docker runtime unavailable — agents fall back to tmux" },
      });
    }
    if (u.includes("/api/system/info")) return jsonResponse({ hostname: "host", os: "darwin", arch: "arm64" });
    // Doctor: everything short of essentials so the setup nudge also arms.
    if (u.includes("/api/doctor")) return jsonResponse({ categories: [] });
    return jsonResponse([]);
  });
}

function renderLayout() {
  return render(
    <ThemeProvider>
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route element={<Layout />}>
            <Route index element={<div data-testid="page" />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
  window.localStorage?.clear();
  window.sessionStorage?.clear();
  mockApi();
});

describe("Layout readiness surfacing", () => {
  it("degraded banner links to the readiness surface instead of telling you to run a CLI", async () => {
    renderLayout();
    const links = await waitFor(() => {
      const found = screen.getAllByRole("link", { name: "Check setup" });
      expect(found.length).toBeGreaterThan(0);
      return found;
    });
    for (const link of links) {
      expect(link.getAttribute("href")).toBe("/readiness");
    }
    // The old, non-actionable copy is gone.
    expect(screen.queryByText(/run mycel doctor/i)).not.toBeInTheDocument();
  });

  it("no longer exposes a standalone Setup entry in the drawer footer", async () => {
    renderLayout();
    // Setup folded into Settings (the Setup section) + the /welcome wizard;
    // the drawer footer no longer carries its own Setup link.
    await screen.findByRole("link", { name: "Settings" });
    expect(screen.queryByRole("link", { name: "Setup" })).not.toBeInTheDocument();
  });
});
