import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
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

/** Route-aware API mock covering both the Stats and Costs tab data. */
function mockApi() {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    if (u.includes("/api/global/costs")) {
      return jsonResponse({ range: { start: "2026-06-05T00:00:00Z" }, groupBy: "repo", rows: [] });
    }
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
  it("defaults to the Metrics tab and renders the Stats view", async () => {
    renderInsights();

    expect(screen.getByRole("tab", { name: "Metrics" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Costs" })).toHaveAttribute("aria-selected", "false");
    // Stats view content (empty-data panels) arrives once polling settles.
    await waitFor(() => expect(screen.getByText("No CPU data")).toBeInTheDocument());
  });

  it("renders the Costs view (with token stats) on ?tab=costs", async () => {
    renderInsights("/insights?tab=costs");

    expect(screen.getByRole("tab", { name: "Costs" })).toHaveAttribute("aria-selected", "true");
    await waitFor(() => expect(screen.getByText("No cost data in range.")).toBeInTheDocument());
    // Token usage presented alongside dollar costs.
    await waitFor(() => expect(screen.getByText("Total tokens")).toBeInTheDocument());
    expect(screen.getByText("1.5M")).toBeInTheDocument();
    expect(screen.getByText("Input tokens")).toBeInTheDocument();
    expect(screen.getByText("Output tokens")).toBeInTheDocument();
  });

  it("switches tabs on click", async () => {
    renderInsights();

    fireEvent.click(screen.getByRole("tab", { name: "Costs" }));
    expect(screen.getByRole("tab", { name: "Costs" })).toHaveAttribute("aria-selected", "true");
    await waitFor(() => expect(screen.getByText("No cost data in range.")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("tab", { name: "Metrics" }));
    expect(screen.getByRole("tab", { name: "Metrics" })).toHaveAttribute("aria-selected", "true");
    await waitFor(() => expect(screen.getByText("No CPU data")).toBeInTheDocument());
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

  it("redirects /costs to /insights?tab=costs", async () => {
    renderApp("/costs");
    await waitFor(() =>
      expect(screen.getByTestId("loc")).toHaveTextContent("/insights?tab=costs"),
    );
  });

  it("redirects /stats and /metrics to /insights?tab=metrics", async () => {
    const { unmount } = renderApp("/stats");
    await waitFor(() =>
      expect(screen.getByTestId("loc")).toHaveTextContent("/insights?tab=metrics"),
    );
    unmount();

    renderApp("/metrics");
    await waitFor(() =>
      expect(screen.getByTestId("loc")).toHaveTextContent("/insights?tab=metrics"),
    );
  });
});
