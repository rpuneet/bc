import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { Layout, prettifyHostname } from "../Layout";
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

/** Mock every API the chrome touches; hostname=null makes /system/info fail. */
function mockApi(hostname: string | null) {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    if (u.includes("/api/system/info")) {
      if (hostname === null) return Promise.reject(new Error("network down"));
      return jsonResponse({ hostname, os: "darwin", arch: "arm64" });
    }
    if (u.includes("/api/health")) return jsonResponse({ status: "ok" });
    return jsonResponse([]);
  });
}

function renderLayout() {
  return render(
    <ThemeProvider>
      <MemoryRouter initialEntries={["/live"]}>
        <Routes>
          <Route element={<Layout />}>
            <Route path="live" element={<div data-testid="page" />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
  window.localStorage?.clear();
});

describe("prettifyHostname", () => {
  it("strips mDNS suffixes and keeps everything else as-is", () => {
    expect(prettifyHostname("Puneets-MacBook-Pro.local")).toBe("Puneets-MacBook-Pro");
    expect(prettifyHostname("nas.lan")).toBe("nas");
    expect(prettifyHostname("build.example.com")).toBe("build.example.com");
    expect(prettifyHostname("plainhost")).toBe("plainhost");
  });
});

describe("Layout chrome", () => {
  it("puts the brand and the drawer toggle inside the drawer, not the header", async () => {
    mockApi("test-host");
    renderLayout();

    const nav = screen.getByRole("navigation");
    // Brand row is the drawer's first row.
    expect(within(nav).getByText("mycel")).toBeInTheDocument();
    // Explicit toggle button lives in the drawer.
    expect(within(nav).getByRole("button", { name: "Collapse sidebar" })).toBeInTheDocument();

    // Header carries no brand, no toggle, no utility menu.
    const header = screen.getByRole("banner");
    expect(within(header).queryByText("mycel")).not.toBeInTheDocument();
    expect(within(header).queryByRole("button", { name: /sidebar/i })).not.toBeInTheDocument();
    expect(within(header).queryByRole("button", { name: "Utilities" })).not.toBeInTheDocument();

    await waitFor(() => expect(screen.getByText("test-host")).toBeInTheDocument());
  });

  it("collapses to an icon rail via the drawer toggle; toggle stays available", async () => {
    mockApi("test-host");
    renderLayout();

    const nav = screen.getByRole("navigation");
    within(nav).getByRole("button", { name: "Collapse sidebar" }).click();
    await waitFor(() =>
      expect(within(nav).getByRole("button", { name: "Expand sidebar" })).toBeInTheDocument(),
    );
    // Wordmark hides in the rail; the mark (home link) stays.
    expect(within(nav).queryByText("mycel")).not.toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "mycel home" })).toBeInTheDocument();
  });

  it("renders the flattened nav: Marketplace + Insights, no group captions", async () => {
    mockApi("test-host");
    renderLayout();

    expect(screen.getByRole("link", { name: /Marketplace/ })).toHaveAttribute("href", "/templates");
    expect(screen.getByRole("link", { name: /Insights/ })).toHaveAttribute("href", "/insights");
    // Old separate items and captions are gone.
    expect(screen.queryByText("Configure")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Metrics" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Costs" })).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("test-host")).toBeInTheDocument());
  });

  it("labels the tools item with the prettified hostname from /api/system/info", async () => {
    mockApi("Puneets-MacBook-Pro.local");
    renderLayout();

    const link = await screen.findByRole("link", { name: /Puneets-MacBook-Pro/ });
    expect(link).toHaveAttribute("href", "/tools");
    expect(screen.queryByText("Puneets-MacBook-Pro.local")).not.toBeInTheDocument();
  });

  it("falls back to the 'Host' label when system info is unavailable", async () => {
    mockApi(null);
    renderLayout();

    const link = screen.getByRole("link", { name: /Host/ });
    expect(link).toHaveAttribute("href", "/tools");
    // Stays on the fallback after the failed fetch settles.
    await waitFor(() =>
      expect(screen.getByRole("link", { name: /Host/ })).toBeInTheDocument(),
    );
  });

  it("keeps Theme, Settings and About in the drawer footer", async () => {
    mockApi("test-host");
    renderLayout();

    const nav = screen.getByRole("navigation");
    expect(within(nav).getByRole("button", { name: /Switch theme/ })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "Settings" })).toHaveAttribute("href", "/settings");
    expect(within(nav).getByRole("link", { name: "About" })).toHaveAttribute("href", "/about");
    await waitFor(() => expect(screen.getByText("test-host")).toBeInTheDocument());
  });
});
