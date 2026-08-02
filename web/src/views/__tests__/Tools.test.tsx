/**
 * ProvidersToolsSection's "Enable" toggle for a not-installed CLI tool must
 * not just flip a DB flag — there is nothing installed to enable. Clicking
 * it should route through the real streamed installer (the same POST
 * /api/deps/install mechanism CLIInstallAction uses) and only call
 * enableTool once the install actually succeeds.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { ProvidersToolsSection } from "../../settings/ProvidersToolsSection";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

function streamResponse(chunks: string[]): Response {
  const enc = new TextEncoder();
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(enc.encode(c));
      controller.close();
    },
  });
  return { ok: true, status: 200, body } as unknown as Response;
}

const notInstalledTool = {
  name: "wrangler",
  type: "cli",
  status: "not_installed",
  required: false,
  install_cmd: "npm install -g wrangler",
};

beforeEach(() => {
  fetchMock.mockReset();
});

function renderSection() {
  fetchMock.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
    const u = String(url);
    if (u === "/api/providers") return jsonResponse([]);
    if (u === "/api/tools/unified") return jsonResponse([notInstalledTool]);
    if (u === "/api/system/package-managers") return jsonResponse({ os: "darwin", arch: "arm64", managers: [] });
    if (u === "/api/deps/install" && init?.method === "POST") {
      return streamResponse([
        '{"type":"start","command":"npm install -g wrangler"}\n',
        '{"type":"done","code":0}\n',
      ]);
    }
    if (u === "/api/tools/wrangler/enable" && init?.method === "POST") {
      return jsonResponse({ enabled: true });
    }
    return jsonResponse({});
  });

  return render(
    <MemoryRouter>
      <ProvidersToolsSection />
    </MemoryRouter>,
  );
}

describe("ProvidersToolsSection CLI tools table", () => {
  it("shows the status in Status and the version in Version, not the version twice", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u === "/api/providers") return jsonResponse([]);
      if (u === "/api/tools/unified") {
        return jsonResponse([{ name: "git", type: "cli", status: "installed", version: "2.50.1", required: false }]);
      }
      if (u === "/api/system/package-managers") return jsonResponse({ os: "darwin", arch: "arm64", managers: [] });
      return jsonResponse({});
    });

    render(
      <MemoryRouter>
        <ProvidersToolsSection />
      </MemoryRouter>,
    );

    const nameCell = await screen.findByText("git");
    const cells = Array.from(nameCell.closest("tr")!.querySelectorAll("td")).map((td) => td.textContent?.trim());
    // Tool | Status | Version | Required | Actions
    expect(cells[1]).toBe("Installed");
    expect(cells[2]).toBe("2.50.1");
  });
});

describe("ProvidersToolsSection optional services", () => {
  // DependenciesSection was written, then left with no importer, so the whole
  // /api/deps lifecycle had no entry point in the app and the Code tab's "Edit
  // in VS Code" button — which only renders while mycel-code-server runs — could
  // never appear. This asserts the component is actually mounted.
  it("renders the dependency manager, so /api/deps has an entry point", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u === "/api/providers") return jsonResponse([]);
      if (u === "/api/tools/unified") return jsonResponse([]);
      if (u === "/api/system/package-managers") return jsonResponse({ os: "darwin", arch: "arm64", managers: [] });
      if (u === "/api/deps") {
        return jsonResponse({
          deps: [{
            id: "mycel-code-server",
            name: "mycel-code-server",
            description: "VS Code in the browser",
            state: "stopped",
            deprecated: false,
          }],
        });
      }
      return jsonResponse({});
    });

    render(
      <MemoryRouter>
        <ProvidersToolsSection />
      </MemoryRouter>,
    );

    expect(await screen.findByText("Optional Services")).toBeTruthy();
    // The dependency itself is listed, which means /api/deps was fetched and
    // rendered rather than the heading merely existing.
    await waitFor(() => {
      expect(screen.getAllByText("mycel-code-server").length).toBeGreaterThan(0);
    });
  });
});

describe("ProvidersToolsSection expanded tool details", () => {
  /** Render the table with one CLI tool and expand its row. */
  async function expandTool(tool: Record<string, unknown>) {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u === "/api/providers") return jsonResponse([]);
      if (u === "/api/tools/unified") return jsonResponse([tool]);
      if (u === "/api/system/package-managers") return jsonResponse({ os: "darwin", arch: "arm64", managers: [] });
      return jsonResponse({});
    });

    render(
      <MemoryRouter>
        <ProvidersToolsSection />
      </MemoryRouter>,
    );

    fireEvent.click(await screen.findByText(String(tool.name)));
  }

  it("shows the resolved path and inferred owner, not the bare command name", async () => {
    await expandTool({
      name: "rg",
      type: "cli",
      status: "installed",
      version: "14.1.1",
      required: false,
      command: "rg",
      path: "/opt/homebrew/bin/rg",
      manager: "brew",
    });

    // The real path — the old UI labelled the bare command "rg" as the Path,
    // which told the user nothing.
    expect(await screen.findByText("/opt/homebrew/bin/rg")).toBeInTheDocument();
    expect(screen.getByText("Homebrew")).toBeInTheDocument();
    // The version command was pure derivable noise and is gone.
    expect(screen.queryByText(/Version cmd/)).not.toBeInTheDocument();
    expect(screen.queryByText("rg --version")).not.toBeInTheDocument();
  });

  it("names the update command its manager would use when none is configured", async () => {
    await expandTool({
      name: "rg",
      type: "cli",
      status: "installed",
      required: false,
      command: "rg",
      path: "/opt/homebrew/bin/rg",
      manager: "brew",
    });

    // No upgrade_cmd is configured, so the hint must point at something real
    // instead of "copy the command above" with no command above.
    expect(await screen.findByText("brew upgrade rg")).toBeInTheDocument();
    expect(screen.getByText(/copy the Update command below/)).toBeInTheDocument();
  });

  it("says an OS-provided tool is not mycel's to update", async () => {
    await expandTool({
      name: "git",
      type: "cli",
      status: "installed",
      version: "2.50.1",
      required: false,
      command: "git",
      path: "/usr/bin/git",
      manager: "system",
    });

    expect(await screen.findByText("/usr/bin/git")).toBeInTheDocument();
    expect(screen.getByText("Your OS")).toBeInTheDocument();
    expect(screen.getByText(/update it through the system/i)).toBeInTheDocument();
    // No manager owns it, so no update command may be suggested.
    expect(screen.queryByText(/upgrade git/)).not.toBeInTheDocument();
    // And /usr/bin/git is not mycel's to delete — an Uninstall button here
    // could only ever produce the backend's refusal.
    expect(screen.queryByRole("button", { name: /uninstall/i })).not.toBeInTheDocument();
  });

  it("still offers Uninstall for a tool a package manager owns", async () => {
    await expandTool({
      name: "rg",
      type: "cli",
      status: "installed",
      required: false,
      command: "rg",
      path: "/opt/homebrew/bin/rg",
      manager: "brew",
    });

    expect(await screen.findByRole("button", { name: /uninstall/i })).toBeInTheDocument();
  });

  it("reports honestly when a tool is not on PATH", async () => {
    await expandTool({
      name: "wrangler",
      type: "cli",
      status: "not_installed",
      required: false,
      command: "wrangler",
      manager: "npm",
      install_cmd: "npm install -g wrangler",
    });

    expect(await screen.findByText("Not found on PATH")).toBeInTheDocument();
    expect(screen.getByText("npm (global)")).toBeInTheDocument();
  });

  it("falls back to Unknown when nothing identifies an owner", async () => {
    await expandTool({
      name: "mytool",
      type: "cli",
      status: "installed",
      required: false,
      command: "mytool",
      path: "/usr/local/bin/mytool",
    });

    expect(await screen.findByText("Unknown")).toBeInTheDocument();
  });
});

describe("ProvidersToolsSection providers list", () => {
  it("renders providers as a list/table with no card/grid view toggle", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u === "/api/providers") {
        return jsonResponse([
          { name: "claude", installed: true, agent_count: 1, total_tokens: 1000, total_cost_usd: 0.01, models: [] },
        ]);
      }
      if (u === "/api/tools/unified") return jsonResponse([]);
      if (u === "/api/system/package-managers") return jsonResponse({ os: "darwin", arch: "arm64", managers: [] });
      return jsonResponse({});
    });

    const { container } = render(
      <MemoryRouter>
        <ProvidersToolsSection />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("claude")).toBeInTheDocument();
    });

    // Table/list only — no card-grid view and no view-mode toggle.
    expect(container.querySelector("table")).toBeInTheDocument();
    expect(screen.queryByLabelText("Card view")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Table view")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Sort providers")).not.toBeInTheDocument();
  });
});

describe("ProvidersToolsSection CLI dependency toggle", () => {
  it("routes a not-installed tool's Enable through a real install, then enables it", async () => {
    renderSection();

    const installBtn = await screen.findByRole("switch", { name: "Install and enable wrangler" });
    expect(installBtn.textContent).toBe("Install");
    fireEvent.click(installBtn);

    await waitFor(() => {
      const calls = fetchMock.mock.calls.filter(([u]) => u === "/api/deps/install");
      expect(calls.length).toBe(1);
    });
    const [, installInit] = fetchMock.mock.calls.find(([u]) => u === "/api/deps/install") as [string, RequestInit];
    expect(JSON.parse(installInit.body as string)).toEqual({ id: "wrangler", mode: "install" });

    // enableTool only fires after the install stream reports success.
    await waitFor(() => {
      const calls = fetchMock.mock.calls.filter(([u]) => u === "/api/tools/wrangler/enable");
      expect(calls.length).toBe(1);
    });
  });

  it("does not call enableTool when the install fails", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = String(url);
      if (u === "/api/providers") return jsonResponse([]);
      if (u === "/api/tools/unified") return jsonResponse([notInstalledTool]);
      if (u === "/api/system/package-managers") return jsonResponse({ os: "darwin", arch: "arm64", managers: [] });
      if (u === "/api/deps/install" && init?.method === "POST") {
        return streamResponse(['{"type":"start","command":"npm install -g wrangler"}\n', '{"type":"done","code":1}\n']);
      }
      return jsonResponse({});
    });

    render(
      <MemoryRouter>
        <ProvidersToolsSection />
      </MemoryRouter>,
    );

    const installBtn = await screen.findByRole("switch", { name: "Install and enable wrangler" });
    fireEvent.click(installBtn);

    await waitFor(() => {
      const calls = fetchMock.mock.calls.filter(([u]) => u === "/api/deps/install");
      expect(calls.length).toBe(1);
    });
    // Give any (incorrect) enable call a chance to fire before asserting it didn't.
    await new Promise((r) => setTimeout(r, 10));
    expect(fetchMock.mock.calls.some(([u]) => u === "/api/tools/wrangler/enable")).toBe(false);
  });
});
