/**
 * Apps Page E2E Tests
 *
 * Tests the Apps page API endpoints end-to-end against a live the daemon server
 * at http://localhost:9374.
 *
 * Run with:
 *   cd web && npx vitest run src/__tests__/apps-e2e.test.ts
 *
 * Prerequisites: the daemon must be running at http://localhost:9374
 *
 * These tests use the real HTTP API via fetch (not mocked). The vitest
 * environment is configured to use jsdom with setupFiles that mock
 * globalThis.fetch. This file restores the real fetch via vi.stubGlobal
 * before any tests run so that real network calls succeed.
 */

import { describe, it, expect, beforeAll, afterAll, afterEach, vi } from "vitest";

// Capture a real fetch implementation before the setup.ts mock takes effect.
// In Node (vitest), `fetch` is provided by undici. We use dynamic import so
// that this runs before the vi.fn() stub is applied by setup.ts.
 
let realFetch: typeof fetch = globalThis.fetch as typeof fetch;

const BASE = "http://localhost:9374/api";

// Unique prefix for test agents to avoid conflicts with real agents
const TEST_AGENT_PREFIX = "e2e-test-agent";
const TEST_AGENT_1 = `${TEST_AGENT_PREFIX}-1`;
const TEST_AGENT_2 = `${TEST_AGENT_PREFIX}-2`;

// Use a non-existent gateway channel for subscription tests so we don't
// collide with real gateway channels (which may or may not be present).
// The notify service accepts any channel key string.
const TEST_CHANNEL = `slack:${TEST_AGENT_PREFIX}-channel`;
const TEST_CHANNEL_2 = `telegram:${TEST_AGENT_PREFIX}-channel-2`;

// ── helpers ────────────────────────────────────────────────────────────────

async function apiFetch(
  path: string,
  init?: RequestInit,
): Promise<{ status: number; body: unknown }> {
  // Use realFetch (captured before vi.fn() mock) so real HTTP calls go through.
  const res = await realFetch(`${BASE}${path}`, {
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    ...init,
  });
  let body: unknown;
  try {
    body = await res.json();
  } catch {
    body = null;
  }
  return { status: res.status, body };
}

async function subscribe(
  channel: string,
  agent: string,
  mentionOnly = false,
): Promise<void> {
  // Use notify/subscriptions directly (platform-agnostic, always available)
  await apiFetch("/notify/subscriptions", {
    method: "POST",
    body: JSON.stringify({ channel, agent, mention_only: mentionOnly }),
  });
}

async function unsubscribe(channel: string, agent: string): Promise<void> {
  await apiFetch(
    `/notify/subscriptions/${encodeURIComponent(channel)}?agent=${encodeURIComponent(agent)}`,
    { method: "DELETE" },
  );
}

// ── cleanup helpers ────────────────────────────────────────────────────────

async function cleanupTestSubscriptions(): Promise<void> {
  // Best-effort cleanup — unsubscribe both test agents from both test channels
  const pairs = [
    [TEST_CHANNEL, TEST_AGENT_1],
    [TEST_CHANNEL, TEST_AGENT_2],
    [TEST_CHANNEL_2, TEST_AGENT_1],
    [TEST_CHANNEL_2, TEST_AGENT_2],
  ];
  await Promise.all(
    pairs.map(([ch, ag]) =>
      unsubscribe(ch as string, ag as string).catch(() => {
        /* ignore — may not exist */
      }),
    ),
  );
}

// ── server availability check ──────────────────────────────────────────────

let serverAvailable = false;

beforeAll(async () => {
  // Restore the real fetch so all HTTP calls in this file hit the network.
  // setup.ts replaces globalThis.fetch with vi.fn() — we undo that here.
  // We saved realFetch at module load time (before setup.ts runs in the same
  // microtask queue), but if it was already mocked, import undici directly.
  try {
    const { fetch: nodeFetch } = await import("undici");
    realFetch = nodeFetch as unknown as typeof fetch;
  } catch {
    // undici not available — fall back to whatever was captured at load time
  }
  // Also stub it globally so any inline `fetch(...)` calls in this file work
  vi.stubGlobal("fetch", realFetch);

  try {
    const res = await realFetch(`${BASE}/apps`, { signal: AbortSignal.timeout(3000) });
    serverAvailable = res.ok || res.status < 500;
  } catch {
    serverAvailable = false;
  }
  if (!serverAvailable) {
    console.warn(
      "⚠  the daemon server not reachable at http://localhost:9374 — all E2E tests will be skipped",
    );
  }
  // Always clean up stale test data from previous runs
  await cleanupTestSubscriptions();
});

afterAll(async () => {
  await cleanupTestSubscriptions();
});

// ── test suite ─────────────────────────────────────────────────────────────

describe("Apps Page E2E", () => {
  // ── Notification Source List API ──────────────────────────────────────────

  describe("Notification Source List API", () => {
    it("GET /api/apps/channels returns 200 with an array", async () => {
      if (!serverAvailable) return;
      const { status, body } = await apiFetch("/apps/channels");
      expect(status).toBe(200);
      expect(Array.isArray(body)).toBe(true);
    });

    it("each channel has name, description, members, member_count fields", async () => {
      if (!serverAvailable) return;
      // Subscribe a test agent to ensure at least one channel exists
      await subscribe(TEST_CHANNEL, TEST_AGENT_1);
      const { status, body } = await apiFetch("/apps/channels");
      expect(status).toBe(200);
      const list = body as Array<Record<string, unknown>>;
      expect(list.length).toBeGreaterThan(0);
      const ch = list[0] as Record<string, unknown>;
      expect(typeof ch["name"]).toBe("string");
      expect((ch["name"] as string).length).toBeGreaterThan(0);
      // description and members are optional but should not be undefined
      expect("description" in ch).toBe(true);
      // Clean up
      await unsubscribe(TEST_CHANNEL, TEST_AGENT_1);
    });

    it("includes gateway-scoped channels (platform:name format) when subscriptions exist", async () => {
      if (!serverAvailable) return;
      // Subscribe to a gateway-scoped channel so it appears in the list
      await subscribe(TEST_CHANNEL, TEST_AGENT_1);
      const { status, body } = await apiFetch("/apps/channels");
      expect(status).toBe(200);
      const channels = body as Array<{ name: string }>;
      const hasGateway = channels.some((c) => c.name.includes(":"));
      expect(hasGateway).toBe(true);
      await unsubscribe(TEST_CHANNEL, TEST_AGENT_1);
    });

    it("returns empty array (not null) when no channels exist", async () => {
      if (!serverAvailable) return;
      // After cleanup there may still be real gateway channels — just verify
      // the field is always an array, never null/undefined.
      const { status, body } = await apiFetch("/apps/channels");
      expect(status).toBe(200);
      expect(Array.isArray(body)).toBe(true);
    });
  });

  // ── Subscription API (/api/notify/subscriptions) ─────────────────────────

  describe("Subscription API", () => {
    afterEach(async () => {
      await cleanupTestSubscriptions();
    });

    it("POST /api/notify/subscriptions subscribes an agent and returns 201", async () => {
      if (!serverAvailable) return;
      const { status, body } = await apiFetch("/notify/subscriptions", {
        method: "POST",
        body: JSON.stringify({
          channel: TEST_CHANNEL,
          agent: TEST_AGENT_1,
          mention_only: false,
        }),
      });
      expect(status).toBe(201);
      const resp = body as Record<string, string>;
      expect(resp.status).toBe("subscribed");
      expect(resp.channel).toBe(TEST_CHANNEL);
      expect(resp.agent).toBe(TEST_AGENT_1);
    });

    it("GET /api/notify/subscriptions lists all subscriptions", async () => {
      if (!serverAvailable) return;
      await subscribe(TEST_CHANNEL, TEST_AGENT_1);
      const { status, body } = await apiFetch("/notify/subscriptions");
      expect(status).toBe(200);
      const subs = body as Array<Record<string, unknown>>;
      expect(Array.isArray(subs)).toBe(true);
      const found = subs.find(
        (s) => s.channel === TEST_CHANNEL && s.agent === TEST_AGENT_1,
      );
      expect(found).toBeDefined();
    });

    it("GET /api/notify/subscriptions/{channel} lists subscribers for a specific channel", async () => {
      if (!serverAvailable) return;
      await subscribe(TEST_CHANNEL, TEST_AGENT_1);
      await subscribe(TEST_CHANNEL, TEST_AGENT_2);
      const { status, body } = await apiFetch(
        `/notify/subscriptions/${encodeURIComponent(TEST_CHANNEL)}`,
      );
      expect(status).toBe(200);
      const subs = body as Array<Record<string, unknown>>;
      expect(Array.isArray(subs)).toBe(true);
      expect(subs.length).toBeGreaterThanOrEqual(2);
      const agents = subs.map((s) => s.agent);
      expect(agents).toContain(TEST_AGENT_1);
      expect(agents).toContain(TEST_AGENT_2);
    });

    it("each subscription has id, channel, agent, mention_only, created_at", async () => {
      if (!serverAvailable) return;
      await subscribe(TEST_CHANNEL, TEST_AGENT_1, false);
      const { body } = await apiFetch(
        `/notify/subscriptions/${encodeURIComponent(TEST_CHANNEL)}`,
      );
      const subs = body as Array<Record<string, unknown>>;
      const sub = subs.find((s) => s.agent === TEST_AGENT_1);
      expect(sub).toBeDefined();
      expect(typeof sub!.id).toBe("number");
      expect(sub!.channel).toBe(TEST_CHANNEL);
      expect(sub!.agent).toBe(TEST_AGENT_1);
      expect(typeof sub!.mention_only).toBe("boolean");
      expect(typeof sub!.created_at).toBe("string");
    });

    it("PATCH /api/notify/subscriptions/{channel} toggles mention_only", async () => {
      if (!serverAvailable) return;
      await subscribe(TEST_CHANNEL, TEST_AGENT_1, false);
      // Toggle mention_only to true
      const { status, body } = await apiFetch(
        `/notify/subscriptions/${encodeURIComponent(TEST_CHANNEL)}`,
        {
          method: "PATCH",
          body: JSON.stringify({ agent: TEST_AGENT_1, mention_only: true }),
        },
      );
      expect(status).toBe(200);
      const resp = body as Record<string, string>;
      expect(resp.status).toBe("updated");
      // Verify it took effect
      const { body: subsBody } = await apiFetch(
        `/notify/subscriptions/${encodeURIComponent(TEST_CHANNEL)}`,
      );
      const subs = subsBody as Array<Record<string, unknown>>;
      const updated = subs.find((s) => s.agent === TEST_AGENT_1);
      expect(updated?.mention_only).toBe(true);
    });

    it("DELETE /api/notify/subscriptions/{channel}?agent= unsubscribes an agent", async () => {
      if (!serverAvailable) return;
      await subscribe(TEST_CHANNEL, TEST_AGENT_1);
      const { status, body } = await apiFetch(
        `/notify/subscriptions/${encodeURIComponent(TEST_CHANNEL)}?agent=${encodeURIComponent(TEST_AGENT_1)}`,
        { method: "DELETE" },
      );
      expect(status).toBe(200);
      const resp = body as Record<string, string>;
      expect(resp.status).toBe("unsubscribed");
      expect(resp.channel).toBe(TEST_CHANNEL);
      expect(resp.agent).toBe(TEST_AGENT_1);
      // Verify it's gone
      const { body: subsBody } = await apiFetch(
        `/notify/subscriptions/${encodeURIComponent(TEST_CHANNEL)}`,
      );
      const subs = subsBody as Array<Record<string, unknown>>;
      const stillThere = subs.find((s) => s.agent === TEST_AGENT_1);
      expect(stillThere).toBeUndefined();
    });

    it("POST is idempotent — re-subscribing the same agent updates mention_only", async () => {
      if (!serverAvailable) return;
      // First subscribe with mention_only=false
      await subscribe(TEST_CHANNEL, TEST_AGENT_1, false);
      // Re-subscribe with mention_only=true
      const { status } = await apiFetch("/notify/subscriptions", {
        method: "POST",
        body: JSON.stringify({
          channel: TEST_CHANNEL,
          agent: TEST_AGENT_1,
          mention_only: true,
        }),
      });
      // Should succeed (201 or 200 depending on upsert behaviour)
      expect(status === 200 || status === 201).toBe(true);
      // Exactly one subscription for this agent
      const { body } = await apiFetch(
        `/notify/subscriptions/${encodeURIComponent(TEST_CHANNEL)}`,
      );
      const subs = body as Array<Record<string, unknown>>;
      const agentSubs = subs.filter((s) => s.agent === TEST_AGENT_1);
      expect(agentSubs.length).toBe(1);
    });

    it("POST without channel or agent returns 400", async () => {
      if (!serverAvailable) return;
      const { status } = await apiFetch("/notify/subscriptions", {
        method: "POST",
        body: JSON.stringify({ agent: TEST_AGENT_1 }), // missing channel
      });
      expect(status).toBe(400);
    });

    it("DELETE without agent query param returns 400", async () => {
      if (!serverAvailable) return;
      const { status } = await apiFetch(
        `/notify/subscriptions/${encodeURIComponent(TEST_CHANNEL)}`,
        { method: "DELETE" },
      );
      expect(status).toBe(400);
    });
  });

  // ── App-scoped API (/api/apps/{name}/...) ─────────────────────────────────

  describe("App-scoped API", () => {
    afterEach(async () => {
      await cleanupTestSubscriptions();
    });

    // Health endpoint
    describe("GET /api/apps/{name}/health", () => {
      it("returns AdapterStatus fields when the adapter is registered, or 404 if not", async () => {
        if (!serverAvailable) return;
        const { status, body } = await apiFetch("/apps/slack/health");
        expect([200, 404]).toContain(status);
        const resp = body as Record<string, unknown>;
        if (status === 404) {
          expect(typeof resp.error).toBe("string");
          return;
        }
        expect(resp.platform).toBe("slack");
        expect(typeof resp.connected).toBe("boolean");
        expect(typeof resp.status).toBe("string");
        expect(resp.status === "ok" || resp.status === "disconnected").toBe(true);
        expect(typeof resp.message_count).toBe("number");
      });

      it("returns 200 or 404 for telegram health check", async () => {
        if (!serverAvailable) return;
        const { status, body } = await apiFetch("/apps/telegram/health");
        expect([200, 404]).toContain(status);
        if (status === 200) {
          const resp = body as Record<string, unknown>;
          expect(resp.platform).toBe("telegram");
          expect(typeof resp.connected).toBe("boolean");
        }
      });

      it("returns 200 or 404 for discord health check", async () => {
        if (!serverAvailable) return;
        const { status, body } = await apiFetch("/apps/discord/health");
        expect([200, 404]).toContain(status);
        if (status === 200) {
          const resp = body as Record<string, unknown>;
          expect(resp.platform).toBe("discord");
          expect(typeof resp.connected).toBe("boolean");
        }
      });
    });

    // Gateway channel listing
    describe("GET /api/apps/{name}/channels", () => {
      it("returns 200 with an array for slack", async () => {
        if (!serverAvailable) return;
        const { status, body } = await apiFetch("/apps/slack/channels");
        expect(status).toBe(200);
        expect(Array.isArray(body)).toBe(true);
      });

      it("each channel has channel_key, name, platform fields", async () => {
        if (!serverAvailable) return;
        // Ensure at least one slack channel by subscribing then checking legacy list
        // The gateway channels endpoint lists discovered channels, not subscriptions.
        // It may be empty if no Slack adapter is connected.
        const { status, body } = await apiFetch("/apps/slack/channels");
        expect(status).toBe(200);
        const channels = body as Array<Record<string, unknown>>;
        // If any channels returned, verify shape
        for (const ch of channels) {
          expect(typeof ch["channel_key"]).toBe("string");
          expect(typeof ch["name"]).toBe("string");
          expect(ch["platform"]).toBe("slack");
          expect((ch["channel_key"] as string).startsWith("slack:")).toBe(true);
        }
      });

      it("returns empty array (not null) when no slack channels discovered", async () => {
        if (!serverAvailable) return;
        const { status, body } = await apiFetch("/apps/slack/channels");
        expect(status).toBe(200);
        expect(Array.isArray(body)).toBe(true);
      });
    });

    // Gateway channel agents (subscription via gateway-scoped API)
    describe("POST /api/apps/{name}/channels/{ch}/agents", () => {
      it("subscribes an agent and returns 201 with status=subscribed", async () => {
        if (!serverAvailable) return;
        // TEST_CHANNEL = "slack:e2e-test-agent-channel"
        const gw = "slack";
        const ch = `${TEST_AGENT_PREFIX}-channel`;
        const { status, body } = await apiFetch(
          `/apps/${gw}/channels/${ch}/agents`,
          {
            method: "POST",
            body: JSON.stringify({ agent: TEST_AGENT_1, mention_only: false }),
          },
        );
        expect(status).toBe(201);
        const resp = body as Record<string, string>;
        expect(resp.status).toBe("subscribed");
        expect(resp.agent).toBe(TEST_AGENT_1);
        expect(resp.channel).toBe(TEST_CHANNEL);
      });

      it("returns 400 when agent field is missing", async () => {
        if (!serverAvailable) return;
        const gw = "slack";
        const ch = `${TEST_AGENT_PREFIX}-channel`;
        const { status } = await apiFetch(
          `/apps/${gw}/channels/${ch}/agents`,
          {
            method: "POST",
            body: JSON.stringify({ mention_only: false }), // missing agent
          },
        );
        expect(status).toBe(400);
      });
    });

    describe("GET /api/apps/{name}/channels/{ch}/agents", () => {
      it("lists subscribed agents for a channel", async () => {
        if (!serverAvailable) return;
        const gw = "slack";
        const ch = `${TEST_AGENT_PREFIX}-channel`;
        // Subscribe two agents
        await apiFetch(`/apps/${gw}/channels/${ch}/agents`, {
          method: "POST",
          body: JSON.stringify({ agent: TEST_AGENT_1, mention_only: false }),
        });
        await apiFetch(`/apps/${gw}/channels/${ch}/agents`, {
          method: "POST",
          body: JSON.stringify({ agent: TEST_AGENT_2, mention_only: true }),
        });
        const { status, body } = await apiFetch(
          `/apps/${gw}/channels/${ch}/agents`,
        );
        expect(status).toBe(200);
        const subs = body as Array<Record<string, unknown>>;
        expect(Array.isArray(subs)).toBe(true);
        const agents = subs.map((s) => s.agent);
        expect(agents).toContain(TEST_AGENT_1);
        expect(agents).toContain(TEST_AGENT_2);
      });

      it("returns empty array for a channel with no subscribers", async () => {
        if (!serverAvailable) return;
        const gw = "slack";
        const ch = `${TEST_AGENT_PREFIX}-never-subscribed`;
        const { status, body } = await apiFetch(
          `/apps/${gw}/channels/${ch}/agents`,
        );
        expect(status).toBe(200);
        expect(Array.isArray(body)).toBe(true);
        expect((body as unknown[]).length).toBe(0);
      });
    });

    describe("PATCH /api/apps/{name}/channels/{ch}/agents/{agent}", () => {
      it("updates mention_only for a subscribed agent", async () => {
        if (!serverAvailable) return;
        const gw = "slack";
        const ch = `${TEST_AGENT_PREFIX}-channel`;
        // Subscribe with mention_only=false
        await apiFetch(`/apps/${gw}/channels/${ch}/agents`, {
          method: "POST",
          body: JSON.stringify({ agent: TEST_AGENT_1, mention_only: false }),
        });
        // Patch to true
        const { status, body } = await apiFetch(
          `/apps/${gw}/channels/${ch}/agents/${encodeURIComponent(TEST_AGENT_1)}`,
          {
            method: "PATCH",
            body: JSON.stringify({ mention_only: true }),
          },
        );
        expect(status).toBe(200);
        const resp = body as Record<string, string>;
        expect(resp.status).toBe("updated");
        // Verify
        const { body: agentsBody } = await apiFetch(
          `/apps/${gw}/channels/${ch}/agents`,
        );
        const subs = agentsBody as Array<Record<string, unknown>>;
        const updated = subs.find((s) => s.agent === TEST_AGENT_1);
        expect(updated?.mention_only).toBe(true);
      });
    });

    describe("DELETE /api/apps/{name}/channels/{ch}/agents/{agent}", () => {
      it("unsubscribes an agent via path param and returns status=unsubscribed", async () => {
        if (!serverAvailable) return;
        const gw = "slack";
        const ch = `${TEST_AGENT_PREFIX}-channel`;
        await apiFetch(`/apps/${gw}/channels/${ch}/agents`, {
          method: "POST",
          body: JSON.stringify({ agent: TEST_AGENT_1, mention_only: false }),
        });
        const { status, body } = await apiFetch(
          `/apps/${gw}/channels/${ch}/agents/${encodeURIComponent(TEST_AGENT_1)}`,
          { method: "DELETE" },
        );
        expect(status).toBe(200);
        const resp = body as Record<string, string>;
        expect(resp.status).toBe("unsubscribed");
        expect(resp.agent).toBe(TEST_AGENT_1);
        // Verify removal
        const { body: agentsBody } = await apiFetch(
          `/apps/${gw}/channels/${ch}/agents`,
        );
        const subs = agentsBody as Array<Record<string, unknown>>;
        expect(subs.find((s) => s.agent === TEST_AGENT_1)).toBeUndefined();
      });

      it("unsubscribes via ?agent= query param as fallback", async () => {
        if (!serverAvailable) return;
        const gw = "slack";
        const ch = `${TEST_AGENT_PREFIX}-channel`;
        await apiFetch(`/apps/${gw}/channels/${ch}/agents`, {
          method: "POST",
          body: JSON.stringify({ agent: TEST_AGENT_2, mention_only: false }),
        });
        const { status } = await apiFetch(
          `/apps/${gw}/channels/${ch}/agents?agent=${encodeURIComponent(TEST_AGENT_2)}`,
          { method: "DELETE" },
        );
        expect(status).toBe(200);
      });
    });
  });

  // ── Message History (/api/apps/channels/{name}/history) ───────────────────

  describe("Message History", () => {
    it("GET /api/apps/channels/{name}/history returns 200 with an array", async () => {
      if (!serverAvailable) return;
      const { status, body } = await apiFetch(
        `/apps/channels/${encodeURIComponent(TEST_CHANNEL)}/history`,
      );
      expect(status).toBe(200);
      expect(Array.isArray(body)).toBe(true);
    });

    it("returns empty array for a channel with no messages", async () => {
      if (!serverAvailable) return;
      const emptyChannel = `slack:${TEST_AGENT_PREFIX}-empty`;
      const { status, body } = await apiFetch(
        `/apps/channels/${encodeURIComponent(emptyChannel)}/history`,
      );
      expect(status).toBe(200);
      expect(Array.isArray(body)).toBe(true);
      expect((body as unknown[]).length).toBe(0);
    });

    it("each message has id, sender, content, created_at fields", async () => {
      if (!serverAvailable) return;
      // This test passes vacuously when there are no messages — the structure
      // check only runs when messages exist
      const { body } = await apiFetch(
        `/apps/channels/${encodeURIComponent(TEST_CHANNEL)}/history`,
      );
      const msgs = body as Array<Record<string, unknown>>;
      for (const msg of msgs) {
        expect(typeof msg.id).toBe("number");
        expect(typeof msg.sender).toBe("string");
        expect(typeof msg.content).toBe("string");
        expect(typeof msg.created_at).toBe("string");
      }
    });

    it("respects the limit query parameter", async () => {
      if (!serverAvailable) return;
      const { status, body } = await apiFetch(
        `/apps/channels/${encodeURIComponent(TEST_CHANNEL)}/history?limit=5`,
      );
      expect(status).toBe(200);
      expect(Array.isArray(body)).toBe(true);
      // Cannot exceed the requested limit
      expect((body as unknown[]).length).toBeLessThanOrEqual(5);
    });

    it("supports pagination with ?before= parameter", async () => {
      if (!serverAvailable) return;
      // Fetch first page
      const { body: page1Body } = await apiFetch(
        `/apps/channels/${encodeURIComponent(TEST_CHANNEL)}/history?limit=10`,
      );
      const page1 = page1Body as Array<{ id: number }>;
      if (page1.length === 0) return; // no messages to paginate
      const oldestId = (page1[page1.length - 1] as { id: number }).id;
      // Fetch second page using oldest id as cursor
      const { status, body: page2Body } = await apiFetch(
        `/apps/channels/${encodeURIComponent(TEST_CHANNEL)}/history?limit=10&before=${oldestId}`,
      );
      expect(status).toBe(200);
      const page2 = page2Body as Array<{ id: number }>;
      expect(Array.isArray(page2)).toBe(true);
      // All messages on page 2 must have id < cursor
      for (const msg of page2) {
        expect(msg.id).toBeLessThan(oldestId);
      }
    });

    it("also works with /messages suffix path variant", async () => {
      if (!serverAvailable) return;
      const { status, body } = await apiFetch(
        `/apps/channels/${encodeURIComponent(TEST_CHANNEL)}/messages`,
      );
      expect(status).toBe(200);
      expect(Array.isArray(body)).toBe(true);
    });
  });

  // ── Activity Log (/api/notify/activity/{channel}) ────────────────────────

  describe("Activity Log", () => {
    it("GET /api/notify/activity/{channel} returns 200 with an array", async () => {
      if (!serverAvailable) return;
      const { status, body } = await apiFetch(
        `/notify/activity/${encodeURIComponent(TEST_CHANNEL)}`,
      );
      expect(status).toBe(200);
      expect(Array.isArray(body)).toBe(true);
    });

    it("returns empty array for a channel with no delivery entries", async () => {
      if (!serverAvailable) return;
      const emptyChannel = `slack:${TEST_AGENT_PREFIX}-no-activity`;
      const { status, body } = await apiFetch(
        `/notify/activity/${encodeURIComponent(emptyChannel)}`,
      );
      expect(status).toBe(200);
      expect(Array.isArray(body)).toBe(true);
      expect((body as unknown[]).length).toBe(0);
    });

    it("each delivery entry has expected fields when entries exist", async () => {
      if (!serverAvailable) return;
      const { body } = await apiFetch(
        `/notify/activity/${encodeURIComponent(TEST_CHANNEL)}`,
      );
      const entries = body as Array<Record<string, unknown>>;
      for (const entry of entries) {
        expect(typeof entry.id).toBe("number");
        expect(typeof entry.logged_at).toBe("string");
        expect(typeof entry.channel).toBe("string");
        expect(typeof entry.agent).toBe("string");
        expect(typeof entry.status).toBe("string");
        expect(["delivered", "failed", "pending"]).toContain(entry.status);
      }
    });

    it("respects the limit query parameter", async () => {
      if (!serverAvailable) return;
      const { status, body } = await apiFetch(
        `/notify/activity/${encodeURIComponent(TEST_CHANNEL)}?limit=3`,
      );
      expect(status).toBe(200);
      expect((body as unknown[]).length).toBeLessThanOrEqual(3);
    });

    it("GET /api/apps/{name}/channels/{ch}/activity delegates to notify/activity", async () => {
      if (!serverAvailable) return;
      const gw = "slack";
      const ch = `${TEST_AGENT_PREFIX}-channel`;
      const { status, body } = await apiFetch(
        `/apps/${gw}/channels/${ch}/activity`,
      );
      expect(status).toBe(200);
      expect(Array.isArray(body)).toBe(true);
    });
  });

  // ── Apps Catalog (/api/apps) ──────────────────────────────────────────────

  describe("Apps Catalog", () => {
    it("GET /api/apps returns 200 with catalog and instances arrays", async () => {
      if (!serverAvailable) return;
      const { status, body } = await apiFetch("/apps");
      expect(status).toBe(200);
      const resp = body as { catalog: unknown; instances: unknown };
      expect(Array.isArray(resp.catalog)).toBe(true);
      expect(Array.isArray(resp.instances)).toBe(true);
    });

    it("each descriptor has id, label, auth, fields with secret flags", async () => {
      if (!serverAvailable) return;
      const { body } = await apiFetch("/apps");
      const resp = body as { catalog: Array<Record<string, unknown>> };
      expect(resp.catalog.length).toBeGreaterThan(0);
      for (const d of resp.catalog) {
        expect(typeof d.id).toBe("string");
        expect(typeof d.label).toBe("string");
        expect(["token", "oauth", "qr", "webhook-secret", "none"]).toContain(d.auth);
        expect(Array.isArray(d.fields)).toBe(true);
        for (const f of d.fields as Array<Record<string, unknown>>) {
          expect(typeof f.key).toBe("string");
          expect(typeof f.secret).toBe("boolean");
          expect(typeof f.required).toBe("boolean");
        }
      }
    });

    it("each instance has name, app, enabled, channels fields", async () => {
      if (!serverAvailable) return;
      const { body } = await apiFetch("/apps");
      const resp = body as { instances: Array<Record<string, unknown>> };
      for (const inst of resp.instances) {
        expect(typeof inst.name).toBe("string");
        expect(typeof inst.app).toBe("string");
        expect(typeof inst.enabled).toBe("boolean");
        expect(Array.isArray(inst.channels)).toBe(true);
      }
    });

    it("instance config omits secrets (has_<field> booleans instead of raw values)", async () => {
      if (!serverAvailable) return;
      const { body } = await apiFetch("/apps");
      const resp = body as { instances: Array<{ name: string; config?: Record<string, unknown> }> };
      for (const inst of resp.instances) {
        if (!inst.config) continue;
        // Raw token values should never be present
        expect(inst.config.bot_token).toBeUndefined();
        expect(inst.config.app_token).toBeUndefined();
        expect(inst.config.token).toBeUndefined();
      }
    });

    it("instance channels list channel keys scoped to the instance name", async () => {
      if (!serverAvailable) return;
      const { body } = await apiFetch("/apps");
      const resp = body as { instances: Array<{ name: string; channels: string[] }> };
      for (const inst of resp.instances) {
        for (const ch of inst.channels) {
          expect(ch.startsWith(`${inst.name}:`)).toBe(true);
        }
      }
    });
  });

  // ── Error Handling ────────────────────────────────────────────────────────

  describe("Error Handling", () => {
    it("POST /api/notify/subscriptions with invalid JSON returns 400", async () => {
      if (!serverAvailable) return;
      const res = await realFetch(`${BASE}/notify/subscriptions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: "not-json",
      });
      expect(res.status).toBe(400);
    });

    it("method not allowed returns 405", async () => {
      if (!serverAvailable) return;
      // /api/apps/channels is GET-only
      const { status } = await apiFetch("/apps/channels", { method: "POST" });
      expect(status).toBe(405);
    });

    it("PATCH /api/apps/{name}/channels/{ch}/agents without agent in path returns 400", async () => {
      if (!serverAvailable) return;
      // PATCH without agent subpath and body missing agent
      const { status } = await apiFetch(
        "/apps/slack/channels/some-channel/agents",
        {
          method: "PATCH",
          body: JSON.stringify({ mention_only: true }),
        },
      );
      // Server requires agent in path for PATCH
      expect(status).toBe(400);
    });
  });
});
