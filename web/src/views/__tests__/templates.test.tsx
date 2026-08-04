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

/**
 * #3550: secrets and plugins are editable and applied at agent create.
 * The old "Saved but not applied" banner is gone.
 */
describe("Templates secrets and plugins", () => {
  function mockTemplates(detail: Record<string, unknown>) {
    fetchMock.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = String(url);
      if (u === "/api/templates" && (!init || init.method === undefined)) {
        return jsonResponse([{ name: detail.name, description: detail.description, mcps: detail.mcps }]);
      }
      if (u.startsWith("/api/templates/")) return jsonResponse(detail);
      return jsonResponse([]);
    });
  }

  it("offers editable Secrets and Plugins fields", async () => {
    mockTemplates({
      name: "reviewer",
      description: "Code review agent",
      mcps: ["mycel"],
      secrets: ["GITHUB_TOKEN"],
      plugins: ["code-review"],
      system_prompt: "review things",
    });
    renderTemplates();

    await waitFor(() => expect(screen.getByText("reviewer")).toBeInTheDocument());
    (await screen.findByText("reviewer")).click();

    await waitFor(() => expect(screen.getByText("GITHUB_TOKEN")).toBeInTheDocument());
    expect(screen.getByText("code-review")).toBeInTheDocument();
    expect(screen.queryByText("Saved but not applied")).not.toBeInTheDocument();

    // Enter edit mode — fields become inputs.
    (await screen.findByRole("button", { name: /^Edit$/i })).click();
    await waitFor(() => {
      expect(screen.getByLabelText(/Secrets \(comma-separated/i)).toBeInTheDocument();
    });
    expect(screen.getByLabelText(/Plugins \(comma-separated/i)).toBeInTheDocument();
  });

  it("shows editable guardrails on the template detail", async () => {
    mockTemplates({
      name: "reviewer",
      description: "Code review agent",
      mcps: ["mycel"],
      secrets: [],
      plugins: [],
      max_cost_usd: 5,
      stuck_timeout_min: 30,
      system_prompt: "review things",
    });
    renderTemplates();

    await waitFor(() => expect(screen.getByText("reviewer")).toBeInTheDocument());
    (await screen.findByText("reviewer")).click();

    await waitFor(() => expect(screen.getByText("$5.00")).toBeInTheDocument());
    expect(screen.getByText("30 min")).toBeInTheDocument();

    (await screen.findByRole("button", { name: /^Edit$/i })).click();
    await waitFor(() => {
      expect(screen.getByLabelText(/Max cost USD/i)).toBeInTheDocument();
    });
    expect(screen.getByLabelText(/Stuck timeout minutes/i)).toBeInTheDocument();
  });
});
