import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Agents } from "../Agents";
import { Channels } from "../Channels";
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

describe("Channels", () => {
  it("renders skeleton loading then empty state when no gateway channels", async () => {
    // pkg/channel was deleted; channels are now gateway-backed.
    // An empty response means no gateway channels are connected yet.
    fetchMock.mockReturnValue(jsonResponse([]));
    const { container } = wrap(<Channels />);
    expectSkeletonLoading(container);
    await waitFor(() => {
      // The Channels view shows "Connect your first app" when no gateway channels exist.
      expect(screen.getByText("Connect your first app")).toBeInTheDocument();
    });
  });

  it("renders empty state for gateway channels", async () => {
    // Simulate a slack gateway channel — the frontend renders a feed view.
    fetchMock.mockReturnValue(
      jsonResponse([
        { name: "slack:general", description: "Gateway channel", members: [], member_count: 0 },
      ]),
    );
    wrap(<Channels />);
    await waitFor(() => {
      // When a gateway channel exists but none is selected, shows "Select a notification source".
      expect(screen.getByText("Select a notification source")).toBeInTheDocument();
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

