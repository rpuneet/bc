import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import { Agents } from "../Agents";
import { AgentDetail } from "../AgentDetail";
import { Notifications } from "../Notifications";
import { Tools } from "../Tools";
import { Live } from "../Live";
import { Cron } from "../Cron";
import { Secrets } from "../Secrets";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function wrap(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

beforeEach(() => {
  fetchMock.mockReset();
});

function expectSkeletonLoading(container: HTMLElement) {
  const pulseElements = container.querySelectorAll(".animate-pulse");
  expect(pulseElements.length).toBeGreaterThan(0);
}

describe("Agents", () => {
  it("renders skeleton loading then agent list", async () => {
    fetchMock.mockReturnValue(
      jsonResponse([
        {
          name: "bot-1",
          role: "engineer",
          tool: "claude",
          state: "running",
          cost_usd: 0.01,
          started_at: "",
        },
      ]),
    );
    const { container } = wrap(<Agents />);
    expectSkeletonLoading(container);
    await waitFor(() => {
      expect(screen.getByText("bot-1")).toBeInTheDocument();
    });
  });
});

describe("AgentDetail tab navigation", () => {
  function LocationProbe() {
    const location = useLocation();
    return <div data-testid="location">{location.pathname}</div>;
  }

  it("uses absolute tab URLs — no segment appending across clicks (#3259)", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.endsWith("/api/agents/bot-1")) {
        return jsonResponse({
          name: "bot-1",
          role: "engineer",
          tool: "claude",
          state: "working",
          cost_usd: 0,
          created_at: "2026-07-01T00:00:00Z",
          started_at: "2026-07-01T00:00:00Z",
          updated_at: "2026-07-01T00:00:00Z",
        });
      }
      return jsonResponse([]);
    });

    render(
      <MemoryRouter initialEntries={["/agents/bot-1"]}>
        <Routes>
          <Route path="agents/:name" element={<AgentDetail />} />
          <Route path="agents/:name/*" element={<AgentDetail />} />
        </Routes>
        <LocationProbe />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("bot-1")).toBeInTheDocument();
    });

    // Attach (default) → Config
    fireEvent.click(screen.getByRole("button", { name: "Config" }));
    await waitFor(() => {
      expect(screen.getByTestId("location")).toHaveTextContent("/agents/bot-1/config");
    });

    // Config → Metrics: the old relative builder produced
    // /agents/bot-1/config/metrics here.
    fireEvent.click(screen.getByRole("button", { name: "Metrics" }));
    await waitFor(() => {
      expect(screen.getByTestId("location").textContent).toBe("/agents/bot-1/metrics");
    });

    // Metrics → Code — still absolute.
    fireEvent.click(screen.getByRole("button", { name: "Code" }));
    await waitFor(() => {
      expect(screen.getByTestId("location").textContent).toBe("/agents/bot-1/code");
    });
  });
});

describe("Notifications", () => {
  it("renders skeleton loading then empty state when no gateway sources", async () => {
    // An empty response means no gateway notification sources are connected yet.
    fetchMock.mockReturnValue(jsonResponse([]));
    const { container } = wrap(<Notifications />);
    expectSkeletonLoading(container);
    await waitFor(() => {
      // The Notifications view shows "Connect your first app" when no gateway sources exist.
      expect(screen.getByText("Connect your first app")).toBeInTheDocument();
    });
  });

  it("renders empty state for gateway notification sources", async () => {
    // Simulate a slack gateway source — the frontend renders a feed view.
    fetchMock.mockReturnValue(
      jsonResponse([
        { name: "slack:general", description: "Gateway channel", members: [], member_count: 0 },
      ]),
    );
    wrap(<Notifications />);
    await waitFor(() => {
      // When a gateway source exists but none is selected, shows "Select a channel".
      expect(screen.getByText("Select a channel")).toBeInTheDocument();
    });
  });
});

describe("Tools", () => {
  it("renders skeleton loading then tool list", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url.includes("/providers")) return jsonResponse([]);
      if (url.includes("/tools/check")) return jsonResponse([]);
      return jsonResponse([
        {
          name: "my-tool",
          type: "cli",
          status: "installed",
          command: "/usr/bin/tool",
        },
      ]);
    });
    const { container } = wrap(<Tools />);
    expectSkeletonLoading(container);
    await waitFor(() => {
      expect(screen.getByText("my-tool")).toBeInTheDocument();
    });
  });
});

describe("Live", () => {
  it("renders without crashing", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url.includes("/agents")) return jsonResponse([]);
      if (url.includes("/logs")) return jsonResponse([]);
      return jsonResponse([]);
    });
    wrap(<Live />);
    await waitFor(() => {
      expect(screen.getByText("No activity yet")).toBeInTheDocument();
    });
  });
});

describe("Cron", () => {
  it("renders skeleton loading then cron table", async () => {
    fetchMock.mockReturnValue(
      jsonResponse([
        {
          name: "nightly",
          schedule: "0 0 * * *",
          agent_name: "bot",
          prompt: "",
          command: "",
          enabled: true,
          run_count: 5,
          last_run: null,
          next_run: null,
          created_at: "",
        },
      ]),
    );
    const { container } = wrap(<Cron />);
    expectSkeletonLoading(container);
    await waitFor(() => {
      expect(screen.getByText(/nightly/)).toBeInTheDocument();
    });
  });
});

describe("Secrets", () => {
  it("renders skeleton loading then secrets table", async () => {
    fetchMock.mockReturnValue(
      jsonResponse([
        {
          name: "API_KEY",
          description: "key",
          backend: "env",
          created_at: "2025-01-01",
        },
      ]),
    );
    const { container } = wrap(<Secrets />);
    expectSkeletonLoading(container);
    await waitFor(() => {
      expect(screen.getByText("API_KEY")).toBeInTheDocument();
    });
  });
});

