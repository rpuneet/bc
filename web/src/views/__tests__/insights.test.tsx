import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { Insights } from "../Insights";
import { AppRoutes } from "../../App";
import { HeaderSlotProvider } from "../../context/HeaderSlotContext";
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

/** Route-aware API mock for the single-page Stats dashboard. */
function mockApi() {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    if (u.includes("/api/costs/agents") || u.includes("/api/costs/models")) {
      return jsonResponse([]);
    }
    if (u.includes("/api/costs")) {
      return jsonResponse({
        input_tokens: 1_200_000,
        output_tokens: 300_000,
        total_tokens: 1_500_000,
        total_cost_usd: 12.34,
        record_count: 42,
      });
    }
    if (u.includes("/api/system/info")) {
      return jsonResponse({ hostname: "test-host", os: "darwin", arch: "arm64" });
    }
    if (u.includes("/api/health")) return jsonResponse({ status: "ok" });
    return jsonResponse([]);
  });
}

function renderInsights(entry = "/insights") {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <HeaderSlotProvider>
        <Insights />
      </HeaderSlotProvider>
    </MemoryRouter>,
  );
}

function LocationProbe() {
  const loc = useLocation();
  return <div data-testid="loc">{loc.pathname + loc.search}</div>;
}

beforeEach(() => {
  fetchMock.mockReset();
  window.localStorage?.clear();
  mockApi();
});

describe("Insights", () => {
  it("renders one dashboard with no tab bar", async () => {
    renderInsights();

    // The old Metrics/Costs tab bar is gone.
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();

    // Chart panels arrive once polling settles.
    await waitFor(() => expect(screen.getByText("No CPU data")).toBeInTheDocument());
  });

  it("shows the KPI strip", async () => {
    renderInsights();

    // The dashboard mounts once the first poll settles (a skeleton shows
    // until then), so wait for a KPI tile label to appear.
    await waitFor(() => expect(screen.getByText("Spend (this range)")).toBeInTheDocument());
    expect(screen.getByText("Active agents")).toBeInTheDocument();
    expect(screen.getByText("Burn rate")).toBeInTheDocument();
  });

  it("renders the grouped section headers", async () => {
    renderInsights();

    // Section headers double as anchor-nav targets; the pill labels and
    // the headers share text, so each label appears at least once.
    await waitFor(() => expect(screen.getByText("No CPU data")).toBeInTheDocument());
    for (const label of ["Cost", "Usage", "System", "Activity"]) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
  });
});

describe("Insights redirects", () => {
  function renderApp(entry: string) {
    return render(
      <ThemeProvider>
        <MemoryRouter initialEntries={[entry]}>
          <LocationProbe />
          <AppRoutes />
        </MemoryRouter>
      </ThemeProvider>,
    );
  }

  it("redirects /costs to /insights", async () => {
    renderApp("/costs");
    await waitFor(() =>
      expect(screen.getByTestId("loc")).toHaveTextContent("/insights"),
    );
  });

  it("redirects /stats and /metrics to /insights", async () => {
    const { unmount } = renderApp("/stats");
    await waitFor(() =>
      expect(screen.getByTestId("loc")).toHaveTextContent("/insights"),
    );
    unmount();

    renderApp("/metrics");
    await waitFor(() =>
      expect(screen.getByTestId("loc")).toHaveTextContent("/insights"),
    );
  });
});
