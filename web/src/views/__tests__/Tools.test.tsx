/**
 * Tools.tsx's "Enable" toggle for a not-installed CLI tool must not just
 * flip a DB flag — there is nothing installed to enable. Clicking it should
 * route through the real streamed installer (the same POST /api/deps/install
 * mechanism CLIInstallAction uses) and only call enableTool once the
 * install actually succeeds.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Tools } from "../Tools";

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

function renderTools() {
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
      <Tools />
    </MemoryRouter>,
  );
}

describe("Tools CLI dependency toggle", () => {
  it("routes a not-installed tool's Enable through a real install, then enables it", async () => {
    renderTools();

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
        <Tools />
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
