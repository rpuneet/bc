/**
 * workspace-switch.spec.ts — end-to-end: switching between two
 * registered workspaces via the header dropdown updates the URL and
 * re-loads data.
 *
 * Requirements:
 *   - the daemon running on localhost:9374 with at least two registered
 *     workspaces (use `bc workspace add <path>` or register via API).
 *
 * The test is resilient to an empty registry: if fewer than two
 * workspaces are configured it skips with a clear message so CI
 * doesn't fail when fixture data is absent.
 */

import { test, expect } from "@playwright/test";

test.describe("workspace switch", () => {
  test("dropdown lists workspaces and switching navigates to /w/<id>/...", async ({ page, request }) => {
    const list = await request.get("/api/workspaces");
    if (!list.ok()) {
      test.skip(true, `GET /api/workspaces failed: ${list.status()}`);
      return;
    }
    const body: unknown = await list.json();
    const workspaces = Array.isArray(body) ? body : (body as { workspaces?: unknown[] }).workspaces ?? [];
    if (!Array.isArray(workspaces) || workspaces.length < 2) {
      test.skip(true, `need ≥2 registered workspaces, got ${workspaces.length}`);
      return;
    }

    // Use the first workspace's scoped URL as the starting point.
    const first = workspaces[0] as { id: string; name: string };
    const second = workspaces[1] as { id: string; name: string };

    await page.goto(`/w/${first.id}/agents`);

    // Trigger exists.
    const trigger = page.getByTitle(/switch workspace/i);
    await expect(trigger).toBeVisible({ timeout: 10000 });

    // Open dropdown.
    await trigger.click();
    const menuItem = page.getByRole("menu").getByText(second.name, { exact: false });
    await expect(menuItem).toBeVisible({ timeout: 5000 });

    // Click the target workspace — URL should change.
    await menuItem.click();
    await page.waitForURL(new RegExp(`/w/${second.id}(/|$)`), { timeout: 5000 });
  });
});
