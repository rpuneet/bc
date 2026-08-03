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
 * The editor used to accept Secrets and render a chip per entry, which read as
 * confirmation: you typed GITHUB_TOKEN, a chip appeared, and the agent never
 * received it (#3550). The fields are gone until they work — but values an
 * earlier build saved are still shown, because hiding them would suggest they
 * had been deleted.
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

  it("does not offer a Secrets field to fill in", async () => {
    mockTemplates({ name: "reviewer", description: "Code review agent", mcps: ["mycel"], secrets: [], plugins: [] });
    renderTemplates();

    await waitFor(() => expect(screen.getByText("reviewer")).toBeInTheDocument());
    expect(screen.queryByLabelText(/secrets/i)).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/GITHUB_TOKEN/)).not.toBeInTheDocument();
  });

  it("drops the Secrets column from the list", async () => {
    mockTemplates({ name: "reviewer", description: "Code review agent", mcps: ["mycel"], secrets: [], plugins: [] });
    renderTemplates();

    await waitFor(() => expect(screen.getByText("reviewer")).toBeInTheDocument());
    expect(screen.queryByRole("columnheader", { name: "Secrets" })).not.toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "MCPs" })).toBeInTheDocument();
  });

  it("still shows a secret an earlier build saved, and says it does nothing", async () => {
    mockTemplates({
      name: "reviewer",
      description: "Code review agent",
      mcps: ["mycel"],
      secrets: ["GITHUB_TOKEN"],
      plugins: [],
      system_prompt: "review things",
    });
    renderTemplates();

    await waitFor(() => expect(screen.getByText("reviewer")).toBeInTheDocument());
    (await screen.findByText("reviewer")).click();

    await waitFor(() => expect(screen.getByText("Saved but not applied")).toBeInTheDocument());
    expect(screen.getByText("GITHUB_TOKEN")).toBeInTheDocument();
    expect(screen.getByText(/do not receive them yet/)).toBeInTheDocument();
  });

  it("says nothing at all when a template records neither", async () => {
    mockTemplates({
      name: "reviewer",
      description: "Code review agent",
      mcps: ["mycel"],
      secrets: [],
      plugins: [],
      system_prompt: "review things",
    });
    renderTemplates();

    (await screen.findByText("reviewer")).click();
    await waitFor(() => expect(screen.getByText("Code review agent")).toBeInTheDocument());
    expect(screen.queryByText("Saved but not applied")).not.toBeInTheDocument();
  });
});
