import { test, expect } from "@playwright/test";

// Unscoped paths that should either render directly or redirect into the
// workspace-scoped equivalent (/w/<active>/<tab>). Retired surfaces
// (/mcp, /doctor, /daemons) are intentionally absent — see App.tsx for
// the redirect table that lands them on the nearest live page.
const pages = [
  { path: "/", name: "Live (root)" },
  { path: "/agents", name: "Agents" },
  { path: "/notifications", name: "Notifications" },
  { path: "/channels", name: "Channels (legacy → Notifications)" },
  { path: "/costs", name: "Costs" },
  { path: "/tools", name: "Tools" },
  { path: "/code", name: "Code" },
  { path: "/cron", name: "Cron" },
  { path: "/secrets", name: "Secrets" },
  { path: "/settings", name: "Settings" },
  { path: "/templates", name: "Templates" },
  { path: "/logs", name: "Logs (legacy → Live)" },
  { path: "/stats", name: "Metrics" },
  { path: "/workspace", name: "Workspace (legacy → Settings)" },
  { path: "/roles", name: "Roles (legacy → Templates)" },
];

test.describe("Smoke tests — every sidebar page loads", () => {
  for (const { path, name } of pages) {
    test(`${name} (${path}) has a visible heading`, async ({ page }) => {
      await page.goto(path);
      const heading = page.locator("h1, h2, h3").first();
      await expect(heading).toBeVisible({ timeout: 10000 });
    });
  }
});
