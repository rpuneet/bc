/**
 * ProviderDetail's Install / Update controls must be real, not decorative.
 *
 *  - Install streams the provider's install command through the same
 *    POST /api/deps/install NDJSON path Tools.tsx uses (installDep), with
 *    live output — never a "toast that echoes a hint" stub.
 *  - When the install hint is a bare URL (no runnable command — e.g. a GUI
 *    download page) there is nothing to execute, so the control honestly
 *    falls back to a copyable hint instead of a fake "Install" button.
 *  - Update streams a real re-install through POST /api/providers/:name/update.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import { ProviderDetail } from "../ProviderDetail";

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

function baseProvider(overrides: Record<string, unknown> = {}) {
  return {
    name: "codex",
    description: "OpenAI Codex CLI",
    binary: "codex",
    command: "codex",
    install_hint: "npm install -g @openai/codex",
    version: "",
    status: "not_installed",
    models: [],
    total_cost_usd: 0,
    total_tokens: 0,
    agent_count: 0,
    installed: false,
    enabled: false,
    config: {},
    agents: [],
    cost_by_model: [],
    ...overrides,
  };
}

function renderDetail(provider: ReturnType<typeof baseProvider>) {
  fetchMock.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
    const u = String(url);
    if (u.endsWith("/commands")) return jsonResponse([]);
    if (u.endsWith("/mcps")) return jsonResponse([]);
    if (u === "/api/deps/install" && init?.method === "POST") {
      return streamResponse([
        '{"type":"start","command":"npm install -g @openai/codex"}\n',
        '{"type":"log","line":"+ codex@1.2.4"}\n',
        '{"type":"done","code":0}\n',
      ]);
    }
    if (u.endsWith("/update") && init?.method === "POST") {
      return streamResponse([
        '{"type":"start","command":"npm install -g @openai/codex"}\n',
        '{"type":"done","code":0}\n',
      ]);
    }
    if (u === `/api/providers/${provider.name}`) return jsonResponse(provider);
    return jsonResponse(provider);
  });

  return render(
    <MemoryRouter initialEntries={[`/settings/providers/${provider.name}`]}>
      <Routes>
        <Route path="/settings/providers/:provider" element={<ProviderDetail />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("ProviderDetail install", () => {
  it("streams a real install via /api/deps/install for a runnable hint", async () => {
    renderDetail(baseProvider());

    const installBtn = await screen.findByRole("button", { name: "Install" });
    fireEvent.click(installBtn);

    await waitFor(() => {
      const calls = fetchMock.mock.calls.filter(([u]) => u === "/api/deps/install");
      expect(calls.length).toBe(1);
    });
    const [, init] = fetchMock.mock.calls.find(([u]) => u === "/api/deps/install") as [string, RequestInit];
    expect(JSON.parse(init.body as string)).toEqual({ id: "codex", mode: "install" });

    // Live output renders in an aria-live region, and the button reflects
    // real completion rather than an instant fake toast.
    await waitFor(() => expect(screen.getByText("+ codex@1.2.4")).toBeTruthy());
    await waitFor(() => expect(screen.getByRole("button", { name: "Install again" })).toBeTruthy());
  });

  it("falls back to a copyable hint (not a fake Install button) when the hint is a bare URL", async () => {
    renderDetail(baseProvider({ name: "cursor", install_hint: "https://cursor.sh" }));

    await waitFor(() => expect(screen.getAllByText("https://cursor.sh").length).toBeGreaterThan(0));
    expect(screen.queryByRole("button", { name: "Install" })).toBeNull();
  });
});

describe("ProviderDetail update", () => {
  it("streams a real update via POST /api/providers/:name/update", async () => {
    renderDetail(baseProvider({ installed: true, version: "1.0.0", status: "healthy" }));

    const updateBtn = await screen.findByRole("button", { name: "Update now" });
    fireEvent.click(updateBtn);

    await waitFor(() => {
      const calls = fetchMock.mock.calls.filter(([u]) => u === "/api/providers/codex/update");
      expect(calls.length).toBe(1);
    });
    const [, init] = fetchMock.mock.calls.find(([u]) => u === "/api/providers/codex/update") as [string, RequestInit];
    expect(init.method).toBe("POST");

    await waitFor(() => expect(screen.getByRole("button", { name: "Update again" })).toBeTruthy());
  });

  it("does not offer an Update action for a non-runnable install hint", async () => {
    renderDetail(baseProvider({ name: "cursor", install_hint: "https://cursor.sh", installed: true, version: "1.0.0", status: "healthy" }));

    await screen.findByText("cursor");
    expect(screen.queryByRole("button", { name: "Update now" })).toBeNull();
  });

  it("check-update surfaces an honest 'couldn't verify' message when the server reports checked=false", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = String(url);
      if (u.endsWith("/commands")) return jsonResponse([]);
      if (u.endsWith("/mcps")) return jsonResponse([]);
      if (u.endsWith("/check-update") && init?.method === "POST") {
        return jsonResponse({ current_version: "1.0.0", latest_version: "", update_command: "npm install -g @openai/codex", update_available: false, checked: false });
      }
      if (u === "/api/providers/codex") return jsonResponse(baseProvider({ installed: true, version: "1.0.0", status: "healthy" }));
      return jsonResponse({});
    });

    render(
      <MemoryRouter initialEntries={["/settings/providers/codex"]}>
        <Routes>
          <Route path="/settings/providers/:provider" element={<ProviderDetail />} />
        </Routes>
      </MemoryRouter>,
    );

    const checkBtn = await screen.findByRole("button", { name: "Check for Update" });
    fireEvent.click(checkBtn);

    await waitFor(() => expect(screen.getByText(/couldn't verify the latest release automatically/)).toBeTruthy());
  });
});

describe("ProviderDetail error state", () => {
  it("'Back to Settings' navigates to /settings directly instead of relying on history.back()", async () => {
    // A deep-linked / bookmarked /settings/providers/:provider URL has no
    // guaranteed prior in-app history entry (single-entry MemoryRouter
    // simulates that). window.history.back() would have no in-app
    // destination to land on; navigate("/settings") always resolves to a
    // known page.
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u === "/api/providers/ghost") return Promise.reject(new Error("not found"));
      return jsonResponse([]);
    });

    let lastPathname = "";
    function LocationSpy() {
      lastPathname = useLocation().pathname;
      return null;
    }

    render(
      <MemoryRouter initialEntries={["/settings/providers/ghost"]}>
        <LocationSpy />
        <Routes>
          <Route path="/settings" element={<div>Settings Page</div>} />
          <Route path="/settings/providers/:provider" element={<ProviderDetail />} />
        </Routes>
      </MemoryRouter>,
    );

    const backBtn = await screen.findByRole("button", { name: "Back to Settings" });
    fireEvent.click(backBtn);

    await waitFor(() => {
      expect(lastPathname).toBe("/settings");
    });
    expect(screen.getByText("Settings Page")).toBeInTheDocument();
  });
});
