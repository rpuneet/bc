import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { AgentNavTree } from "../AgentNavTree";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as Response);
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("AgentNavTree", () => {
  it("lists running agents as character chips linking to their detail page", async () => {
    fetchMock.mockImplementation(() =>
      jsonResponse([
        { name: "zen-zebra", state: "working", role: "engineer" },
        { name: "lucid-meerkat", state: "idle", role: "base" },
        { name: "old-bear", state: "stopped", role: "base" },
      ]),
    );
    render(
      <MemoryRouter>
        <AgentNavTree />
      </MemoryRouter>,
    );

    await waitFor(() => expect(screen.getByText("zen-zebra")).toBeInTheDocument());
    expect(screen.getByRole("link", { name: /zen-zebra/ })).toHaveAttribute(
      "href",
      "/agents/zen-zebra",
    );
    // Characters render with live state.
    expect(screen.getByRole("img", { name: "zen-zebra — working" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "lucid-meerkat — idle" })).toBeInTheDocument();
    // Stopped agents fold into a muted count row, not chips.
    expect(screen.queryByRole("img", { name: /old-bear/ })).not.toBeInTheDocument();
    expect(screen.getByText("1 stopped")).toBeInTheDocument();
  });

  it("shows a calm empty state when nothing is running", async () => {
    fetchMock.mockImplementation(() => jsonResponse([]));
    render(
      <MemoryRouter>
        <AgentNavTree />
      </MemoryRouter>,
    );
    await waitFor(() =>
      expect(screen.getByText("No running agents")).toBeInTheDocument(),
    );
  });
});
