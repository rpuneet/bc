/**
 * Settings — the progressive-disclosure redesign (setup is Settings
 * revealing itself; there is no separate /welcome wizard).
 *
 *   - Renders every section (Setup, Profile, Providers & Tools, Runtime,
 *     Apps, Budgets, Advanced).
 *   - The Setup section reads onboarding state and shows progress.
 *   - The re-run icon (page header, top-right) replays the guided reveal by
 *     clearing onboarding.completed — it never blanks real config.
 *   - Editing a field raises the floating save bar; Save PATCHes /api/settings.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Settings } from "../Settings";
import { REVEAL_ORDER } from "../../settings/useProgressiveReveal";

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
  onboarding: { step: "runtime", completed: ["runtime"] },
};

// A partway-through, agent-less install: setup genuinely unfinished
// (profile has no name yet in this override set below).
const ONBOARDING = { firstRun: false, hasAgents: false, prefsValid: true, completed: ["runtime"], step: "runtime" };

/** Route every GET the page fans out to a sane payload. */
function mockApi(overrides: Record<string, unknown> = {}) {
  const onboarding = (overrides.onboarding as object) ?? ONBOARDING;
  const settings = (overrides.settings as object) ?? SETTINGS;
  fetchMock.mockImplementation((url: string, init?: RequestInit) => {
    if (init?.method === "PATCH") return jsonResponse({ ...settings, ...(overrides.patched as object) });
    if (url.includes("/onboarding/state")) return jsonResponse(onboarding);
    if (url.includes("/settings/injected-instructions")) return jsonResponse({ injected_instructions: "" });
    if (url.includes("/settings")) return jsonResponse(settings);
    if (url.includes("/system/info")) return jsonResponse({ hostname: "test-host", os: "darwin", arch: "arm64" });
    if (url.includes("/providers")) {
      return jsonResponse([
        {
          name: "claude",
          installed: true,
          agent_count: 1,
          total_tokens: 1000,
          total_cost_usd: 0.01,
          version: "1.0.0",
          install_hint: "",
          models: [{ id: "claude-sonnet-4", available: true }],
        },
      ]);
    }
    if (url.includes("/costs/budgets")) return jsonResponse([]);
    if (url.includes("/apps")) return jsonResponse({ catalog: [], instances: [] });
    if (url.includes("/deps")) return jsonResponse({ deps: [] });
    // useReadiness (behind useProgressiveReveal) derives "provider
    // installed" from the doctor report's Tools category, not /providers —
    // give it a claude entry so "providers & tools" can self-complete.
    if (url.includes("/doctor")) {
      return jsonResponse({
        categories: [
          {
            name: "Tools",
            items: [
              { name: "tmux", severity: "ok", message: "installed" },
              { name: "git", severity: "ok", message: "installed" },
              { name: "claude", severity: "ok", message: "installed" },
            ],
          },
        ],
      });
    }
    if (url.includes("/health")) return jsonResponse({ status: "ok" });
    if (url.includes("/agents")) return jsonResponse([]);
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
  it("renders every section, including budgets", async () => {
    mockApi();
    renderSettings();
    for (const label of ["setup", "profile", "providers & tools", "runtime", "apps", "budgets", "advanced"]) {
      expect(await screen.findByText(label)).toBeInTheDocument();
    }
    // Apps still summarizes and drills out to its own full manager.
    const appsLinks = screen.getAllByRole("link").filter((a) => a.getAttribute("href") === "/apps");
    expect(appsLinks.length).toBeGreaterThan(0);
  });

  it("Providers & Tools is folded in directly — a full list, not a drilldown summary", async () => {
    mockApi();
    renderSettings();
    await screen.findByText("providers & tools");
    // The provider renders inline as a real table row (list-only, no card
    // grid) — no more "drill into /tools" indirection.
    expect(await screen.findByText("claude")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /install tools, sign in/i })).not.toBeInTheDocument();
  });

  it("reads onboarding state into the Setup section", async () => {
    // Name unset → profile is outstanding. Runtime is acknowledged
    // (onboarding.completed) and providers self-completes (the mocked
    // provider is installed); apps has neither a connected app nor an
    // acknowledgement, so it's still outstanding too — 3 of 5 done.
    mockApi({ settings: { ...SETTINGS, user: { name: "" } } });
    renderSettings();
    expect(await screen.findByText("3 of 5 sections done")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /re-run setup/i })).toBeInTheDocument();
  });

  it("treats an install that already runs agents as setup-complete (no nag)", async () => {
    mockApi({
      onboarding: { firstRun: false, hasAgents: true, prefsValid: true, completed: [], step: "" },
      settings: { ...SETTINGS, user: { name: "" }, onboarding: { step: "", completed: [] } },
    });
    renderSettings();
    // Even with an unfinished reveal, a live fleet means done.
    await waitFor(() => expect(screen.getAllByText(/Complete/).length).toBeGreaterThan(0));
    expect(screen.getByRole("button", { name: /re-run setup/i })).toBeInTheDocument();
  });

  it("never locks a section that is already satisfied", async () => {
    // Profile has no name, so setup is genuinely unfinished — but a provider
    // is installed, so "providers & tools" is satisfied and must stay
    // reachable. Locking it would hide real config (and the optional
    // budgets section) behind a padlock on a first run.
    mockApi({
      settings: { ...SETTINGS, user: { name: "" }, onboarding: { step: "", completed: [] } },
      onboarding: { firstRun: true, hasAgents: false, prefsValid: true, completed: [], step: "" },
    });
    renderSettings();

    await waitFor(() =>
      expect(screen.getByText("profile").closest("[data-reveal]")).toHaveAttribute("data-reveal", "active"),
    );
    expect(screen.getByText("providers & tools").closest("[data-reveal]")).toHaveAttribute("data-reveal", "complete");
    expect(screen.getByText("budgets").closest("[data-reveal]")).toHaveAttribute("data-reveal", "complete");
    // Satisfied means usable: the body mounted, so the provider row renders.
    expect(await screen.findByText("claude")).toBeInTheDocument();
    // Sections that genuinely aren't done yet still gate behind profile.
    expect(screen.getByText("runtime").closest("[data-reveal]")).toHaveAttribute("data-reveal", "locked");
  });

  it("walks the reveal top-down — gated sections match the rendered order", async () => {
    mockApi();
    const { container } = renderSettings();
    await screen.findByText("budgets");

    const rendered = Array.from(container.querySelectorAll<HTMLElement>("[data-reveal]")).map(
      (card) => card.querySelector("button")?.textContent ?? "",
    );
    const positions = REVEAL_ORDER.map((id) => rendered.findIndex((text) => text.startsWith(id)));
    expect(positions).not.toContain(-1);
    // Ascending — otherwise the "active" card can land below a locked one.
    expect(positions).toEqual([...positions].sort((a, b) => a - b));
  });

  it("the re-run icon clears onboarding.completed without touching other config", async () => {
    mockApi();
    renderSettings();
    const rerun = await screen.findByRole("button", { name: /re-run setup/i });
    fireEvent.click(rerun);

    await waitFor(() => {
      const patched = fetchMock.mock.calls.find(
        ([url, init]) => String(url).includes("/settings") && (init as RequestInit | undefined)?.method === "PATCH",
      );
      expect(patched).toBeTruthy();
      const body = JSON.parse(((patched?.[1] as RequestInit).body) as string);
      expect(body.onboarding).toEqual({ step: "", completed: [] });
      // Only the onboarding key is written — nothing else gets touched.
      expect(Object.keys(body)).toEqual(["onboarding"]);
    });
  });

  it("associates the Name field's label with its input via htmlFor/id", async () => {
    mockApi();
    renderSettings();
    // getByLabelText only resolves through a real htmlFor → id link (or
    // wrapping) — this fails if Field() ever regresses to an orphan label.
    const nameInput = (await screen.findByLabelText("Name")) as HTMLInputElement;
    expect(nameInput.value).toBe("dana");
  });

  it("keeps the password-reveal toggle keyboard-reachable", async () => {
    mockApi();
    renderSettings();

    // Reveal the Advanced > Storage fields and switch to TimescaleDB, whose
    // password field renders the show/hide toggle under test.
    fireEvent.click(await screen.findByText("advanced"));
    fireEvent.click(await screen.findByRole("button", { name: "▸ Storage" }));
    const backendSelect = (await screen.findByLabelText("Backend")) as HTMLSelectElement;
    fireEvent.change(backendSelect, { target: { value: "timescale" } });

    const toggle = await screen.findByRole("button", { name: /show password/i });
    expect(toggle).not.toHaveAttribute("tabindex", "-1");
    toggle.focus();
    expect(toggle).toHaveFocus();
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
