/**
 * agent-detail-tabs.spec.ts — end-to-end: deep-linking and keyboard
 * shortcuts on the agent detail page update the tab sub-path.
 *
 * Requirements:
 *   - the daemon running on localhost:9374 with ≥1 registered workspace and
 *     ≥1 agent in that workspace.
 */

import { test, expect } from "@playwright/test";

type Ws = { id: string; name: string; path: string };
type Agent = { name: string };

async function firstWorkspaceAndAgent(
  request: import("@playwright/test").APIRequestContext,
): Promise<{ ws: Ws; agent: Agent } | null> {
  const wsResp = await request.get("/api/workspaces");
  if (!wsResp.ok()) return null;
  const body: unknown = await wsResp.json();
  const list = (Array.isArray(body) ? body : (body as { workspaces?: unknown[] }).workspaces ?? []) as Ws[];
  if (!list.length) return null;
  const ws = list[0];

  // /api/agents filter compares against the agent's bound Workspace
  // (absolute filesystem path), so pass ws.path — not the registry id hash.
  const aResp = await request.get(`/api/agents?workspace=${encodeURIComponent(ws.path)}`);
  if (!aResp.ok()) return null;
  const agents = (await aResp.json()) as Agent[];
  if (!Array.isArray(agents) || !agents.length) return null;
  return { ws, agent: agents[0] };
}

test.describe("agent detail tabs", () => {
  test("deep link /config renders Config tab", async ({ page, request }) => {
    const fx = await firstWorkspaceAndAgent(request);
    if (!fx) {
      test.skip(true, "need a workspace with ≥1 agent");
      return;
    }
    await page.goto(`/w/${fx.ws.id}/agents/${fx.agent.name}/config`);
    await expect(page.getByText("Config")).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain(`/agents/${fx.agent.name}/config`);
  });

  test("keyboard shortcut 3 jumps to Config tab", async ({ page, request }) => {
    const fx = await firstWorkspaceAndAgent(request);
    if (!fx) {
      test.skip(true, "need a workspace with ≥1 agent");
      return;
    }
    await page.goto(`/w/${fx.ws.id}/agents/${fx.agent.name}/attach`);
    await expect(page.getByText("Attach")).toBeVisible({ timeout: 10000 });

    await page.keyboard.press("3");
    await page.waitForURL(new RegExp(`/agents/${fx.agent.name}/config`), { timeout: 5000 });
  });

  test("keyboard shortcut 4 jumps to Metrics tab", async ({ page, request }) => {
    const fx = await firstWorkspaceAndAgent(request);
    if (!fx) {
      test.skip(true, "need a workspace with ≥1 agent");
      return;
    }
    await page.goto(`/w/${fx.ws.id}/agents/${fx.agent.name}/attach`);
    await expect(page.getByText("Attach")).toBeVisible({ timeout: 10000 });

    await page.keyboard.press("4");
    await page.waitForURL(new RegExp(`/agents/${fx.agent.name}/metrics`), { timeout: 5000 });
  });
});
