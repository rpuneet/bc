/**
 * Settings — the wizard-mirrored redesign.
 *
 *   - Renders every redesigned section (Setup, Profile, Providers, Runtime,
 *     Tools, Apps, Notifications, Budgets, Advanced).
 *   - The Setup section reads onboarding state and shows progress.
 *   - Editing a field raises the floating save bar; Save PATCHes /api/settings.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Settings } from "../Settings";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

const SETTINGS = {
  version: 3,
  user: { name: "dana" },
  server: { host: "127.0.0.1", port: 9374, cors_origin: "*" },
  runtime: { default: "docker", docker: { image: "img", network: "n", docker_socket_path: "/s", extra_mounts: [], cpus: 2, memory_mb: 4096 }, tmux: { session_prefix: "mycel", history_limit: 10000, default_shell: "/bin/bash" } },
  providers: { default: "claude", default_model: "", providers: { claude: { command: "claude" } } },
  storage: { default: "sqlite", sqlite: { path: ".mycel" } },
  logs: { path: "", max_bytes: 0 },
  ui: { theme: "dark", mode: "auto", default_view: "dashboard" },
  notifications: { default_channel: "", enabled: true },
};

// A partway-through, agent-less install: setup genuinely unfinished.
const ONBOARDING = { firstRun: false, hasAgents: false, prefsValid: true, completed: ["welcome", "system", "runtime"], step: "providers" };

/** Route every GET the page fans out to a sane payload. */
function mockApi(overrides: Record<string, unknown> = {}) {
  const onboarding = (overrides.onboarding as object) ?? ONBOARDING;
  fetchMock.mockImplementation((url: string, init?: RequestInit) => {
    if (init?.method === "PATCH") return jsonResponse({ ...SETTINGS, ...(overrides.patched as object) });
    if (url.includes("/onboarding/state")) return jsonResponse(onboarding);
    if (url.includes("/settings/injected-instructions")) return jsonResponse({ injected_instructions: "" });
    if (url.includes("/settings")) return jsonResponse(SETTINGS);
    if (url.includes("/system/info")) return jsonResponse({ hostname: "test-host", os: "darwin", arch: "arm64" });
    if (url.includes("/providers")) return jsonResponse([{ name: "claude", models: [{ id: "claude-sonnet-4", available: true }] }]);
    if (url.includes("/costs/budgets")) return jsonResponse([]);
    if (url.includes("/apps")) return jsonResponse({ catalog: [], instances: [] });
    if (url.includes("/deps")) return jsonResponse({ deps: [] });
    return jsonResponse([]);
  });
}

function renderSettings() {
  return render(
    <MemoryRouter>
      <Settings />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("Settings redesign", () => {
  it("renders the slim section set (4 config sections + link cards)", async () => {
    mockApi();
    renderSettings();
    for (const label of ["setup", "profile", "providers & tools", "runtime", "apps", "advanced"]) {
      expect(await screen.findByText(label)).toBeInTheDocument();
    }
    // Budgets moved to Insights — it is no longer a Settings section.
    expect(screen.queryByText("budgets")).not.toBeInTheDocument();
    // Link cards route out instead of duplicating config.
    expect(screen.getByRole("link", { name: /Open Tools & Providers/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Manage apps/i })).toBeInTheDocument();
  });

  it("Tools & Providers card links to the flat /tools page", async () => {
    mockApi();
    renderSettings();
    expect(await screen.findByRole("link", { name: /Open Tools & Providers/i })).toHaveAttribute("href", "/tools");
  });

  it("reads onboarding state into the Setup section", async () => {
    mockApi();
    renderSettings();
    // 3 of 8 steps completed, no agents yet → "Step 4 of 8".
    expect(await screen.findByText(/Step 4 of 8/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /resume setup/i })).toBeInTheDocument();
  });

  it("treats an install that already runs agents as setup-complete (no nag)", async () => {
    mockApi({ onboarding: { firstRun: false, hasAgents: true, prefsValid: true, completed: [], step: "welcome" } });
    renderSettings();
    // Even with completed:[] and step "welcome", a live fleet means done.
    expect(await screen.findByText(/Complete/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /re-run setup/i })).toBeInTheDocument();
    expect(screen.queryByText(/Step 1 of 8/)).not.toBeInTheDocument();
  });

  it("raises the save bar and PATCHes on edit + save", async () => {
    mockApi();
    renderSettings();

    const nameInput = (await screen.findByPlaceholderText("Your name")) as HTMLInputElement;
    expect(nameInput.value).toBe("dana");
    fireEvent.change(nameInput, { target: { value: "dana-2" } });

    // Floating save bar surfaces the dirty Profile section.
    const bar = (await screen.findByText(/Unsaved:/)).closest("div.sticky") as HTMLElement;
    expect(within(bar).getByText(/Profile/)).toBeInTheDocument();

    fireEvent.click(within(bar).getByRole("button", { name: /^Save$/ }));

    await waitFor(() => {
      const patched = fetchMock.mock.calls.find(
        ([url, init]) => String(url).includes("/settings") && (init as RequestInit | undefined)?.method === "PATCH",
      );
      expect(patched).toBeTruthy();
      const body = JSON.parse(((patched?.[1] as RequestInit).body) as string);
      expect(body.user.name).toBe("dana-2");
    });
  });
});
