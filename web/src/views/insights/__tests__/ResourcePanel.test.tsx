import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { ResourcePanel } from "../ResourcePanel";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

const settings = {
  version: 1,
  user: { name: "test" },
  server: { host: "localhost", port: 9374, cors_origin: "*" },
  runtime: {
    default: "docker",
    docker: {
      image: "mycel/agent",
      network: "bridge",
      docker_socket_path: "/var/run/docker.sock",
      extra_mounts: [],
      cpus: 1,
      memory_mb: 1024,
    },
    tmux: { session_prefix: "mycel", history_limit: 5000, default_shell: "/bin/bash" },
  },
  providers: {},
};

function mockApi(latest: unknown[]) {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    if (u.includes("/api/agents/stats/latest")) return jsonResponse(latest);
    if (u.includes("/api/settings")) return jsonResponse(settings);
    if (u.includes("/api/agents")) {
      return jsonResponse([
        { name: "resource-fox", role: "engineer", tool: "claude", state: "working", total_cost_usd: 0, started_at: "", created_at: "", updated_at: "", runtime_backend: "docker", cpus: 2, memory_mb: 4096 },
      ]);
    }
    return jsonResponse([]);
  });
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("ResourcePanel", () => {
  it("shows committed-vs-actual usage per agent when live stats exist", async () => {
    mockApi([
      { time: "2026-08-02T00:00:00Z", agent_name: "resource-fox", role: "engineer", tool: "claude", runtime: "docker", state: "working", cpu_percent: 40, mem_used_bytes: 1_258_291_200 /* ~1.2 GB */, mem_limit_bytes: 0, mem_percent: 0, net_rx_bytes: 0, net_tx_bytes: 0, disk_read_bytes: 0, disk_write_bytes: 0 },
    ]);

    render(<ResourcePanel />);

    await waitFor(() => expect(screen.getByText("resource-fox")).toBeInTheDocument());
    // Cap and live-used figures both render — never just "coming soon". With
    // a single agent, the fleet total and the per-agent row show the same
    // figures, so assert at least one of each rather than uniqueness.
    expect(screen.getByText(/2 CPU cap/)).toBeInTheDocument();
    expect(screen.getAllByText(/0\.4 used/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/1\.2 GB used/).length).toBeGreaterThan(0);
    expect(screen.queryByText(/coming soon/i)).not.toBeInTheDocument();
  });

  it("falls back to caps-only messaging when no live sample exists yet", async () => {
    mockApi([]);

    render(<ResourcePanel />);

    await waitFor(() => expect(screen.getByText("resource-fox")).toBeInTheDocument());
    expect(screen.getByText(/2 CPU cap/)).toBeInTheDocument();
    expect(screen.queryByText(/used/)).not.toBeInTheDocument();
  });
});
