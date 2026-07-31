import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Readiness } from "../Readiness";
import { deriveReadiness } from "../readiness/readiness";
import type { DoctorReport, HealthReport } from "../../api/client";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

/** A doctor report exercising every severity:
 *  tmux ok, git FAIL, claude ok, codex not-installed (warn), one image present. */
const DOCTOR: DoctorReport = {
  categories: [
    {
      name: "Tools",
      items: [
        { name: "tmux", message: "/opt/homebrew/bin/tmux (tmux 3.5a)", severity: "ok" },
        { name: "git", message: "not found", fix: "brew install git   OR  apt install git", severity: "fail" },
        { name: "claude", message: "/opt/homebrew/bin/claude (2.1.205)", severity: "ok" },
        { name: "codex", message: "not found", fix: "npm install -g @openai/codex", severity: "warn" },
        { name: "image:mycel-agent-claude:latest", message: "present", severity: "ok" },
      ],
    },
  ],
};

/** Health degraded on the runtime → Docker verdict should be a warning. */
const HEALTH: HealthReport = {
  status: "degraded",
  degraded: { runtime: "docker runtime unavailable — agents fall back to tmux" },
};

function mockApi(doctor: DoctorReport = DOCTOR, health: HealthReport = HEALTH) {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    if (u.includes("/api/doctor")) return jsonResponse(doctor);
    if (u.includes("/api/health")) return jsonResponse(health);
    return jsonResponse([]);
  });
}

beforeEach(() => {
  fetchMock.mockReset();
  window.sessionStorage?.clear();
});

describe("deriveReadiness", () => {
  it("marks the runtime a warning when only tmux is usable (Docker degraded)", () => {
    const r = deriveReadiness(DOCTOR, HEALTH);
    expect(r.tmuxOk).toBe(true);
    expect(r.dockerOk).toBe(false);
    expect(r.anyRuntime).toBe(true);
    const runtime = r.groups.find((g) => g.id === "runtime")!;
    expect(runtime.status).toBe("warn");
  });

  it("fails overall when git is missing (an essential)", () => {
    const r = deriveReadiness(DOCTOR, HEALTH);
    expect(r.overall).toBe("setup");
    expect(r.groups.find((g) => g.id === "git")!.status).toBe("fail");
  });

  it("reports installed vs missing providers", () => {
    const r = deriveReadiness(DOCTOR, HEALTH);
    expect(r.providers.claude).toBe(true);
    expect(r.providers.codex).toBe(false);
  });

  it("is ready when a runtime, git and one provider are all present", () => {
    const healthy: DoctorReport = {
      categories: [
        {
          name: "Tools",
          items: [
            { name: "tmux", message: "ok", severity: "ok" },
            { name: "git", message: "ok", severity: "ok" },
            { name: "claude", message: "ok", severity: "ok" },
          ],
        },
      ],
    };
    const r = deriveReadiness(healthy, { status: "ok" });
    expect(r.overall).toBe("ready");
    expect(r.headline).toMatch(/ready to run agents/i);
  });

  it("is 'almost' when the machine is capable but no agent tool is installed", () => {
    const noProvider: DoctorReport = {
      categories: [
        {
          name: "Tools",
          items: [
            { name: "tmux", message: "ok", severity: "ok" },
            { name: "git", message: "ok", severity: "ok" },
            { name: "claude", message: "not found", fix: "npx …", severity: "warn" },
          ],
        },
      ],
    };
    const r = deriveReadiness(noProvider, { status: "ok" });
    expect(r.overall).toBe("almost");
  });

  it("does not throw on an unexpected payload shape", () => {
    expect(() => deriveReadiness(null, null)).not.toThrow();
    // A bare array (what a mis-typed endpoint might return) must be tolerated.
    expect(() => deriveReadiness([] as unknown as DoctorReport, null)).not.toThrow();
  });
});

function renderReadiness() {
  return render(
    <MemoryRouter initialEntries={["/readiness"]}>
      <Readiness />
    </MemoryRouter>,
  );
}

describe("Readiness view", () => {
  it("renders the overall verdict and grouped pass/warn/fail checks", async () => {
    mockApi();
    renderReadiness();

    // Overall verdict — git missing drives "Setup needed".
    await waitFor(() => expect(screen.getByText("Setup needed")).toBeInTheDocument());

    // Runtime group: tmux OK, Docker WARN carrying the degraded reason.
    expect(screen.getByText("Runtime")).toBeInTheDocument();
    expect(screen.getByText(/docker runtime unavailable/i)).toBeInTheDocument();

    // git FAIL surfaces its fix as a copyable command (whitespace-normalized).
    expect(screen.getByText("brew install git OR apt install git")).toBeInTheDocument();

    // Providers: claude ready, codex missing with its install hint.
    expect(screen.getByText("Claude Code")).toBeInTheDocument();
    expect(screen.getByText("npm install -g @openai/codex")).toBeInTheDocument();
  });

  it("re-checks on demand", async () => {
    mockApi();
    renderReadiness();
    await waitFor(() => expect(screen.getByText("Setup needed")).toBeInTheDocument());

    const before = fetchMock.mock.calls.filter((c) => String(c[0]).includes("/api/doctor")).length;
    fireEvent.click(screen.getByRole("button", { name: "Re-check" }));
    await waitFor(() => {
      const after = fetchMock.mock.calls.filter((c) => String(c[0]).includes("/api/doctor")).length;
      expect(after).toBeGreaterThan(before);
    });
  });
});
