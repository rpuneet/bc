/**
 * ProviderDefaults.test.tsx — the fleet-default provider/model picker must
 * persist its choice via PATCH /api/settings under providers.default_model
 * and reflect what is already on disk. This is the surface behind the
 * "default model doesn't stick" report, so the persistence contract is
 * pinned here.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ProviderDefaults } from "../ProviderDefaults";
import type { ProviderInfo } from "../../api/client";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(body) } as Response);
}

const providers: ProviderInfo[] = [
  {
    name: "claude", description: "", binary: "claude", command: "claude", install_hint: "",
    version: "1.0", status: "healthy", installed: true, enabled: true,
    total_cost_usd: 0, total_tokens: 0, agent_count: 0,
    models: [{ id: "claude-sonnet-4", available: true }, { id: "claude-opus-4", available: false }],
  },
  {
    name: "codex", description: "", binary: "codex", command: "codex", install_hint: "",
    version: "", status: "not_installed", installed: false, enabled: false,
    total_cost_usd: 0, total_tokens: 0, agent_count: 0, models: [],
  },
];

const settingsBody = {
  providers: { default: "claude", default_model: "", providers: { claude: { command: "claude" } } },
};

beforeEach(() => {
  fetchMock.mockReset();
  fetchMock.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
    const u = String(url);
    if (u.endsWith("/api/settings") && init?.method === "PATCH") return jsonResponse(settingsBody);
    if (u.endsWith("/api/settings")) return jsonResponse(settingsBody);
    if (u.endsWith("/api/system/info")) return jsonResponse({ hostname: "test.local", os: "darwin", arch: "arm64" });
    return jsonResponse({});
  });
});

describe("ProviderDefaults", () => {
  it("loads current defaults and the host line", async () => {
    render(<ProviderDefaults providers={providers} />);
    await waitFor(() => expect((screen.getByLabelText("Default provider") as HTMLSelectElement).value).toBe("claude"));
    // mDNS suffix stripped, os/arch shown.
    await waitFor(() => expect(screen.getByText(/darwin\/arm64/)).toBeTruthy());
  });

  it("persists a chosen default model via PATCH providers.default_model", async () => {
    render(<ProviderDefaults providers={providers} />);
    const modelSelect = await screen.findByLabelText("Default model");
    await waitFor(() => expect((screen.getByLabelText("Default provider") as HTMLSelectElement).value).toBe("claude"));

    fireEvent.change(modelSelect, { target: { value: "claude-sonnet-4" } });

    await waitFor(() => {
      const patch = fetchMock.mock.calls.find(
        (c) => String(c[0]).endsWith("/api/settings") && (c[1] as RequestInit | undefined)?.method === "PATCH",
      );
      expect(patch).toBeTruthy();
      const body = JSON.parse(String((patch![1] as RequestInit).body));
      expect(body.providers.default).toBe("claude");
      expect(body.providers.default_model).toBe("claude-sonnet-4");
    });
    await waitFor(() => expect(screen.getByText("Saved")).toBeTruthy());
  });

  it("drops a model that the newly-selected provider does not offer", async () => {
    render(<ProviderDefaults providers={providers} />);
    await waitFor(() => expect((screen.getByLabelText("Default provider") as HTMLSelectElement).value).toBe("claude"));
    // Seed a model, then switch to codex (no models) — model must clear.
    fireEvent.change(screen.getByLabelText("Default model"), { target: { value: "claude-opus-4" } });
    fireEvent.change(screen.getByLabelText("Default provider"), { target: { value: "codex" } });

    await waitFor(() => {
      const patches = fetchMock.mock.calls.filter(
        (c) => String(c[0]).endsWith("/api/settings") && (c[1] as RequestInit | undefined)?.method === "PATCH",
      );
      const last = JSON.parse(String((patches[patches.length - 1]![1] as RequestInit).body));
      expect(last.providers.default).toBe("codex");
      expect(last.providers.default_model).toBe("");
    });
  });
});
