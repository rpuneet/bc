/**
 * AgentAppsCard — the "Apps" card on the agent detail Config tab.
 * Lists only this agent's channel subscriptions, offers add/remove, and
 * routes the mutations through the /api/apps subscription endpoints.
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
];

function mockRoutes() {
  fetchMock.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/notify/subscriptions")) return jsonResponse(subs);
    if (u.includes("/api/apps/channels")) return jsonResponse(sources);
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
  it("lists only this agent's app channel subscriptions", async () => {
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
    // The mention-only state renders per subscription.
    expect(screen.getByText("@ mentions")).toBeInTheDocument();
    expect(screen.getByText("all msgs")).toBeInTheDocument();
    // whatsapp:family belongs to no subscription — only in the add list.
    const select = screen.getByLabelText("Channel to subscribe");
    expect(select).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "whatsapp:family" })).toBeInTheDocument();
    // Already-subscribed channels don't appear in the add list.
    expect(screen.queryByRole("option", { name: "slack:general" })).not.toBeInTheDocument();
  });

  it("subscribes via the /api/apps channel-agents route", async () => {
    mockRoutes();
    render(
      <MemoryRouter>
        <AgentAppsCard agentName="bot-1" />
      </MemoryRouter>,
    );
    await waitFor(() => {
      expect(screen.getByLabelText("Channel to subscribe")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Channel to subscribe"), {
      target: { value: "whatsapp:family" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Subscribe" }));

    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        (c) =>
          String(c[0]) === "/api/apps/whatsapp/channels/family/agents" &&
          (c[1] as RequestInit | undefined)?.method === "POST",
      );
      expect(post).toBeDefined();
      const body = JSON.parse(String((post![1] as RequestInit).body)) as Record<string, unknown>;
      expect(body.agent).toBe("bot-1");
    });
  });

  it("removes a subscription via DELETE on the same route", async () => {
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
      const del = fetchMock.mock.calls.find(
        (c) =>
          String(c[0]) === "/api/apps/slack/channels/general/agents/bot-1" &&
          (c[1] as RequestInit | undefined)?.method === "DELETE",
      );
      expect(del).toBeDefined();
    });
  });
});
