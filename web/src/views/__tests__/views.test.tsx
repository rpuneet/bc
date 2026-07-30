import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import { HeaderSlotProvider, useHeaderSlotContext } from "../../context/HeaderSlotContext";
import { Agents } from "../Agents";
import { AgentDetail, lifecycleDisabled } from "../AgentDetail";
import { CodeBrowser } from "../../components/code/CodeBrowser";
import { EmptyState } from "../../components/EmptyState";
import { Apps } from "../Apps";
import { Tools } from "../Tools";
import { Live } from "../Live";
import { CustomKeysSection } from "../../components/apps/CustomKeys";

// Monaco loads its editor bundle from a CDN at mount time — stub it out so
// CodeBrowser can render under jsdom.
vi.mock("@monaco-editor/react", () => ({
  default: () => <div data-testid="monaco-editor" />,
  DiffEditor: () => <div data-testid="monaco-diff-editor" />,
}));

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
        { name: "bot-2", role: "engineer", tool: "agy", state: "stopped", total_cost_usd: 0, started_at: "" },
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

  it("collapses stopped agents per repo by default and expands on toggle", async () => {
    // Two repos: repo-a has 1 active + 1 stopped; repo-b has 1 stopped only.
    fetchMock.mockReturnValue(
      jsonResponse([
        { name: "active-1", role: "engineer", tool: "claude", state: "working", repo: "org/repo-a", total_cost_usd: 0, started_at: "" },
        { name: "stopped-1", role: "engineer", tool: "claude", state: "stopped", repo: "org/repo-a", total_cost_usd: 0, started_at: "" },
        { name: "stopped-2", role: "engineer", tool: "agy",    state: "stopped", repo: "org/repo-b", total_cost_usd: 0, started_at: "" },
      ]),
    );

    wrap(<Agents />);
    await waitFor(() => {
      expect(screen.getByText("active-1")).toBeInTheDocument();
    });

    // Stopped agents are hidden by default when groupByRepo is on.
    expect(screen.queryByText("stopped-1")).not.toBeInTheDocument();
    expect(screen.queryByText("stopped-2")).not.toBeInTheDocument();

    // Toggle rows exist — one per repo with stopped agents.
    const toggles = screen.getAllByRole("button", { name: /stopped/ });
    expect(toggles.length).toBeGreaterThanOrEqual(1);

    // Expanding repo-a's stopped section reveals stopped-1. Assert the
    // collapsed toggle exists first so a regression that stops emitting it
    // fails the test rather than silently skipping the reveal check.
    const repoAToggle = toggles.find((b) => b.getAttribute("aria-expanded") === "false");
    expect(repoAToggle).toBeDefined();
    fireEvent.click(repoAToggle!);
    await waitFor(() => {
      // At least one stopped agent is now visible.
      const visible = screen.queryByText("stopped-1") ?? screen.queryByText("stopped-2");
      expect(visible).toBeInTheDocument();
    });
  });
});

/** Renders the header slot the way Layout's full-width bar does, so
 *  content a view contributes via useHeaderSlot (AgentDetail's identity,
 *  lifecycle controls and tabs) is present in the DOM under test. */
function HeaderSlotHost() {
  const { slot } = useHeaderSlotContext();
  return (
    <div data-testid="header-host">
      {slot.title}
      {slot.actions}
    </div>
  );
}

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
        <HeaderSlotProvider>
          <HeaderSlotHost />
          <Routes>
            <Route path="agents/:name" element={<AgentDetail />} />
            <Route path="agents/:name/*" element={<AgentDetail />} />
          </Routes>
          <LocationProbe />
        </HeaderSlotProvider>
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

  it("code tab embeds the CodeBrowser pinned to the agent worktree", async () => {
    const treeCalls: string[] = [];
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
      if (u.includes("/api/code/tree")) {
        treeCalls.push(u);
        // Empty tree → agent has no worktree (or nothing in it).
        return jsonResponse([]);
      }
      return jsonResponse([]);
    });

    render(
      <MemoryRouter initialEntries={["/agents/bot-1/code"]}>
        <HeaderSlotProvider>
          <HeaderSlotHost />
          <Routes>
            <Route path="agents/:name" element={<AgentDetail />} />
            <Route path="agents/:name/*" element={<AgentDetail />} />
          </Routes>
        </HeaderSlotProvider>
      </MemoryRouter>,
    );

    // The embedded browser fetched the tree for this agent's worktree.
    await waitFor(() => {
      expect(treeCalls.length).toBeGreaterThan(0);
    });
    expect(treeCalls[0]).toContain("worktree=bot-1");

    // No worktree → the EmptyState pattern, not the old redirect stub.
    await waitFor(() => {
      expect(screen.getByText("No worktree to browse")).toBeInTheDocument();
    });
    expect(screen.queryByText("Open in Code view")).not.toBeInTheDocument();

    // The "open full view" affordance links to /code with the worktree pinned.
    const fullView = screen.getByRole("link", { name: /Full view/ });
    expect(fullView).toHaveAttribute("href", "/code?worktree=bot-1");
  });
});

describe("CodeBrowser", () => {
  it("renders the file tree with the embedded header controls", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.includes("/api/code/tree")) {
        return jsonResponse([
          { name: "src", path: "src", is_dir: true },
          { name: "main.go", path: "main.go", is_dir: false, size: 42 },
        ]);
      }
      return jsonResponse([]);
    });

    render(
      <MemoryRouter>
        <CodeBrowser
          worktree="bot-1"
          embedded
          fullViewHref="/code?worktree=bot-1"
        />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("src")).toBeInTheDocument();
      expect(screen.getByText("main.go")).toBeInTheDocument();
    });

    // Embedded header: diff/plain toggle (diff is the default), full-view link.
    expect(screen.getByRole("button", { name: "Diff" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Plain" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Full view/ })).toHaveAttribute(
      "href",
      "/code?worktree=bot-1",
    );
    // No worktree dropdown in embedded mode.
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    // No file selected yet → viewer shows the hint, no Monaco mounted.
    expect(screen.getByText("Select a file from the tree")).toBeInTheDocument();
  });

  it("renders the provided emptyState when the worktree has no files", async () => {
    fetchMock.mockReturnValue(jsonResponse([]));

    render(
      <MemoryRouter>
        <CodeBrowser
          worktree="bot-1"
          embedded
          emptyState={<EmptyState title="No worktree to browse" />}
        />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("No worktree to browse")).toBeInTheDocument();
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
        <HeaderSlotProvider>
          <HeaderSlotHost />
          <Routes>
            <Route path="agents/:name" element={<AgentDetail />} />
            <Route path="agents/:name/*" element={<AgentDetail />} />
          </Routes>
        </HeaderSlotProvider>
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

describe("Apps", () => {
  it("renders skeleton loading then empty state when nothing is connected", async () => {
    // An empty response means no apps or channels are connected yet.
    fetchMock.mockReturnValue(jsonResponse([]));
    const { container } = wrap(<Apps />);
    expectSkeletonLoading(container);
    await waitFor(() => {
      // The Apps view shows "Connect your first app" when nothing exists.
      expect(screen.getByText("Connect your first app")).toBeInTheDocument();
    });
  });

  it("renders the apps home hub when no channel is selected", async () => {
    // Simulate a slack channel — the hub lists it grouped by app.
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
      if (u.includes("/history")) return jsonResponse([]);
      if (u.includes("/api/apps/channels")) {
        return jsonResponse([
          { name: "slack:general", description: "Gateway channel", members: [], member_count: 0 },
        ]);
      }
      if (u.includes("/api/apps")) {
        return jsonResponse({
          catalog: [{ id: "slack", label: "Slack", auth: "token", multi: false, fields: [], docs: [] }],
          instances: [{ name: "slack", app: "slack", enabled: true, connected: true, channels: [] }],
        });
      }
      return jsonResponse([]);
    });
    wrap(<Apps />);
    await waitFor(() => {
      // The hub renders the channel row (leaf name) and the connect pill.
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

  it("provider card shows model count and expands model list on click", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url.includes("/providers")) {
        return jsonResponse([
          {
            name: "claude",
            installed: true,
            agent_count: 1,
            total_tokens: 1000,
            total_cost_usd: 0.01,
            models: [
              { id: "claude-opus-4", available: true },
              { id: "claude-sonnet-4", available: false },
            ],
          },
        ]);
      }
      if (url.includes("/tools/check")) return jsonResponse([]);
      return jsonResponse([]);
    });
    wrap(<Tools />);
    await waitFor(() => {
      expect(screen.getByText("claude")).toBeInTheDocument();
    });
    // Model count affordance should be visible
    const modelsBtn = screen.getByRole("button", { name: /Show models for claude/i });
    expect(modelsBtn).toBeInTheDocument();
    // Model list is hidden initially
    expect(screen.queryByText("claude-opus-4")).not.toBeInTheDocument();
    // Expand
    fireEvent.click(modelsBtn);
    await waitFor(() => {
      expect(screen.getByText("claude-opus-4")).toBeInTheDocument();
      expect(screen.getByText("claude-sonnet-4")).toBeInTheDocument();
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

describe("CustomKeysSection", () => {
  it("renders skeleton loading then the custom keys list", async () => {
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
    const { container } = wrap(<CustomKeysSection />);
    expectSkeletonLoading(container);
    await waitFor(() => {
      expect(screen.getByText("API_KEY")).toBeInTheDocument();
    });
    // The ${secret:NAME} usage hint renders per key.
    expect(screen.getByText("${secret:API_KEY}")).toBeInTheDocument();
  });
});

