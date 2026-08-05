/**
 * AgentAppsCard — Notifications subscriptions on the agent detail Config tab.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { AgentAppsCard } from "../AgentAppsCard";

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

const subs = [
  { id: 1, channel: "slack:general", agent: "bot-1", mention_only: false, created_at: NOW },
  { id: 2, channel: "telegram:standup", agent: "bot-1", mention_only: true, created_at: NOW },
  { id: 3, channel: "slack:general", agent: "other-agent", mention_only: false, created_at: NOW },
];

const sources = [
  { name: "slack:general", description: "", members: [], member_count: 0 },
  { name: "telegram:standup", description: "", members: [], member_count: 0 },
  { name: "whatsapp:family", description: "", members: [], member_count: 0 },
  { name: "gmail:alerts", description: "", members: [], member_count: 0 },
  { name: "gmail:news", description: "", members: [], member_count: 0 },
];

function mockRoutes() {
  fetchMock.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/notify/subscriptions")) return jsonResponse(subs);
    if (u.includes("/api/apps/channels") || u.includes("/apps/channels")) return jsonResponse(sources);
    if (init?.method === "POST" || init?.method === "DELETE") {
      return jsonResponse({ status: "ok" });
    }
    return jsonResponse([]);
  });
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("AgentAppsCard", () => {
  it("lists only this agent's notification channel subscriptions", async () => {
    mockRoutes();
    render(
      <MemoryRouter>
        <AgentAppsCard agentName="bot-1" />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("general")).toBeInTheDocument();
    });
    expect(screen.getByText("standup")).toBeInTheDocument();
    expect(screen.getByText("@ mentions")).toBeInTheDocument();
    expect(screen.getByText("all msgs")).toBeInTheDocument();
    expect(screen.getByLabelText("Add notification channel")).toBeInTheDocument();
  });

  it("opens a searchable grouped picker and subscribes", async () => {
    mockRoutes();
    render(
      <MemoryRouter>
        <AgentAppsCard agentName="bot-1" />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByLabelText("Add notification channel")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("Add notification channel"));

    await waitFor(() => {
      expect(screen.getByLabelText("Search channels")).toBeInTheDocument();
    });

    // Group headers for available platforms (gmail + whatsapp; slack/telegram already subscribed).
    expect(screen.getByText("Gmail")).toBeInTheDocument();
    expect(screen.getByText("Whatsapp")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Search channels"), {
      target: { value: "family" },
    });

    await waitFor(() => {
      expect(screen.getByText("family")).toBeInTheDocument();
    });
    expect(screen.queryByText("alerts")).not.toBeInTheDocument();

    fireEvent.click(screen.getByText("family"));

    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        (c) =>
          String(c[0]).includes("/api/apps/whatsapp/channels/family/agents") &&
          (c[1] as RequestInit | undefined)?.method === "POST",
      );
      expect(post).toBeTruthy();
      expect(JSON.parse(String((post![1] as RequestInit).body))).toEqual({
        agent: "bot-1",
        mention_only: false,
      });
    });
  });

  it("unsubscribes via the apps channel-agents route", async () => {
    mockRoutes();
    render(
      <MemoryRouter>
        <AgentAppsCard agentName="bot-1" />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("general")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Unsubscribe from slack:general" }));

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          (c) =>
            String(c[0]) === "/api/apps/slack/channels/general/agents/bot-1" &&
            (c[1] as RequestInit | undefined)?.method === "DELETE",
        ),
      ).toBe(true);
    });
  });
});
