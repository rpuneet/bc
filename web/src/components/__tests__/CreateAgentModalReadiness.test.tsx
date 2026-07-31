import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { CreateAgentModal } from "../CreateAgentModal";
import type { DoctorReport } from "../../api/client";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(body) } as Response);
}

/** Claude installed, codex NOT — runtime healthy. */
const DOCTOR: DoctorReport = {
  categories: [
    {
      name: "Tools",
      items: [
        { name: "tmux", message: "ok", severity: "ok" },
        { name: "git", message: "ok", severity: "ok" },
        { name: "claude", message: "/opt/homebrew/bin/claude", severity: "ok" },
        { name: "codex", message: "not found", fix: "npm install -g @openai/codex", severity: "warn" },
        { name: "image:mycel-agent-claude:latest", message: "present", severity: "ok" },
      ],
    },
  ],
};

function mockApi() {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    if (u.includes("/api/doctor")) return jsonResponse(DOCTOR);
    if (u.includes("/api/health")) return jsonResponse({ status: "ok" });
    if (u.includes("/api/repos")) return jsonResponse({ repos: [], default: "/tmp/repo" });
    return jsonResponse([]);
  });
}

function renderModal() {
  return render(
    <MemoryRouter>
      <CreateAgentModal open onClose={() => undefined} existingNames={[]} />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
  mockApi();
});

describe("CreateAgentModal readiness pre-flight", () => {
  it("warns with the install fix when the selected provider isn't installed", async () => {
    renderModal();

    // Switch the provider to codex, which the doctor reports as missing.
    // The Provider select is the combobox currently holding "claude".
    const selects = screen.getAllByRole("combobox") as HTMLSelectElement[];
    const provider = selects.find((s) => s.value === "claude")!;
    fireEvent.change(provider, { target: { value: "codex" } });

    await waitFor(() => {
      expect(screen.getByText(/isn't installed on this machine/i)).toBeInTheDocument();
    });
    // The exact fix command is one copy away.
    expect(screen.getByText("npm install -g @openai/codex")).toBeInTheDocument();
    // And a shortcut to the full readiness surface is offered.
    expect(screen.getByRole("button", { name: /Open System readiness/i })).toBeInTheDocument();
  });

  it("stays quiet for an installed provider on a healthy runtime", async () => {
    renderModal();
    // Default provider is claude (installed) with docker image present.
    await waitFor(() => {
      // Give the readiness probe time to resolve.
      expect(fetchMock.mock.calls.some((c) => String(c[0]).includes("/api/doctor"))).toBe(true);
    });
    expect(screen.queryByText(/isn't installed on this machine/i)).not.toBeInTheDocument();
  });
});
