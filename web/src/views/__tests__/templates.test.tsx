import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { HeaderSlotProvider } from "../../context/HeaderSlotContext";
import { Templates } from "../Templates";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

function renderTemplates() {
  return render(
    <MemoryRouter initialEntries={["/templates"]}>
      <HeaderSlotProvider>
        <Templates />
      </HeaderSlotProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("Templates marketplace section", () => {
  it("links to the real /marketplace route instead of claiming it is coming soon", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (u.includes("/api/templates")) {
        return jsonResponse([
          { name: "reviewer", description: "Code review agent", mcps: [], secrets: [], plugins: [] },
        ]);
      }
      return jsonResponse([]);
    });

    renderTemplates();

    await waitFor(() => expect(screen.getByText("reviewer")).toBeInTheDocument());

    // The marketplace already ships at /marketplace — the page must link
    // to it, not claim it's a future feature.
    const link = screen.getByRole("link", { name: /Browse the marketplace/ });
    expect(link).toHaveAttribute("href", "/marketplace");

    // No leftover "coming soon" placeholder copy.
    expect(screen.queryByText(/coming soon/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Browse community templates/)).not.toBeInTheDocument();
  });
});
