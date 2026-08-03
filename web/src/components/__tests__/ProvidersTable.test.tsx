/**
 * ProvidersTable.test — the table must not offer an action the provider's detail
 * page knows is impossible.
 *
 * The table showed Install/Update whenever an install hint was non-empty, while
 * the detail page gated the real control on the hint being runnable. So Install
 * for cursor led to a page that only offers the URL as copyable text: a button
 * that promises an installation and delivers a link (#3475).
 */
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { ProviderInfo } from "../../api/client";
import { ProvidersTable } from "../ProvidersTable";

function provider(overrides: Partial<ProviderInfo> = {}): ProviderInfo {
  return {
    name: "codex",
    description: "OpenAI Codex CLI",
    binary: "codex",
    command: "codex",
    install_hint: "npm install -g @openai/codex",
    version: "",
    status: "not_installed",
    models: [],
    total_cost_usd: 0,
    total_tokens: 0,
    agent_count: 0,
    installed: false,
    enabled: false,
    ...overrides,
  } as ProviderInfo;
}

function renderTable(providers: ProviderInfo[]) {
  return render(
    <MemoryRouter>
      <ProvidersTable providers={providers} search="" />
    </MemoryRouter>,
  );
}

describe("ProvidersTable install actions", () => {
  it("offers Install when the hint is a command the daemon can run", () => {
    renderTable([provider()]);
    expect(screen.getByRole("button", { name: "Install" })).toBeTruthy();
  });

  it("offers Update for an installed provider with a runnable hint", () => {
    renderTable([provider({ installed: true, version: "1.0.0", status: "healthy" })]);
    expect(screen.getByRole("button", { name: "Update" })).toBeTruthy();
  });

  it("offers neither for a hint that is only a download page", () => {
    // cursor's real hint. There is no command here to run.
    renderTable([
      provider({ name: "cursor", binary: "cursor-agent", install_hint: "https://cursor.sh" }),
    ]);

    expect(screen.queryByRole("button", { name: "Install" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Update" })).toBeNull();
    // The row is still reachable — the detail page is where the URL is shown.
    expect(screen.getByRole("button", { name: /configure/i })).toBeTruthy();
  });

  it("still offers Configure for a provider it cannot install", () => {
    renderTable([
      provider({ name: "cursor", install_hint: "https://cursor.sh", installed: true, version: "2026.07.23" }),
    ]);
    expect(screen.getByRole("button", { name: /configure/i })).toBeTruthy();
  });
});
