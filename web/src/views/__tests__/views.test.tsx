import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import { HeaderSlotProvider, useHeaderSlotContext } from "../../context/HeaderSlotContext";
import { Agents } from "../Agents";
import { AgentDetail, lifecycleDisabled } from "../AgentDetail";
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
          total_cost_usd: 0.01,
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

  it("peek row shows the agent activity feed with a link to the full feed", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.includes("/activity")) {
        return jsonResponse([
          {
            timestamp: "2026-07-04T00:00:00.000Z",
            event: "PreToolUse",
            message: "Bash: git status",
            data: { tool_name: "Bash", tool_input: { command: "git status" } },
          },
        ]);
      }
      return jsonResponse([
        {
          name: "bot-1",
          role: "engineer",
          tool: "claude",
          state: "working",
          total_cost_usd: 0.01,
          started_at: "",
        },
      ]);
    });
    wrap(<Agents />);
    await waitFor(() => {
      expect(screen.getByText("bot-1")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Peek activity" }));
    // Shared EventRow renders the tool name and its args summary.
    await waitFor(() => {
      expect(screen.getByText("Bash")).toBeInTheDocument();
      expect(screen.getByText("git status")).toBeInTheDocument();
    });
    expect(screen.getByText("View all activity →")).toBeInTheDocument();

    // Toggling again hides the feed.
    fireEvent.click(screen.getByRole("button", { name: "Hide activity" }));
    await waitFor(() => {
      expect(screen.queryByText("View all activity →")).not.toBeInTheDocument();
    });
  });

  it("moves the filter controls into the header's Filters popover", async () => {
    fetchMock.mockReturnValue(
      jsonResponse([
        { name: "bot-1", role: "engineer", tool: "claude", state: "working", total_cost_usd: 0, started_at: "" },
        { name: "bot-2", role: "engineer", tool: "gemini", state: "stopped", total_cost_usd: 0, started_at: "" },
      ]),
    );

    // Render the header slot the way Layout's full-width bar does.
    function HeaderHost() {
      const { slot } = useHeaderSlotContext();
      return (
        <div data-testid="header-host">
          {slot.title}
          {slot.actions}
        </div>
      );
    }
    render(
      <MemoryRouter>
        <HeaderSlotProvider>
          <HeaderHost />
          <Agents />
        </HeaderSlotProvider>
      </MemoryRouter>,
    );
    await waitFor(() => {
      expect(screen.getByText("bot-1")).toBeInTheDocument();
    });

    // The body carries no filter row — the selects live behind the chip.
    expect(screen.queryByLabelText("Filter by state")).not.toBeInTheDocument();

    // Open the Filters chip: selects + group-by-repo appear in the popover.
    const chip = screen.getByRole("button", { name: "Filters" });
    fireEvent.click(chip);
    expect(screen.getByTestId("agents-filters-popover")).toBeInTheDocument();
    expect(screen.getByLabelText("Filter by state")).toBeInTheDocument();
    expect(screen.getByLabelText("Filter by tool")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Group by repo" })).toBeInTheDocument();

    // Selecting a state filters the table and lights the chip badge.
    fireEvent.change(screen.getByLabelText("Filter by state"), { target: { value: "working" } });
    await waitFor(() => {
      expect(screen.queryByText("bot-2")).not.toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Filters" }).textContent).toContain("1");

    // Escape closes the popover.
    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => {
      expect(screen.queryByTestId("agents-filters-popover")).not.toBeInTheDocument();
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
          total_cost_usd: 0,
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

describe("AgentDetail lifecycle controls", () => {
  it("disables Start when the agent is alive, Stop/Restart when stopped (#3283)", () => {
    // Alive states — Start disabled, Stop/Restart enabled.
    for (const state of ["working", "idle", "starting", "stuck", "done", "running"]) {
      expect(lifecycleDisabled(state, false)).toEqual({
        start: true,
        stop: false,
        restart: false,
      });
    }
    // Dead states — Start enabled, Stop/Restart disabled.
    for (const state of ["stopped", "error"]) {
      expect(lifecycleDisabled(state, false)).toEqual({
        start: false,
        stop: true,
        restart: true,
      });
    }
    // In-flight request disables everything regardless of state.
    expect(lifecycleDisabled("working", true)).toEqual({
      start: true,
      stop: true,
      restart: true,
    });
    expect(lifecycleDisabled("stopped", true)).toEqual({
      start: true,
      stop: true,
      restart: true,
    });
  });

  it("renders Start/Stop/Restart controls in the detail header", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.endsWith("/api/agents/bot-1")) {
        return jsonResponse({
          name: "bot-1",
          role: "engineer",
          tool: "claude",
          state: "working",
          total_cost_usd: 0,
          created_at: "2026-07-01T00:00:00Z",
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
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("bot-1")).toBeInTheDocument();
    });

    const start = screen.getByRole("button", { name: "Start agent bot-1" });
    const stop = screen.getByRole("button", { name: "Stop agent bot-1" });
    const restart = screen.getByRole("button", { name: "Restart agent bot-1" });
    // state=working → Start disabled, Stop/Restart enabled.
    expect(start).toBeDisabled();
    expect(stop).toBeEnabled();
    expect(restart).toBeEnabled();
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

  it("renders the notifications home hub when no channel is selected", async () => {
    // Simulate a slack gateway source — the hub lists it grouped by app.
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.includes("/notifications/overview")) {
        return Promise.resolve({
          ok: false,
          status: 404,
          statusText: "Not Found",
          json: () => Promise.resolve({ error: "not found" }),
        } as Response);
      }
      if (u.includes("/api/channels") && !u.includes("/history")) {
        return jsonResponse([
          { name: "slack:general", description: "Gateway channel", members: [], member_count: 0 },
        ]);
      }
      return jsonResponse([]);
    });
    wrap(<Notifications />);
    await waitFor(() => {
      // The hub renders the channel row (leaf name) and the connect card.
      expect(screen.getByText("general")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: /Connect an app/ })).toBeInTheDocument();
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

