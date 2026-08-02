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
  it("puts the brand and the drawer toggle in the header, above the drawer", async () => {
    mockApi("test-host");
    renderLayout();

    // Brand + toggle now live in the header's brand column, sized to the
    // drawer so the two share an edge.
    const header = screen.getByRole("banner");
    expect(within(header).getByText("mycel")).toBeInTheDocument();
    expect(within(header).getByRole("button", { name: "Collapse sidebar" })).toBeInTheDocument();

    // The desktop drawer no longer carries its own brand row.
    const nav = screen.getByRole("navigation");
    expect(within(nav).queryByText("mycel")).not.toBeInTheDocument();
  });

  it("collapses to an icon rail via the header toggle; the wordmark hides", async () => {
    mockApi("test-host");
    renderLayout();

    const header = screen.getByRole("banner");
    within(header).getByRole("button", { name: "Collapse sidebar" }).click();
    await waitFor(() =>
      expect(within(header).getByRole("button", { name: "Expand sidebar" })).toBeInTheDocument(),
    );
    // Collapsed rail: the mycel wordmark goes away, leaving just the toggle.
    expect(within(header).queryByText("mycel")).not.toBeInTheDocument();
  });

  it("exposes a drawer resize handle when expanded, hidden when collapsed", async () => {
    mockApi("test-host");
    renderLayout();

    expect(screen.getByRole("separator", { name: "Resize sidebar" })).toBeInTheDocument();

    const header = screen.getByRole("banner");
    within(header).getByRole("button", { name: "Collapse sidebar" }).click();
    await waitFor(() =>
      expect(screen.queryByRole("separator", { name: "Resize sidebar" })).not.toBeInTheDocument(),
    );
  });

  it("renders the flattened nav: Marketplace + Insights, no group captions", async () => {
    mockApi("test-host");
    renderLayout();

    expect(screen.getByRole("link", { name: /Marketplace/ })).toHaveAttribute("href", "/marketplace");
    expect(screen.getByRole("link", { name: /Insights/ })).toHaveAttribute("href", "/insights");
    // Templates is a real CRUD feature and gets its own primary-nav entry,
    // distinct from the community Marketplace.
    expect(screen.getByRole("link", { name: /Templates/ })).toHaveAttribute("href", "/templates");
    // Old separate items and captions are gone.
    expect(screen.queryByText("Configure")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Metrics" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Costs" })).not.toBeInTheDocument();
  });

  it("drops the host-named Tools item from the primary nav (it lives in Settings now)", async () => {
    mockApi("Puneets-MacBook-Pro.local");
    renderLayout();

    // The old hostname/Tools nav item is gone from the top nav entirely.
    await waitFor(() => expect(screen.getByRole("link", { name: /Marketplace/ })).toBeInTheDocument());
    expect(screen.queryByRole("link", { name: /Puneets-MacBook-Pro/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /^Host$/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /^Tools$/ })).not.toBeInTheDocument();
  });

  it("keeps Theme, Settings and About in the drawer footer, without a Setup entry", async () => {
    mockApi("test-host");
    renderLayout();

    const nav = screen.getByRole("navigation");
    expect(within(nav).getByRole("button", { name: /Switch theme/ })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "Settings" })).toHaveAttribute("href", "/settings");
    expect(within(nav).getByRole("link", { name: "About" })).toHaveAttribute("href", "/about");
    // Setup moved into Settings — no standalone footer entry.
    expect(within(nav).queryByRole("link", { name: "Setup" })).not.toBeInTheDocument();
  });
});

describe("Layout header back/forward", () => {
  it("renders one Back and one Forward control in the header, on every route", async () => {
    mockApi("test-host");
    renderLayout();

    const header = screen.getByRole("banner");
    expect(within(header).getByRole("button", { name: "Go back" })).toBeInTheDocument();
    expect(within(header).getByRole("button", { name: "Go forward" })).toBeInTheDocument();
  });

  it("Back calls history navigate(-1) and Forward calls navigate(1)", async () => {
    mockApi("test-host");
    render(
      <ThemeProvider>
        <MemoryRouter initialEntries={["/", "/agents", "/apps"]} initialIndex={1}>
          <Routes>
            <Route element={<Layout />}>
              <Route index element={<div data-testid="home-page" />} />
              <Route path="/agents" element={<div data-testid="agents-page" />} />
              <Route path="/apps" element={<div data-testid="apps-page" />} />
            </Route>
          </Routes>
        </MemoryRouter>
      </ThemeProvider>,
    );

    await waitFor(() => expect(screen.getByTestId("agents-page")).toBeInTheDocument());

    const header = screen.getByRole("banner");
    within(header).getByRole("button", { name: "Go back" }).click();
    await waitFor(() => expect(screen.getByTestId("home-page")).toBeInTheDocument());

    within(header).getByRole("button", { name: "Go forward" }).click();
    await waitFor(() => expect(screen.getByTestId("agents-page")).toBeInTheDocument());
  });
});
