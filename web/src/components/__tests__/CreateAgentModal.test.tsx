import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { CreateAgentModal } from "../CreateAgentModal";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as Response);
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
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    if (u.includes("/api/repos")) return jsonResponse({ repos: [], default: "/tmp/repo" });
    return jsonResponse([]);
  });
});

describe("CreateAgentModal identity", () => {
  it("shows a live 96px character preview derived from the name", () => {
    renderModal();
    const preview = screen.getByTestId("agent-identity-preview");
    const svg = preview.querySelector("svg");
    expect(svg).not.toBeNull();
    expect(svg?.getAttribute("width")).toBe("96");
    // The preview character is named after the current name field value.
    const nameInput = screen.getByPlaceholderText("agent-name");
    const name = (nameInput as HTMLInputElement).value;
    expect(preview.querySelector(`[aria-label="${name} — idle"]`)).not.toBeNull();
  });

  it("has no shape selector — identity comes from the name alone", () => {
    renderModal();
    expect(screen.queryByText("Shape")).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "hexagon" })).not.toBeInTheDocument();
  });

  it("morphs the character as the name changes", () => {
    renderModal();
    const nameInput = screen.getByPlaceholderText("agent-name");
    fireEvent.change(nameInput, { target: { value: "misty-heron" } });
    const preview = screen.getByTestId("agent-identity-preview");
    expect(preview.querySelector('[aria-label="misty-heron — idle"]')).not.toBeNull();
  });

  it("keeps the regenerate button as 'meet a different agent'", () => {
    renderModal();
    const regen = screen.getByRole("button", { name: "Meet a different agent" });
    const before = (screen.getByPlaceholderText("agent-name") as HTMLInputElement).value;
    fireEvent.click(regen);
    const after = (screen.getByPlaceholderText("agent-name") as HTMLInputElement).value;
    expect(after).not.toBe("");
    // Regeneration produces a (different) generated name and the preview follows.
    const preview = screen.getByTestId("agent-identity-preview");
    expect(preview.querySelector(`[aria-label="${after} — idle"]`)).not.toBeNull();
    expect(after).not.toBe(before);
  });

  it("does not send a shape field to POST /api/agents", async () => {
    renderModal();
    fireEvent.change(screen.getByPlaceholderText("agent-name"), {
      target: { value: "bold-otter" },
    });
    fireEvent.change(screen.getByPlaceholderText("/absolute/path/to/repo"), {
      target: { value: "/tmp/repo" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create agent" }));
    const post = fetchMock.mock.calls.find(
      (c) => String(c[0]) === "/api/agents" && (c[1] as RequestInit | undefined)?.method === "POST",
    );
    expect(post).toBeDefined();
    const body = JSON.parse(String((post![1] as RequestInit).body)) as Record<string, unknown>;
    expect(body.name).toBe("bold-otter");
    expect("shape" in body).toBe(false);
  });
});
