import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { HeaderSlotProvider, useHeaderSlotContext } from "../../context/HeaderSlotContext";
import { NotificationActivity } from "../NotificationActivity";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

const NOW = new Date().toISOString();
const SLACK_CH = "slack:general";
const TG_CH = "telegram:standup";

const sources = [
  { name: SLACK_CH, description: "", members: [], member_count: 0 },
  { name: TG_CH, description: "", members: [], member_count: 0 },
];
const stats = [
  { name: SLACK_CH, message_count: 2, member_count: 0, last_activity: NOW, top_senders: [] },
  { name: TG_CH, message_count: 1, member_count: 0, last_activity: NOW, top_senders: [] },
];
const slackHistory = [
  { id: 1, sender: "alice", content: "ship it", created_at: NOW },
];
const tgHistory = [
  { id: 2, sender: "[telegram] Bob", content: "standup in 5", created_at: NOW },
];

function mockRoutes() {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    if (u.includes("/history")) {
      if (u.includes(encodeURIComponent(SLACK_CH)) || u.includes(SLACK_CH)) return jsonResponse(slackHistory);
      if (u.includes(encodeURIComponent(TG_CH)) || u.includes(TG_CH)) return jsonResponse(tgHistory);
      return jsonResponse([]);
    }
    if (u.includes("/stats/channels")) return jsonResponse(stats);
    if (u.includes("/api/channels")) return jsonResponse(sources);
    return jsonResponse([]);
  });
}

function HeaderHost() {
  const { slot } = useHeaderSlotContext();
  return (
    <div data-testid="header-host">
      {slot.title}
      {slot.actions}
    </div>
  );
}

function renderPage() {
  return render(
    <MemoryRouter>
      <HeaderSlotProvider>
        <HeaderHost />
        <NotificationActivity />
      </HeaderSlotProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("NotificationActivity", () => {
  it("renders messages across channels with search and filter controls", async () => {
    mockRoutes();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("ship it")).toBeInTheDocument();
    });
    expect(screen.getByText("standup in 5")).toBeInTheDocument();
    // Telegram sender prefix is cleaned.
    expect(screen.getByText("Bob")).toBeInTheDocument();

    // Controls live in the shared header slot.
    expect(screen.getByLabelText("Search messages")).toBeInTheDocument();
    expect(screen.getByLabelText("Filter by app")).toBeInTheDocument();
    expect(screen.getByLabelText("Filter by channel")).toBeInTheDocument();
  });

  it("filters by search over sender and content", async () => {
    mockRoutes();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("ship it")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Search messages"), { target: { value: "standup" } });
    expect(screen.getByText("standup in 5")).toBeInTheDocument();
    expect(screen.queryByText("ship it")).not.toBeInTheDocument();
  });

  it("filters by platform", async () => {
    mockRoutes();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("ship it")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Filter by app"), { target: { value: "telegram" } });
    expect(screen.getByText("standup in 5")).toBeInTheDocument();
    expect(screen.queryByText("ship it")).not.toBeInTheDocument();
  });
});
